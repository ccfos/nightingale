package provider

import (
	htmltemplate "html/template"
	"net/url"
	"regexp"
	"strings"
	"sync"

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

// scanTopLevelRefs 返回模板里所有 {{.xxx}} 形式的顶层引用名，按出现顺序去重。
func scanTopLevelRefs(tplStr string) []string {
	ms := topLevelRefRe.FindAllStringSubmatch(tplStr, -1)
	if len(ms) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(ms))
	refs := make([]string, 0, len(ms))
	for _, m := range ms {
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		refs = append(refs, m[1])
	}
	return refs
}

var reservedTplKeys = func() map[string]struct{} {
	m := make(map[string]struct{}, len(models.TplReservedKeys))
	for _, k := range models.TplReservedKeys {
		m[k] = struct{}{}
	}
	return m
}()

// reservedTplKey 判断某个顶层引用是不是内置 key。内置 key 由 buildNotifyTplData 提供，
// 不属于「变量配置」，脱敏与告警都要绕开它们。
func reservedTplKey(name string) bool {
	_, ok := reservedTplKeys[name]
	return ok
}

// warnedUndefinedVars 记录已经告警过的 (模板原文, 变量名) 组合。
// 告警挂在发送路径上，不去重的话一条写错的媒介配置会被高频规则刷成日志噪音。
// key 用模板原文而不是字段名：Authorization 这类字段名在不同媒介间会重名，
// 按原文区分才不会把另一个媒介的问题一起吞掉。进程重启后重新告警一次，正合适。
var warnedUndefinedVars sync.Map

// warnUndefinedVars 在模板引用了不存在的变量时打一条 Warning，同一处只打一次。
//
// 纯日志，不改渲染结果：html/template 对缺失的 key 一律渲染成空串，不报错也不留痕，
// 而渲染出来的值又会被日志脱敏成 ***，变量名写错时现象就只剩「对端认证失败」。
func warnUndefinedVars(name, tplStr string, tplData map[string]interface{}) {
	for _, ref := range scanTopLevelRefs(tplStr) {
		// sendto / event 在没有收件人或事件时本就不写进上下文，它们是内置 key 而非用户变量，
		// 报出来只会把排查方向引到「变量配置」页面上
		if _, ok := tplData[ref]; ok || reservedTplKey(ref) {
			continue
		}
		if _, loaded := warnedUndefinedVars.LoadOrStore(tplStr+"|"+ref, struct{}{}); loaded {
			continue
		}
		logger.Warningf("notify http config %q references undefined variable %q, "+
			"it renders as empty; check 变量配置 (system variable settings)", name, ref)
	}
}

// redactedMark 是日志里代替凭证的占位符。
const redactedMark = "***"

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

// redactField 返回该字段可安全写日志的渲染结果：变量填进去的那几段是 ***，其余原样。
//
// 做法是把模板用「变量值全换成 ***」的上下文重渲染一遍，而不是拿变量值去渲染结果里做
// 字符串替换——后者认不出哪一段是变量填的，irp_env=1 这种短值会把 10.0.0.1 打成
// ***0.0.0.***，日志和通知记录里的失败原因就没法看了。
//
// 上下文里的变量一律掩掉，不去解析「这个字段到底引用了哪几个」：没被引用的变量本就不会
// 出现在渲染结果里，多掩不改变输出；而少掩一个就是漏一个凭证——{{.tk | printf "%s"}}
// 这类写法用顶层引用的正则是认不出来的。needsTemplateRendering 与下面的名字命中只用来
// 决定「要不要多渲染这一次」，不含变量的存量配置因此零额外开销。
func redactField(name, tplStr, rendered string, tplData map[string]interface{},
	vars map[string]string) string {

	if rendered == "" || len(vars) == 0 || !needsTemplateRendering(tplStr) {
		return rendered
	}

	masked := make(map[string]interface{}, len(tplData))
	for k, v := range tplData {
		masked[k] = v
	}

	mentioned := false
	for k, v := range vars {
		// 空值变量没有可掩的东西，掩了反而把日志里的空串显示成 ***，
		// 看不出这个变量其实压根没配上；撞名内置 key 的存量变量在渲染时取的是内置值
		// （见 buildNotifyTplData），掩掉它只会把日志里的事件字段变成 ***
		if v == "" || reservedTplKey(k) {
			continue
		}
		// 认 ".name" 而不是裸变量名：裸名会被 {{$event.RuleName}} 这类文本里的 t / e
		// 顺带命中，白多渲染一遍（结果一样，纯浪费）
		if strings.Contains(tplStr, "."+k) {
			mentioned = true
		}
		masked[k] = htmltemplate.HTML(redactedMark)
	}
	if !mentioned {
		return rendered
	}

	return getParsedHTTPString(name, tplStr, masked)
}

// redactForLog 返回可安全写日志的 url / 请求头 / 查询参数：先按 key 名掩掉常见凭证字段，
// 再把变量填进去的片段掩成 ***。原始 map 不动，发送用的仍是明文。
//
// tplData 是本次发送真实用的渲染上下文，重渲染必须用同一份，否则掩码结果和实际发出去的
// 内容对不上，日志会误导排查。
func redactForLog(httpConfig *models.HTTPRequestConfig, tplData map[string]interface{},
	url string, headers, parameters map[string]string) (string, map[string]string, map[string]string) {

	// 变量表每次调用都是一份新拷贝，这里取一次传下去，别在每个 header / param 上各拷一遍
	vars := getUserVariables()

	safeURL := redactURL(redactField("url", httpConfig.URL, url, tplData, vars))

	safeHeaders := redactKV(headers)
	for k, v := range safeHeaders {
		if v == redactedMark {
			continue
		}
		safeHeaders[k] = redactField(k, httpConfig.Headers[k], v, tplData, vars)
	}

	safeParams := redactKV(parameters)
	for k, v := range safeParams {
		if v == redactedMark {
			continue
		}
		safeParams[k] = redactField(k, httpConfig.Request.Parameters[k], v, tplData, vars)
	}

	return safeURL, safeHeaders, safeParams
}

// redactKV 返回可安全写日志的副本：命中敏感 key 的值替换成 ***，原 map 不动。
// 渲染后的请求头/查询参数里可能带着变量配置中的凭证，而这行日志是 Info 级别、默认落盘。
func redactKV(kv map[string]string) map[string]string {
	out := make(map[string]string, len(kv))
	for k, v := range kv {
		if v != "" && isSensitiveKey(k) {
			out[k] = redactedMark
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
				q.Set(k, redactedMark)
				changed = true
			}
		}
		if changed {
			u.RawQuery = q.Encode()
		}
	}

	return u.String()
}

// redactErrMsg 把错误文本里的原始 URL 换成 redactForLog 已经算好的脱敏版本。
// net/http 创建请求 / 发送失败时的 error 会内嵌完整 URL，而这些文本不只写日志，
// 还会作为 NotifyResult.Response 落进 notify_record 并显示在事件通知记录页面上，
// URL 查询串里若带着变量渲染出来的凭证就等于换个地方泄露。
//
// 直接复用 safeURL 而不是在这里重算：掩码是靠重渲染模板得到的，两边必须是同一个结果，
// 也省掉一次按变量值做全文替换（那会把错误文本里的无关字符也一起打掉）。
func redactErrMsg(err error, rawURL, safeURL string) string {
	if err == nil {
		return ""
	}

	msg := err.Error()
	if rawURL != "" && safeURL != "" && rawURL != safeURL {
		msg = strings.ReplaceAll(msg, rawURL, safeURL)
	}
	return msg
}
