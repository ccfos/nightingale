package chat

import (
	"regexp"
	"strings"
)

// general_chat 路径的反幻觉硬约束。LLM 在 general_chat 上凭训练记忆生成时,
// 会把相邻产品 (Telegraf / node_exporter / Grafana / AlertManager) 的"行业
// 惯例"误用到 n9e 上 (Severity=Critical / Authorization: Bearer /
// [[inputs.xxx]] / :9100/metrics 等), 因为训练数据里 categraf/n9e 真实事实
// 占比远低于这些相邻产品。
//
// ValidateRestrictedGCOutput 对 LLM 最终输出跑正则扫描, 命中 ForbiddenPatterns
// 就让 router 用 BuildRestrictedRefusalResponse 整体替换为拒答模板, 保证错误
// 事实不会落到持久化和 history 里。

// ForbiddenPatterns: 限制版输出的禁止模式列表。
//
// 和 landmines.yaml 定位区分:
//   - landmines.yaml 给 verify_answer 工具读, 覆盖 doc-qa skill 路径
//   - ForbiddenPatterns 给 general_chat 后置校验用, 命中即整体替换为拒答模板
var ForbiddenPatterns = []*regexp.Regexp{
	// === Severity 命名 (n9e 用 Emergency/Warning/Notice, 不是 Critical/Info) ===
	regexp.MustCompile(`(?i)severity\s*[:=]?\s*1\s*[:=]?\s*critical\b`),
	regexp.MustCompile(`(?i)severity\s*[:=]?\s*3\s*[:=]?\s*info\b`),
	regexp.MustCompile(`(?i)\b1\s*[:=]\s*critical\b`),
	regexp.MustCompile(`(?i)\b3\s*[:=]\s*info\b`),

	// === 鉴权 Header (n9e Web API 用 X-User-Token, 不是 Bearer) ===
	// 限制版 GC 模式下 — 因为没有 search 兜底拿到正确 Header — 干脆禁止
	// 提任何认证 Header, 让 LLM 走拒答模板。
	regexp.MustCompile(`(?i)(?:夜莺|n9e|nightingale|flashcat)[\s\S]{0,150}Authorization\s*:\s*Bearer`),

	// === categraf 编造字段 ===
	regexp.MustCompile(`\b(enable_self_metrics|prometheus_exporter_port)\s*=`),
	regexp.MustCompile(`\b(enable_queue|queue_max_size_bytes|queue_pop_timeout)\s*=`),
	regexp.MustCompile(`\bqueue_full_policy\s*=`),

	// === Telegraf 风格 inputs.xxx (categraf 用 [[instances]]) ===
	regexp.MustCompile(`\[\[inputs\.[a-zA-Z_][a-zA-Z0-9_]*\]\]`),

	// === 编造的 ping 指标名 ===
	regexp.MustCompile(`\b(ping_result_code|ping_duration_seconds|ping_packet_loss_ratio)\b`),

	// === 编造的 categraf 环境变量 ===
	regexp.MustCompile(`\bN9E_ADDR\b`),
	regexp.MustCompile(`\bCATEGRAF_(?:GLOBAL_LABELS|HOSTNAME|WRITERS)_[A-Z_]*\b`),

	// === 编造的 categraf websocket 插件 / 指标 ===
	regexp.MustCompile(`\binput\.websocket\b`),
	regexp.MustCompile(`\bwebsocket_(?:connect_duration_ms|status\b|error_count)\b`),

	// === 编造的 n9e API 路径 ===
	regexp.MustCompile(`/api/n9e/target-bindings\b|/api/n9e/target/bindings\b`),
}

// ValidateRestrictedGCOutput 跑后置校验。返回 (clean, hits):
//   - clean == true: 输出干净, 可以直接发送给用户
//   - clean == false: 命中至少 1 条 ForbiddenPattern, 调用方应丢弃 LLM 输出
//     用 BuildRestrictedRefusalResponse 生成的拒答模板替代
//
// hits 包含命中的具体字符串, 用于日志 / metrics / eval 调试。
func ValidateRestrictedGCOutput(answer string) (clean bool, hits []string) {
	for _, p := range ForbiddenPatterns {
		if matches := p.FindAllString(answer, -1); len(matches) > 0 {
			hits = append(hits, matches...)
		}
	}
	return len(hits) == 0, hits
}

// BuildRestrictedRefusalResponse 构造拒答模板。
//
// 后置校验失败时用这个替换 LLM 输出, 保证用户拿到的不是错误事实而是
// 引导查文档的提示。可选 hints 参数允许调用方注入"召回到的相关 chunk
// 摘要"作为概念引导, 让拒答不至于完全无用。
func BuildRestrictedRefusalResponse(userQuery string, conceptualHints ...string) string {
	var sb strings.Builder
	sb.WriteString("我在 n9e/categraf 官方文档里没找到关于这个问题的明确描述, ")
	sb.WriteString("不能凭记忆给出具体字段名/指标名/API 路径以免误导。\n\n")
	sb.WriteString("建议:\n\n")
	sb.WriteString("1. 📖 到 [V9 文档站](https://flashcat.cloud/docs/) 手动搜或切换版本查询\n")
	sb.WriteString("2. 🐛 到 [GitHub Issues](https://github.com/ccfos/nightingale/issues) 搜历史问答\n")
	sb.WriteString("3. 💬 加[夜莺社区群](https://flashcat.cloud/community/)直接问研发\n")

	for _, h := range conceptualHints {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		sb.WriteString("\n---\n\n")
		sb.WriteString(h)
		sb.WriteString("\n")
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("如需准确的 n9e 配置细节，请查阅官方文档或加群咨询。\n")
	return sb.String()
}
