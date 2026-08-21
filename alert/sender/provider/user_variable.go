package provider

import (
	htmltemplate "html/template"
	"net/url"
	"regexp"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// UserVariableGetter 返回「系统配置 - 变量配置」里已解密的用户变量 (ckey -> value)，
// 供通知媒介的 URL / 请求头 / 查询参数 / 请求体模板引用，写法与内置变量一致：{{.my_token}}。
//
// 由启动方注入 memsto.ConfigCache.Get（见 alert.Start），provider 包因此不必反向依赖 memsto。
// 之所以用包级钩子而不是往 NotifyRequest 里加字段：那样要连带改 BuildNotifyContext 与
// SendNotifyRuleMessage 的签名，而后者由包级函数指针 dispatch.SendByNotifyRule 引用、
// n9e-plus 侧有自己的实现，签名一动就是双仓联动。同款先例：models.VerifyByProvider。
//
// 未注入时（单测、未启动告警引擎的进程）返回 nil，渲染结果与改造前完全一致。
var UserVariableGetter func() map[string]string

// getUserVariables 取用户变量快照，钩子未注入时返回 nil。
func getUserVariables() map[string]string {
	if UserVariableGetter == nil {
		return nil
	}
	return UserVariableGetter()
}

// buildNotifyTplData 组装通知模板的渲染上下文，HTTP / 短信 / 语音各 Provider 共用。
//
// 用户变量先铺底、内置 key 后写：变量名与内置 key 撞车时以内置语义为准，避免一个叫
// event 的变量把事件对象顶掉。新建变量时 models.userVariableCheck 已把这些名字列为
// 保留字，这里是针对存量数据（早于该校验写入）的第二道防线。
func buildNotifyTplData(events []*models.AlertCurEvent, tpl map[string]interface{},
	params map[string]string, sendtos []string) map[string]interface{} {

	fullTpl := make(map[string]interface{})

	// 变量值按 template.HTML 注入：请求体走的是 html/template，普通字符串会被转义成
	// &#43; &amp; 之类的实体——+ 是 base64 的合法字符，凭证一旦含 + 就被静默改写。
	// 只放开变量自己，events / params / tpl 仍按原样转义：请求体多为 JSON，
	// 事件文案里的引号正是靠那层转义才没把 JSON 打破（见 http_common_test.go 的既有用例）。
	for k, v := range getUserVariables() {
		fullTpl[k] = htmltemplate.HTML(v)
	}

	fullTpl["sendtos"] = sendtos // 发送对象
	fullTpl["params"] = params   // 自定义参数
	fullTpl["tpl"] = tpl
	fullTpl["events"] = events

	if len(events) > 0 {
		fullTpl["event"] = events[0]
	}

	if len(sendtos) > 0 {
		fullTpl["sendto"] = sendtos[0]
	}

	return fullTpl
}

// topLevelRefRe 匹配模板里对顶层字段的直接引用，如 {{.my_token}} / {{ .my_token }}。
// 只认这一种写法：变量就是这样引用的，$event.Xxx / $params.Xxx 走的是另一套前缀。
var topLevelRefRe = regexp.MustCompile(`{{-?\s*\.([a-zA-Z_][a-zA-Z0-9_]*)\s*-?}}`)

// warnUndefinedVars 在模板引用了不存在的变量时打一条 Warning。
//
// 纯日志，不改渲染结果：html/template 对缺失的 key 一律渲染成空串，不报错也不留痕，
// 而渲染出来的值又会被日志脱敏成 ***，变量名写错时现象就只剩「对端认证失败」。
func warnUndefinedVars(name, tplStr string, tplData map[string]interface{}) {
	for _, m := range topLevelRefRe.FindAllStringSubmatch(tplStr, -1) {
		if _, ok := tplData[m[1]]; !ok {
			logger.Warningf("notify http config %q references undefined variable %q, "+
				"it renders as empty; check 变量配置 (system variable settings)", name, m[1])
		}
	}
}

// sensitiveKeyHints 命中即认为该请求头 / 查询参数携带凭证，写日志时只打掩码。
// 宁可多掩一个（日志里少一段可读信息）也不要把 token 落盘。
var sensitiveKeyHints = []string{
	"auth", "token", "secret", "password", "passwd",
	"cookie", "key", "sign", "credential",
}

func isSensitiveKey(k string) bool {
	lower := strings.ToLower(k)
	for _, hint := range sensitiveKeyHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

// redactVarValues 掩掉渲染结果里的用户变量值。
//
// 只按 key 名认凭证是不够的：变量名由用户自己起（irp_auth_token 可能被塞进名叫 t 的
// 查询参数里），光看 key 名认不出来。这里反过来，用「模板里引用了哪个变量」定位：
// 模板文本里出现过变量名，就把该变量的值从渲染结果里抹掉，不会误伤无关字符串。
func redactVarValues(rendered, tplText string) string {
	if rendered == "" || tplText == "" {
		return rendered
	}

	for k, v := range getUserVariables() {
		if v == "" || !strings.Contains(tplText, k) {
			continue
		}
		rendered = strings.ReplaceAll(rendered, v, "***")
	}
	return rendered
}

// redactForLog 返回可安全写日志的 url / 请求头 / 查询参数：先按 key 名掩掉常见凭证字段，
// 再按模板引用关系掩掉用户变量值。原始 map 不动，发送用的仍是明文。
func redactForLog(httpConfig *models.HTTPRequestConfig, url string,
	headers, parameters map[string]string) (string, map[string]string, map[string]string) {

	safeURL := redactVarValues(redactURL(url), httpConfig.URL)

	safeHeaders := redactKV(headers)
	for k, v := range safeHeaders {
		safeHeaders[k] = redactVarValues(v, httpConfig.Headers[k])
	}

	safeParams := redactKV(parameters)
	for k, v := range safeParams {
		safeParams[k] = redactVarValues(v, httpConfig.Request.Parameters[k])
	}

	return safeURL, safeHeaders, safeParams
}

// redactKV 返回可安全写日志的副本：命中敏感 key 的值替换成 ***，原 map 不动。
// 渲染后的请求头/查询参数里可能带着变量配置中的凭证，而这行日志是 Info 级别、默认落盘。
func redactKV(kv map[string]string) map[string]string {
	if len(kv) == 0 {
		return kv
	}

	out := make(map[string]string, len(kv))
	for k, v := range kv {
		if v != "" && isSensitiveKey(k) {
			out[k] = "***"
			continue
		}
		out[k] = v
	}
	return out
}

// redactURL 掩掉 URL 里的 userinfo 与敏感查询参数值，用于日志输出。
// 解析失败时不猜测结构，直接返回占位符，避免把疑似凭证的原串打出去。
func redactURL(raw string) string {
	if raw == "" {
		return raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "(unparsable url)"
	}

	if u.User != nil {
		u.User = url.User(u.User.Username())
	}

	if q := u.Query(); len(q) > 0 {
		changed := false
		for k := range q {
			if isSensitiveKey(k) {
				q.Set(k, "***")
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}

	return u.String()
}

// redactErrMsg 把错误文本里的原始 URL 换成脱敏版本。
// net/http 创建请求 / 发送失败时的 error 会内嵌完整 URL，而这些文本不只写日志，
// 还会作为 NotifyResult.Response 落进 notify_record 并显示在事件通知记录页面上，
// URL 查询串里若带着变量渲染出来的凭证就等于换个地方泄露。
func redactErrMsg(err error, rawURL, urlTpl string) string {
	if err == nil {
		return ""
	}

	msg := err.Error()
	if rawURL != "" {
		msg = strings.ReplaceAll(msg, rawURL, redactURL(rawURL))
	}
	return redactVarValues(msg, urlTpl)
}
