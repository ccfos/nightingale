package provider

import (
	"net/url"
	"sort"
	"strings"
)

// 通知记录里的「通知目标」是给人看的：名称、工单号、邮箱、手机号原样显示，只遮像凭证的部分。
// 库里存着各种形态的目标（机器人 token、回调地址、routing key、旧版本写入时只遮了后 4 位的 token），
// 所以展示时按内容判断，而不是按渠道一刀切。

// MaskNotifyTarget 返回可展示的通知目标，逗号分隔的每一项分别处理。
func MaskNotifyTarget(target string) string {
	if target == "" {
		return target
	}
	parts := strings.Split(target, ",")
	for i, p := range parts {
		parts[i] = maskTargetPart(p)
	}
	return strings.Join(parts, ",")
}

// NotifyTargetFromParams 用规则参数生成通知记录的目标：有 bot_name 就用名称；否则按参数名排序逐个列出，
// 凭证类参数只留后 4 位，其余的值再过一遍 MaskNotifyTarget（回调地址里可能带着 token）。
func NotifyTargetFromParams(params map[string]string) string {
	if name := strings.TrimSpace(params["bot_name"]); name != "" {
		return name
	}
	keys := make([]string, 0, len(params))
	for k, v := range params {
		// __ 开头的是内部参数（如测试发送的 __test_nonce），不是发送目标
		if strings.TrimSpace(v) != "" && !strings.HasPrefix(k, "__") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		v := strings.TrimSpace(params[k])
		if isSensitiveKey(k) {
			values = append(values, maskAPIKey(v))
		} else {
			values = append(values, MaskNotifyTarget(v))
		}
	}
	return strings.Join(values, ",")
}

func maskTargetPart(s string) string {
	t := strings.TrimSpace(s)
	switch {
	case t == "":
		return s
	case strings.Contains(t, "://"):
		return maskTargetURL(t)
	case looksLikeSecret(t):
		return maskSecretValue(t)
	}
	return s
}

// maskTargetURL 保留协议、域名和普通路径，遮掉密码、敏感查询参数和像 token 的路径段
// （Discord / Slack / 飞书 webhook 末尾那段、Telegram 的 bot<token>）。
func maskTargetURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return redactedMark
	}
	var b strings.Builder
	b.WriteString(u.Scheme + "://")
	if u.User != nil {
		if name := u.User.Username(); name != "" && !looksLikeSecret(name) {
			b.WriteString(name + "@")
		} else {
			b.WriteString(redactedMark + "@")
		}
	}
	b.WriteString(u.Host)

	segs := strings.Split(u.EscapedPath(), "/")
	for i, seg := range segs {
		if plain, err := url.PathUnescape(seg); err == nil && looksLikeSecret(plain) {
			segs[i] = redactedMark
		}
	}
	b.WriteString(strings.Join(segs, "/"))

	if q := u.Query(); len(q) > 0 {
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			v := q.Get(k)
			if isSensitiveKey(k) || looksLikeSecret(v) {
				v = redactedMark
			} else {
				v = url.QueryEscape(v)
			}
			pairs = append(pairs, url.QueryEscape(k)+"="+v)
		}
		b.WriteString("?" + strings.Join(pairs, "&"))
	}
	return b.String()
}

// looksLikeSecret 判断一个不含空格的串是不是凭证：够长、只由 token 常见字符组成、字母数字都有。
// 含空格、@、#、中文的是名称、邮箱或命令；纯数字的是手机号或频道 ID，都不算。
// 旧版本写入时把 token 的后 4 位换成了 ****，这种长串也按凭证处理。
func looksLikeSecret(s string) bool {
	if len(s) < 16 {
		return false
	}
	letters, digits := false, false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
			letters = true
		case r >= '0' && r <= '9':
			digits = true
		case strings.ContainsRune("-_.:+/=*", r):
		default:
			return false
		}
	}
	if strings.HasSuffix(s, "****") {
		return true
	}
	return len(s) >= 20 && letters && digits
}

func maskSecretValue(s string) string {
	if strings.HasSuffix(s, "****") {
		return redactedMark
	}
	return maskAPIKey(s)
}
