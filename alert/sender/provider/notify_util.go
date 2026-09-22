package provider

// 原生对接媒介（Jira / Discord / Slack / Mattermost）共用的发送工具。
//
// 成功判定、重试分类、Retry-After 与按字符截断的做法移植自 Prometheus Alertmanager
// notify/util.go（Apache License 2.0，https://github.com/prometheus/alertmanager），
// 重试循环保留夜莺「有限次数」的语义，而不是 Alertmanager 那种重试到超时。

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

const (
	defaultNativeRetryTimes = 3
	defaultNativeRetrySleep = time.Second
	// maxRetryAfter 是单次等待的上限：对方要求等太久时宁可按失败记录，也不要把发送 goroutine 挂住
	maxRetryAfter = 30 * time.Second
	// maxResponseBody 限制读入内存的响应体大小，错误详情只需要前面一段
	maxResponseBody = 1 << 20
)

// nativeRetrySettings 取原生媒介配置里的重试次数与间隔，未配置时用默认值。
func nativeRetrySettings(n *models.NativeNetworkConfig) (int, time.Duration) {
	retryTimes, sleep := defaultNativeRetryTimes, defaultNativeRetrySleep
	if n != nil {
		if n.RetryTimes > 0 {
			retryTimes = n.RetryTimes
		}
		if n.RetrySleep > 0 {
			sleep = time.Duration(n.RetrySleep) * time.Millisecond
		}
	}
	return retryTimes, sleep
}

// nativeResponse 是读完并关闭后的响应，供成功判定与错误解析使用
type nativeResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// HTTPStatusError 表示第三方返回了非预期的状态码
type HTTPStatusError struct {
	StatusCode int
	Detail     string
}

func (e *HTTPStatusError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("unexpected status code %d", e.StatusCode)
	}
	return fmt.Sprintf("unexpected status code %d: %s", e.StatusCode, e.Detail)
}

// checkStatus 是默认的成功判定（移植自 Alertmanager Retrier.Check）：2xx 成功；
// 5xx 与 429 可重试；其余立即失败。detail 用于把第三方的报错原文带进错误里。
func checkStatus(r *nativeResponse, detail func(*nativeResponse) string) (bool, error) {
	if r.StatusCode/100 == 2 {
		return false, nil
	}
	retry := r.StatusCode/100 == 5 || r.StatusCode == http.StatusTooManyRequests
	d := ""
	if detail != nil {
		d = detail(r)
	}
	if d == "" {
		d = truncateBody(r.Body)
	}
	return retry, &HTTPStatusError{StatusCode: r.StatusCode, Detail: d}
}

// truncateBody 把响应体压成一行，截到 512 字符，用于错误详情
func truncateBody(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	s, _ = truncateInRunes(s, 512)
	return s
}

// doWithRetry 发送请求并按 check 的结论重试：网络错误与 check 判定可重试的响应会重试，
// 最多 retryTimes 次（不含首次）；对方给了 Retry-After 时按它等待（封顶 maxRetryAfter）。
//
// build 每次都要构造新的 *http.Request：请求体在一次发送后就被读完了。
// 返回值：最后一次拿到的响应（可能为 nil）与最终错误。
func doWithRetry(ctx context.Context, client *http.Client, retryTimes int, sleep time.Duration,
	build func(ctx context.Context) (*http.Request, error),
	check func(*nativeResponse) (bool, error)) (*nativeResponse, error) {

	if client == nil {
		return nil, errors.New("http client not found")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var lastResp *nativeResponse
	var lastErr error
	for attempt := 0; attempt <= retryTimes; attempt++ {
		if attempt > 0 {
			wait := sleep
			if lastResp != nil {
				if d, ok := parseRetryAfter(lastResp.Header.Get("Retry-After"), time.Now()); ok {
					wait = d
				}
			}
			if wait > maxRetryAfter {
				wait = maxRetryAfter
			}
			select {
			case <-ctx.Done():
				return lastResp, fmt.Errorf("%w (canceled after %d attempts: %v)", lastErr, attempt, ctx.Err())
			case <-time.After(wait):
			}
		}

		req, err := build(ctx)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastResp, lastErr = nil, err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		resp.Body.Close()
		lastResp = &nativeResponse{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}

		retry, cerr := check(lastResp)
		if cerr == nil {
			return lastResp, nil
		}
		lastErr = cerr
		if !retry {
			return lastResp, cerr
		}
	}
	return lastResp, lastErr
}

// parseRetryAfter 解析 Retry-After：秒数或 HTTP 日期两种格式。
func parseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs * float64(time.Second)), true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

// truncateInRunes 按字符（而不是字节）截断，截断时以省略号结尾（移植自 Alertmanager）。
func truncateInRunes(s string, n int) (string, bool) {
	if n <= 0 {
		return "", s != ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s, false
	}
	r := []rune(s)
	if n <= 3 {
		return string(r[:n]), true
	}
	return string(r[:n-1]) + "…", true
}

// HintError 在第三方原始报错之外附带一条「可能原因 + 怎么办」的提示。
// Error() 返回英文原文 + 英文提示，落进通知记录；测试接口用 LocalizeError 按请求语言翻译提示。
type HintError struct {
	Err  error
	Hint string // i18n key，英文原文即 key
}

func (e *HintError) Error() string {
	if e.Hint == "" {
		return e.Err.Error()
	}
	return e.Err.Error() + " (" + e.Hint + ")"
}

func (e *HintError) Unwrap() error { return e.Err }

// withHint 给错误附上提示；err 为 nil 或 hint 为空时原样返回
func withHint(err error, hint string) error {
	if err == nil || hint == "" {
		return err
	}
	return &HintError{Err: err, Hint: hint}
}

// LocalizeError 把错误里所有 HintError 的提示按 translate 翻译后拼出完整文本。
// 只替换提示部分，第三方报错原文保持原样，方便用户拿去搜索。
func LocalizeError(err error, translate func(string) string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if translate == nil {
		return msg
	}
	var he *HintError
	for e := err; errors.As(e, &he); e = he.Err {
		if he.Hint != "" {
			msg = strings.Replace(msg, "("+he.Hint+")", "("+translate(he.Hint)+")", 1)
		}
	}
	return msg
}

// expandUserVars 展开值里对「变量配置」的引用（写法 {{.my_token}}），不含 {{ 时原样返回。
// 引用了不存在的变量时报错，而不是静默发送一个空凭证。
func expandUserVars(v string) (string, error) {
	if !strings.Contains(v, "{{") {
		return v, nil
	}
	vars := getUserVariables()
	data := make(map[string]string, len(vars))
	for k, val := range vars {
		data[k] = val
	}
	tpl, err := template.New("uservar").Option("missingkey=error").Parse(v)
	if err != nil {
		return "", fmt.Errorf("invalid variable reference: %v", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		logger.Warningf("notify: failed to expand variable reference: %v", err)
		return "", fmt.Errorf("variable referenced in the channel config is not defined in Variable Settings: %v", err)
	}
	return buf.String(), nil
}

// CheckItem 是「校验凭证」的一项结果
type CheckItem struct {
	Name     string `json:"name"`     // i18n key，英文原文即 key
	OK       bool   `json:"ok"`       // 是否通过
	Required bool   `json:"required"` // 必需项失败才算校验不通过；可选项失败只标红
	Skipped  bool   `json:"skipped"`  // 前置项失败或接口不可用，未执行
	Message  string `json:"message"`  // 结果说明或报错原文
	err      error
}

// CredentialChecker 由支持「校验凭证」的 provider 实现：对一份（可能未保存的）媒介配置
// 逐项检查凭证与权限。只在用户点按钮时调用，可以发网络请求。
type CredentialChecker interface {
	CheckCredential(ctx context.Context, config *models.NotifyChannelConfig, client *http.Client) []CheckItem
}

// LocalizeCheckItems 按 translate 翻译检查项名称与提示，供接口返回前调用
func LocalizeCheckItems(items []CheckItem, translate func(string) string) []CheckItem {
	out := make([]CheckItem, len(items))
	for i, it := range items {
		it.Name = translate(it.Name)
		if it.err != nil {
			it.Message = LocalizeError(it.err, translate)
		} else if it.Message != "" {
			it.Message = translate(it.Message)
		}
		out[i] = it
	}
	return out
}

// maskWebhookURL 返回可写进通知记录和日志的 Webhook 地址：路径最后一段（凭证所在）换成 ***，
// 查询串整个去掉。解析失败时返回占位符，不把疑似凭证的原串写出去。
func maskWebhookURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "(invalid webhook url)"
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	if i := strings.LastIndex(path, "/"); i >= 0 && i < len(path)-1 {
		path = path[:i+1] + redactedMark
	}
	// 手工拼接：url.URL.String() 会把 *** 转义成 %2A%2A%2A
	return u.Scheme + "://" + u.Host + path
}

// webhookTarget 是 Webhook 类通知在通知记录里的目标：优先用规则里填的名称，没填时用掩码后的地址。
// provider 自填的 Target 会原样落进 notification_record，而 Webhook 地址本身就是凭证。
func webhookTarget(name, webhookURL string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	return maskWebhookURL(webhookURL)
}

// severityColor 是 Webhook 类消息卡片的级别色：S1 红 / S2 橙 / S3 黄 / 恢复绿
func severityColor(severity int, recovered bool) int {
	if recovered {
		return 0x2EB67D
	}
	switch severity {
	case 1:
		return 0xE01E5A
	case 2:
		return 0xF2994A
	default:
		return 0xECB22E
	}
}

// eventTitle 是卡片标题的兜底格式（模板没有 title 字段时用）
func eventTitle(event *models.AlertCurEvent) string {
	status := "Triggered"
	if event.IsRecovered {
		status = "Recovered"
	}
	return fmt.Sprintf("[S%d] %s: %s", event.Severity, status, event.RuleName)
}

// eventDetailURL 返回事件详情页链接，站点地址为空时返回空串
func eventDetailURL(siteURL string, event *models.AlertCurEvent) string {
	if siteURL == "" || event.Id == 0 {
		return ""
	}
	return strings.TrimRight(siteURL, "/") + "/share/alert-his-events/" + strconv.FormatInt(event.Id, 10)
}
