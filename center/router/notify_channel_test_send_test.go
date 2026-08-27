package router

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// 「测试尚未保存的媒介配置」整条链路的护栏测试。
//
// 它同时钉住三件容易在重构中悄悄失效的事：
//  1. sendToNotifyChannel 能被一个 ID 为 0、从未落库的 config 驱动（不查库、不走 ID 缓存）；
//  2. 传进去的 tplContent 必须是「已渲染」的正文——传模板源码进来会把 {{$event.RuleName}}
//     原样发给第三方，而这种错误在真实群消息里才看得见，单测不看就漏了；
//  3. 自定义参数（如钉钉的 access_token）能经 NotifyConfig.Params 抵达出站请求。
//
// 未配 user_ids/user_group_ids 时 GetNotifyConfigParams 会提前返回，不触碰用户缓存，
// 因此这里可以安全地传 nil cache。
func TestSendToNotifyChannelWithUnsavedConfig(t *testing.T) {
	var gotBody string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	// 未保存的配置：ID 保持零值，形态与前端「保存前测试」提交上来的一致
	nc := &models.NotifyChannelConfig{
		Name:        "unsaved-dingtalk",
		Ident:       "dingtalk",
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				URL:     srv.URL,
				Method:  "POST",
				Timeout: 5000,
				Headers: map[string]string{"Content-Type": "application/json"},
				Request: models.RequestDetail{
					Parameters: map[string]string{"access_token": "{{$params.access_token}}"},
					Body:       `{"msgtype":"markdown","markdown":{"title":"{{$tpl.title}}","text":"{{$tpl.content}}"}}`,
				},
			},
		},
	}
	if nc.ID != 0 {
		t.Fatalf("precondition failed: ID = %d, want 0", nc.ID)
	}

	events := []*models.AlertCurEvent{
		buildChannelTestMockEvent("en_US", MockEventForm{MockSeverity: 1}),
	}

	// 与 handler 完全相同的渲染步骤：内联模板源码 -> RenderEvent -> 已渲染正文
	tpl := &models.MessageTemplate{Content: map[string]string{
		"title":   "{{$event.RuleName}}",
		"content": "severity=S{{$event.Severity}} value={{$event.TriggerValue}}",
	}}
	siteUrl := "http://127.0.0.1:17000"
	tplContent := tpl.RenderEvent(events, siteUrl)

	notifyConfig := models.NotifyConfig{
		Params: map[string]interface{}{"access_token": "tok-12345"},
	}

	c := ctx.NewContext(context.Background(), nil, true)

	if _, err := sendToNotifyChannel(c, nil, nil, notifyConfig, nc, events, tplContent, siteUrl); err != nil {
		t.Fatalf("sendToNotifyChannel on unsaved config failed: %v", err)
	}

	if gotBody == "" {
		t.Fatal("webhook received no request")
	}

	// 出站正文必须是渲染后的结果，不能残留任何模板占位符
	if strings.Contains(gotBody, "{{") {
		t.Fatalf("unrendered template placeholder leaked to the wire: %s", gotBody)
	}

	var payload struct {
		Markdown struct {
			Title string `json:"title"`
			Text  string `json:"text"`
		} `json:"markdown"`
	}
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("body is not valid json: %v, body=%s", err, gotBody)
	}
	if payload.Markdown.Title != events[0].RuleName {
		t.Fatalf("title = %q, want rendered rule name %q", payload.Markdown.Title, events[0].RuleName)
	}
	if want := "severity=S1 value=81.5"; payload.Markdown.Text != want {
		t.Fatalf("text = %q, want %q", payload.Markdown.Text, want)
	}

	// 自定义参数要能抵达出站请求（钉钉靠它带 access_token）
	if !strings.Contains(gotQuery, "access_token=tok-12345") {
		t.Fatalf("query = %q, want access_token=tok-12345", gotQuery)
	}
}

// 「body 直接透传 $events、不引用 $tpl」的媒介（内置 callback 就是这种）必须能在
// 完全没有模板内容的情况下走通投递。
//
// 这条路径此前被 notifyChannelConfigTest 里的 "tpl_content required" 闸门挡死：
// 前端从 body 推不出字段名 -> 不传 tpl_content -> 后端 400，表现为默认媒介的
// 「测试」按钮永远报错。生产链路上 callback 走的是种子模板 {"content": ""}，
// 与这里传空 map 等价，所以放行是与生产对齐而不是放松校验。
func TestSendToNotifyChannelWithEmptyTplContent(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	nc := &models.NotifyChannelConfig{
		Name:        "unsaved-callback",
		Ident:       "callback",
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				URL:     srv.URL,
				Method:  "POST",
				Timeout: 5000,
				Headers: map[string]string{"Content-Type": "application/json"},
				Request: models.RequestDetail{Body: `{{ jsonMarshal $events }}`},
			},
		},
	}

	events := []*models.AlertCurEvent{
		buildChannelTestMockEvent("en_US", MockEventForm{MockSeverity: 1}),
	}
	siteUrl := "http://127.0.0.1:17000"
	c := ctx.NewContext(context.Background(), nil, true)

	// 空模板：与 handler 在 f.TplContent 为空时传入的完全一致
	tplContent := make(map[string]interface{})

	if _, err := sendToNotifyChannel(c, nil, nil, models.NotifyConfig{}, nc, events, tplContent, siteUrl); err != nil {
		t.Fatalf("callback-style config with empty tpl failed: %v", err)
	}
	if !strings.Contains(gotBody, events[0].RuleName) {
		t.Fatalf("event payload missing from body: %s", gotBody)
	}
}

// buildTestTplContent 是 notifyChannelConfigTest 的渲染步骤。它的两个不变量——
// 按媒介 ident 选渲染分支、模板出错立刻失败——都不会在投递链路上暴露出来：
// 分支选错只是内容形态不同，出错被吞掉更是照样返回 success=true，
// 两者都只有在用户真的收到消息时才看得见，所以只能在这里钉住。
func TestBuildTestTplContent(t *testing.T) {
	events := []*models.AlertCurEvent{
		buildChannelTestMockEvent("en_US", MockEventForm{MockSeverity: 1}),
	}
	const siteUrl = "http://site"
	// 正文里同时有换行和引号：默认分支会转义，email 分支必须原样保留
	body := map[string]string{"content": "line1\nsay \"hi\""}

	t.Run("smtp 媒介按 ident 走 email 分支，换行与引号不被转义", func(t *testing.T) {
		nc := &models.NotifyChannelConfig{RequestType: "smtp", Ident: "email"}
		got, err := buildTestTplContent(nc, body, events, siteUrl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := "line1\nsay \"hi\""; fmt.Sprint(got["content"]) != want {
			t.Fatalf("got %q, want %q —— ident 没传到 MessageTemplate 上", fmt.Sprint(got["content"]), want)
		}
	})

	t.Run("普通 http 媒介仍走默认分支的 JSON 转义", func(t *testing.T) {
		nc := &models.NotifyChannelConfig{RequestType: "http", Ident: "dingtalk"}
		got, err := buildTestTplContent(nc, body, events, siteUrl)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := `line1\nsay \"hi\"`; fmt.Sprint(got["content"]) != want {
			t.Fatalf("got %q, want %q", fmt.Sprint(got["content"]), want)
		}
	})

	// 模板写错必须在投递之前失败。若退回 RenderEvent，这里会拿到一段
	// "failed to parse template: ..." 当正文、err 为 nil，接口随后报 success=true
	t.Run("模板写错立即失败，不产出正文", func(t *testing.T) {
		nc := &models.NotifyChannelConfig{RequestType: "http", Ident: "dingtalk"}
		got, err := buildTestTplContent(nc, map[string]string{"content": "{{$nope}}"}, events, siteUrl)
		if err == nil {
			t.Fatalf("模板写错却没报错，渲染结果=%v", got)
		}
		if got != nil {
			t.Fatalf("出错时不应产出正文: %v", got)
		}
		if !strings.Contains(err.Error(), "undefined variable") {
			t.Fatalf("错误信息应保留 Go 模板的原因，got %q", err.Error())
		}
	})

	// 空模板与不走模板的媒介都是合法形态，返回空 map 而不是报错
	t.Run("空模板返回空 map", func(t *testing.T) {
		nc := &models.NotifyChannelConfig{RequestType: "http", Ident: "callback"}
		got, err := buildTestTplContent(nc, nil, events, siteUrl)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
	})

	t.Run("flashduty 不走模板，即使传了内容也返回空 map", func(t *testing.T) {
		nc := &models.NotifyChannelConfig{RequestType: "flashduty", Ident: "flashduty"}
		got, err := buildTestTplContent(nc, body, events, siteUrl)
		if err != nil || len(got) != 0 {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
}

// 前端为新建模板 / 媒介测试自动生成的起步内容里带 {{$.domain}}。
// 这类模板错误 RenderEvent 是不会往外报的——它把解析错误当正文返回，
// 第三方收到 "failed to parse template: ..." 而接口仍报成功。这里守住整条链路。
func TestRenderedStarterContentHasNoTemplateError(t *testing.T) {
	events := []*models.AlertCurEvent{
		buildChannelTestMockEvent("en_US", MockEventForm{MockSeverity: 1}),
	}
	// 与 src/pages/notificationTemplates/utils/tplKeys.ts 的 buildBody 保持一致
	starter := map[string]string{
		"title":   "[S{{$event.Severity}}] {{$event.RuleName}}",
		"content": "Status: {{if $event.IsRecovered}}Recovered{{else}}Firing{{end}}\nDetail: {{$.domain}}/alert-his-events/{{$event.Id}}",
	}

	tpl := &models.MessageTemplate{Content: starter}
	rendered := tpl.RenderEvent(events, "http://site")

	for key, val := range rendered {
		s := fmt.Sprint(val)
		if strings.Contains(s, "failed to parse template") || strings.Contains(s, "failed to execute template") {
			t.Fatalf("starter content %q rendered into a template error: %s", key, s)
		}
		if strings.Contains(s, "{{") {
			t.Fatalf("starter content %q left an unrendered placeholder: %s", key, s)
		}
	}
	// 连事件 ID 一起断言：只匹配前缀的话，站点地址后面被转义/截断也照样通过
	if want := "http://site/alert-his-events/" + fmt.Sprint(events[0].Id); !strings.Contains(fmt.Sprint(rendered["content"]), want) {
		t.Fatalf("$.domain not substituted, want %q in: %s", want, rendered["content"])
	}
}
