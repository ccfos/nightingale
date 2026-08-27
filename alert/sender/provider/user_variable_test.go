package provider

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

// withUserVariables 临时注入变量钩子，测试结束后还原，避免污染同包其他用例。
func withUserVariables(t *testing.T, vars map[string]string) {
	t.Helper()
	old := UserVariableGetter
	UserVariableGetter = func() map[string]string { return vars }
	t.Cleanup(func() { UserVariableGetter = old })
}

func TestUserVariableRenderedIntoHTTPConfig(t *testing.T) {
	withUserVariables(t, map[string]string{
		"irp_auth_token": "s3cr3t-token",
		"irp_endpoint":   "https://irp.example.com",
	})

	cfg := &models.HTTPRequestConfig{
		URL: "{{.irp_endpoint}}/alert",
		Headers: map[string]string{
			"Content-Type":   "application/json",
			"X-IRP-TS-Token": "{{.irp_auth_token}}",
		},
		Request: models.RequestDetail{
			Parameters: map[string]string{"access_token": "{{.irp_auth_token}}"},
			Body:       `{"token":"{{.irp_auth_token}}","rule":"{{$event.RuleName}}"}`,
		},
	}

	event := &models.AlertCurEvent{RuleName: "cpu high"}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{event}, map[string]interface{}{}, nil, nil)

	url, headers, parameters := replaceVariables(cfg, tplData)
	if url != "https://irp.example.com/alert" {
		t.Errorf("url = %q, want rendered endpoint", url)
	}
	if headers["X-IRP-TS-Token"] != "s3cr3t-token" {
		t.Errorf("header token = %q, want s3cr3t-token", headers["X-IRP-TS-Token"])
	}
	if headers["Content-Type"] != "application/json" {
		t.Errorf("static header changed: %q", headers["Content-Type"])
	}
	if parameters["access_token"] != "s3cr3t-token" {
		t.Errorf("parameter token = %q, want s3cr3t-token", parameters["access_token"])
	}

	body, err := parseRequestBody(cfg, tplData)
	if err != nil {
		t.Fatalf("parseRequestBody failed: %v", err)
	}
	if !strings.Contains(string(body), `"token":"s3cr3t-token"`) {
		t.Errorf("body = %s, want rendered token", body)
	}
	if !strings.Contains(string(body), `"rule":"cpu high"`) {
		t.Errorf("body = %s, want builtin event still rendered", body)
	}
}

// 钩子未注入（未启动告警引擎的进程、单测）时行为必须与改造前一致：
// 缺失的变量渲染成空串，不报错也不影响内置变量。
func TestUserVariableGetterUnsetKeepsLegacyBehaviour(t *testing.T) {
	old := UserVariableGetter
	UserVariableGetter = nil
	t.Cleanup(func() { UserVariableGetter = old })

	cfg := &models.HTTPRequestConfig{
		URL:     "{{$params.callback_url}}",
		Headers: map[string]string{"X-IRP-TS-Token": "{{.irp_auth_token}}"},
	}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{{}}, nil,
		map[string]string{"callback_url": "http://x/y"}, nil)

	url, headers, _ := replaceVariables(cfg, tplData)
	if url != "http://x/y" {
		t.Errorf("url = %q, want http://x/y", url)
	}
	if headers["X-IRP-TS-Token"] != "" {
		t.Errorf("token = %q, want empty when getter unset", headers["X-IRP-TS-Token"])
	}
}

// 变量名撞上内置 key 时以内置语义为准，否则一个叫 event 的变量会把事件对象顶掉。
func TestUserVariableCannotShadowBuiltinKeys(t *testing.T) {
	withUserVariables(t, map[string]string{
		"event":  "hijacked",
		"params": "hijacked",
		"tpl":    "hijacked",
	})

	event := &models.AlertCurEvent{RuleName: "disk full"}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{event},
		map[string]interface{}{"title": "t"}, map[string]string{"k": "v"}, []string{"a"})

	if got, ok := tplData["event"].(*models.AlertCurEvent); !ok || got.RuleName != "disk full" {
		t.Errorf("event key = %#v, want the alert event", tplData["event"])
	}
	if got, ok := tplData["params"].(map[string]string); !ok || got["k"] != "v" {
		t.Errorf("params key = %#v, want custom params", tplData["params"])
	}
	if _, ok := tplData["tpl"].(map[string]interface{}); !ok {
		t.Errorf("tpl key = %#v, want message template content", tplData["tpl"])
	}
}

func TestRedactKV(t *testing.T) {
	got := redactKV(map[string]string{
		"Content-Type":   "application/json",
		"X-IRP-TS-Token": "s3cr3t",
		"Authorization":  "Bearer abc",
		"X-Api-Key":      "ak",
		"X-Empty-Token":  "",
	})

	want := map[string]string{
		"Content-Type":   "application/json",
		"X-IRP-TS-Token": "***",
		"Authorization":  "***",
		"X-Api-Key":      "***",
		"X-Empty-Token":  "",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("redactKV[%s] = %q, want %q", k, got[k], v)
		}
	}
}

func TestRedactURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"no secret", "https://x.com/a/b", "https://x.com/a/b"},
		{"query secret", "https://x.com/a?access_token=abc&id=1", "https://x.com/a?access_token=%2A%2A%2A&id=1"},
		{"userinfo", "https://user:pass@x.com/a", "https://user@x.com/a"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := redactURL(c.in); got != c.want {
				t.Errorf("redactURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// 端到端到 wire：变量必须真的出现在发出去的请求头/查询串/请求体里。
func TestSendHTTPRequestCarriesUserVariable(t *testing.T) {
	withUserVariables(t, map[string]string{"irp_auth_token": "s3cr3t-token"})

	var gotToken, gotQuery, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-IRP-TS-Token")
		gotQuery = r.URL.Query().Get("access_token")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := &models.HTTPRequestConfig{
		URL:        srv.URL,
		Method:     "POST",
		Headers:    map[string]string{"X-IRP-TS-Token": "{{.irp_auth_token}}"},
		RetryTimes: 1,
		Request: models.RequestDetail{
			Parameters: map[string]string{"access_token": "{{.irp_auth_token}}"},
			Body:       `{"token":"{{.irp_auth_token}}"}`,
		},
	}

	_, err := SendHTTPRequest(cfg, []*models.AlertCurEvent{{RuleName: "cpu high"}},
		map[string]interface{}{}, nil, nil, srv.Client())
	if err != nil {
		t.Fatalf("SendHTTPRequest failed: %v", err)
	}

	if gotToken != "s3cr3t-token" {
		t.Errorf("request header token = %q, want s3cr3t-token", gotToken)
	}
	if gotQuery != "s3cr3t-token" {
		t.Errorf("request query token = %q, want s3cr3t-token", gotQuery)
	}
	if !strings.Contains(gotBody, `"token":"s3cr3t-token"`) {
		t.Errorf("request body = %s, want rendered token", gotBody)
	}
}

// buildNotifyTplData 的内置 key 必须与 models.TplReservedKeys 一致，
// 否则新加的内置 key 可以被同名用户变量顶掉（models 不能 import 本包，只能这样对账）。
func TestBuiltinTplKeysMatchReservedWords(t *testing.T) {
	old := UserVariableGetter
	UserVariableGetter = nil
	t.Cleanup(func() { UserVariableGetter = old })

	reserved := make(map[string]bool, len(models.TplReservedKeys))
	for _, k := range models.TplReservedKeys {
		reserved[k] = true
	}

	builtin := buildNotifyTplData([]*models.AlertCurEvent{{}}, map[string]interface{}{},
		map[string]string{"k": "v"}, []string{"a"})
	for k := range builtin {
		if !reserved[k] {
			t.Errorf("builtin tpl key %q missing from models.TplReservedKeys", k)
		}
	}
	if len(builtin) != len(models.TplReservedKeys) {
		t.Errorf("builtin keys = %d, models.TplReservedKeys = %d; keep them in sync",
			len(builtin), len(models.TplReservedKeys))
	}
}

// 变量值在 URL / 请求头 / 查询参数 / 请求体四处都必须原样送出：
// + 是 base64 的合法字符，被转义成 &#43; 就等于凭证被静默改写、对端认证失败。
// 同一个请求体里的事件字段仍要保持既有转义，否则文案里的引号会打破 JSON。
func TestUserVariableRenderedVerbatimEverywhere(t *testing.T) {
	const raw = `aB+c/d==&e<f`
	withUserVariables(t, map[string]string{"tk": raw})

	cfg := &models.HTTPRequestConfig{
		URL:     "https://x.com/a?t={{.tk}}",
		Headers: map[string]string{"X-Token": "{{.tk}}"},
		Request: models.RequestDetail{
			Parameters: map[string]string{"access_token": "{{.tk}}"},
			Body:       `{"token":"{{.tk}}","rule":"{{$event.RuleName}}"}`,
		},
	}
	event := &models.AlertCurEvent{RuleName: `rule "quote" & <tag>`}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{event}, nil, nil, nil)

	url, headers, parameters := replaceVariables(cfg, tplData)
	if headers["X-Token"] != raw {
		t.Errorf("header = %q, want raw %q", headers["X-Token"], raw)
	}
	if parameters["access_token"] != raw {
		t.Errorf("parameter = %q, want raw %q", parameters["access_token"], raw)
	}
	if !strings.Contains(url, raw) {
		t.Errorf("url = %q, want raw token inside", url)
	}

	body, err := parseRequestBody(cfg, tplData)
	if err != nil {
		t.Fatalf("parseRequestBody failed: %v", err)
	}
	if !strings.Contains(string(body), `"token":"`+raw+`"`) {
		t.Errorf("body = %s, want raw token", body)
	}
	if !strings.Contains(string(body), "&#34;quote&#34;") {
		t.Errorf("body = %s, want event fields still html-escaped", body)
	}
}

func TestRedactErrMsg(t *testing.T) {
	raw := "https://x.com/a?access_token=s3cr3t"
	safe := redactURL(raw)
	err := fmt.Errorf(`Post %q: dial tcp: i/o timeout`, raw)

	got := redactErrMsg(err, raw, safe)
	if strings.Contains(got, "s3cr3t") {
		t.Errorf("redactErrMsg = %q, still leaks the token", got)
	}
	if !strings.Contains(got, "dial tcp: i/o timeout") {
		t.Errorf("redactErrMsg = %q, lost the underlying error", got)
	}
	if redactErrMsg(nil, raw, safe) != "" {
		t.Error("redactErrMsg(nil) should be empty")
	}
}

// 写日志前必须掩掉凭证：key 名看不出来的（塞进名叫 t 的查询参数）也要按模板引用关系掩掉。
func TestRedactForLogHidesVariableValues(t *testing.T) {
	withUserVariables(t, map[string]string{"irp_auth_token": "s3cr3t-token", "irp_env": "prod"})

	cfg := &models.HTTPRequestConfig{
		URL:     "http://x.com/notify?t={{.irp_auth_token}}&env={{.irp_env}}",
		Headers: map[string]string{"X-Custom": "{{.irp_auth_token}}", "Content-Type": "application/json"},
		Request: models.RequestDetail{Parameters: map[string]string{"cb": "{{.irp_auth_token}}"}},
	}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{{}}, nil, nil, nil)
	url, headers, parameters := replaceVariables(cfg, tplData)

	safeURL, safeHeaders, safeParams := redactForLog(cfg, tplData, url, headers, parameters)
	for name, got := range map[string]string{"url": safeURL, "header": safeHeaders["X-Custom"], "param": safeParams["cb"]} {
		if strings.Contains(got, "s3cr3t-token") {
			t.Errorf("%s = %q, still leaks the token", name, got)
		}
	}
	if safeHeaders["Content-Type"] != "application/json" {
		t.Errorf("static header should stay readable, got %q", safeHeaders["Content-Type"])
	}
	// 原始 map 不能被改写，发送用的还是明文
	if headers["X-Custom"] != "s3cr3t-token" {
		t.Errorf("redaction must not mutate the outgoing headers, got %q", headers["X-Custom"])
	}
}

// 引用了不存在的变量：渲染结果保持既有行为（空串、不报错），只额外打一条 Warning。
func TestWarnUndefinedVarsDoesNotChangeRendering(t *testing.T) {
	withUserVariables(t, map[string]string{"defined_token": "ok"})

	cfg := &models.HTTPRequestConfig{
		Headers: map[string]string{"X-A": "{{.defined_token}}", "X-B": "[{{ .typo_token }}]"},
	}
	_, headers, _ := replaceVariables(cfg, buildNotifyTplData([]*models.AlertCurEvent{{}}, nil, nil, nil))

	if headers["X-A"] != "ok" {
		t.Errorf("defined variable = %q, want ok", headers["X-A"])
	}
	if headers["X-B"] != "[]" {
		t.Errorf("undefined variable = %q, want empty render", headers["X-B"])
	}
}

// 存量行为不能被动到：同一次渲染里，事件字段仍按 html/template 转义，只有变量原样输出。
func TestExistingEscapingUntouchedByVariables(t *testing.T) {
	withUserVariables(t, map[string]string{"tk": `a+b&c`})

	cfg := &models.HTTPRequestConfig{
		URL:     `https://x.com/{{$event.RuleName}}?t={{.tk}}`,
		Headers: map[string]string{"X-Rule": `{{$event.RuleName}}`, "X-Token": `{{.tk}}`},
	}
	event := &models.AlertCurEvent{RuleName: `rule "q" & <tag>`}
	url, headers, _ := replaceVariables(cfg, buildNotifyTplData([]*models.AlertCurEvent{event}, nil, nil, nil))

	if want := `rule &#34;q&#34; &amp; &lt;tag&gt;`; headers["X-Rule"] != want {
		t.Errorf("event header = %q, want legacy escaping %q", headers["X-Rule"], want)
	}
	if headers["X-Token"] != `a+b&c` {
		t.Errorf("variable header = %q, want raw", headers["X-Token"])
	}
	if !strings.Contains(url, `&#34;q&#34;`) || !strings.Contains(url, `t=a+b&c`) {
		t.Errorf("url = %q, want escaped event + raw variable", url)
	}
}

// 短变量值不能牵连无关字符：脱敏靠掩码重渲染定位，而不是拿变量值做全文替换。
// irp_env=1 曾把 10.0.0.1 打成 ***0.0.0.***，失败原因就没法看了。
func TestRedactForLogKeepsNonVariableTextIntact(t *testing.T) {
	withUserVariables(t, map[string]string{"irp_host": "10.0.0.1", "irp_env": "1"})

	cfg := &models.HTTPRequestConfig{
		URL:     "http://{{.irp_host}}:8080/notify?env={{.irp_env}}",
		Headers: map[string]string{"X-Trace": "req-11111"},
	}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{{}}, nil, nil, nil)
	url, headers, parameters := replaceVariables(cfg, tplData)

	safeURL, safeHeaders, _ := redactForLog(cfg, tplData, url, headers, parameters)
	if want := "http://***:8080/notify?env=***"; safeURL != want {
		t.Errorf("safeURL = %q, want %q", safeURL, want)
	}
	// 不含模板的字段一个字都不该动，哪怕它正好包含某个变量的值
	if safeHeaders["X-Trace"] != "req-11111" {
		t.Errorf("static header = %q, want req-11111", safeHeaders["X-Trace"])
	}

	err := fmt.Errorf("Post %q: dial tcp 10.0.0.1:8080: connect: connection refused", url)
	msg := redactErrMsg(err, url, safeURL)
	if !strings.Contains(msg, "dial tcp 10.0.0.1:8080: connect: connection refused") {
		t.Errorf("errmsg = %q, want the underlying reason readable", msg)
	}
}

// 引用了内置 key 的字段不该被当成变量掩掉，也不该被当成「未定义变量」告警。
func TestRedactAndWarnSkipBuiltinKeys(t *testing.T) {
	resetUndefinedVarWarnings(t)
	withUserVariables(t, map[string]string{"tk": "s3cr3t-token"})

	cfg := &models.HTTPRequestConfig{
		Headers: map[string]string{"X-To": "{{.sendto}}", "X-Token": "{{.tk}}"},
	}
	// sendtos 为空 -> tplData 里没有 sendto 键，渲染成空串，但它是内置 key 不是用户变量
	tplData := buildNotifyTplData([]*models.AlertCurEvent{{}}, nil, nil, nil)
	_, headers, _ := replaceVariables(cfg, tplData)

	if _, warned := warnedUndefinedVars.Load("{{.sendto}}|sendto"); warned {
		t.Error("builtin key sendto should not be reported as an undefined variable")
	}

	_, safeHeaders, _ := redactForLog(cfg, tplData, "", headers, nil)
	if safeHeaders["X-To"] != "" {
		t.Errorf("builtin-key header = %q, want empty render kept as is", safeHeaders["X-To"])
	}
	if safeHeaders["X-Token"] != "***" {
		t.Errorf("variable header = %q, want ***", safeHeaders["X-Token"])
	}
}

// 未定义变量只告警一次：告警挂在发送路径上，高频规则下不去重就是刷屏。
func TestWarnUndefinedVarsOnlyOnce(t *testing.T) {
	resetUndefinedVarWarnings(t)
	withUserVariables(t, map[string]string{"defined_token": "ok"})

	cfg := &models.HTTPRequestConfig{
		Headers: map[string]string{"X-B": "[{{ .typo_token }}]"},
	}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{{}}, nil, nil, nil)

	for i := 0; i < 3; i++ {
		replaceVariables(cfg, tplData)
	}

	var n int
	warnedUndefinedVars.Range(func(k, _ interface{}) bool {
		n++
		return true
	})
	if n != 1 {
		t.Errorf("warned entries = %d, want exactly 1 (deduped)", n)
	}
	if _, ok := warnedUndefinedVars.Load("[{{ .typo_token }}]|typo_token"); !ok {
		t.Error("dedup key should be (template text, variable name)")
	}
}

// resetUndefinedVarWarnings 清空告警去重表，避免用例之间互相影响。
func resetUndefinedVarWarnings(t *testing.T) {
	t.Helper()
	clear := func() {
		warnedUndefinedVars.Range(func(k, _ interface{}) bool {
			warnedUndefinedVars.Delete(k)
			return true
		})
	}
	clear()
	t.Cleanup(clear)
}

// 非 {{.tk}} 直接引用的写法（管道 / 函数参数）同样要掩掉，
// 否则用顶层引用的正则去定位就会漏掉这一类凭证。
func TestRedactForLogCoversNonTopLevelRefs(t *testing.T) {
	withUserVariables(t, map[string]string{"tk": "s3cr3t-token"})

	cfg := &models.HTTPRequestConfig{
		Headers: map[string]string{"X-Piped": `v={{ with .tk }}{{ . }}{{ end }}`},
	}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{{}}, nil, nil, nil)
	_, headers, _ := replaceVariables(cfg, tplData)

	if !strings.Contains(headers["X-Piped"], "s3cr3t-token") {
		t.Fatalf("precondition failed: header = %q, want the token actually rendered", headers["X-Piped"])
	}

	_, safeHeaders, _ := redactForLog(cfg, tplData, "", headers, nil)
	if strings.Contains(safeHeaders["X-Piped"], "s3cr3t-token") {
		t.Errorf("safe header = %q, still leaks the token", safeHeaders["X-Piped"])
	}
}

// 空值变量不掩：掩了会把日志里的空串显示成 ***，看不出变量其实没配上。
// 同时不引用任何变量的字段不该因为「变量名恰好是模板文本的子串」被多渲染一遍。
func TestRedactForLogSkipsEmptyVariables(t *testing.T) {
	withUserVariables(t, map[string]string{"t": "x", "empty_var": ""})

	cfg := &models.HTTPRequestConfig{
		Headers: map[string]string{
			"X-Rule":  "{{$event.RuleName}}",
			"X-Empty": "[{{.empty_var}}]",
		},
	}
	event := &models.AlertCurEvent{RuleName: "cpu high"}
	tplData := buildNotifyTplData([]*models.AlertCurEvent{event}, nil, nil, nil)
	_, headers, _ := replaceVariables(cfg, tplData)

	_, safeHeaders, _ := redactForLog(cfg, tplData, "", headers, nil)
	if safeHeaders["X-Rule"] != "cpu high" {
		t.Errorf("non-variable field = %q, want cpu high", safeHeaders["X-Rule"])
	}
	if safeHeaders["X-Empty"] != "[]" {
		t.Errorf("empty variable = %q, want [] (masking it hides that it is unset)", safeHeaders["X-Empty"])
	}
}
