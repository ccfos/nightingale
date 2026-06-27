package chat

import (
	"regexp"
	"strings"
)

// general_chat 后置校验的禁词正则。LLM 凭记忆答 n9e 问题时会把相邻产品
// (Telegraf/node_exporter 等, 训练数据里占比远高于 categraf/n9e) 的惯例
// 误用到 n9e 上, prompt 防不住, 这里硬扫。
// landmines.yaml 给 doc-qa skill 的 verify_answer 工具用, 这里给 general_chat 用。
var ForbiddenPatterns = []*regexp.Regexp{
	// === Severity 命名 (n9e 用 Emergency/Warning/Notice, 不是 Critical/Info) ===
	// 字符类纳入中文全角冒号 ：, 避免 "Severity 1：Critical" 漏判。
	regexp.MustCompile(`(?i)severity\s*[:：=]?\s*1\s*[:：=]?\s*critical\b`),
	regexp.MustCompile(`(?i)severity\s*[:：=]?\s*3\s*[:：=]?\s*info\b`),
	regexp.MustCompile(`(?i)\b1\s*[:：=]\s*critical\b`),
	regexp.MustCompile(`(?i)\b3\s*[:：=]\s*info\b`),

	// === 鉴权 Header (n9e Web API 用 X-User-Token, 不是 Bearer) ===
	regexp.MustCompile(`(?i)(?:夜莺|n9e|nightingale|flashcat)[\s\S]{0,150}Authorization\s*:\s*Bearer`),

	// === categraf 编造字段 ===
	regexp.MustCompile(`\b(enable_self_metrics|prometheus_exporter_port)\s*=`),
	regexp.MustCompile(`\b(enable_queue|queue_max_size_bytes|queue_pop_timeout)\s*=`),
	regexp.MustCompile(`\bqueue_full_policy\s*=`),

	// === Telegraf 风格 inputs.xxx (categraf 用 [[instances]]) ===
	regexp.MustCompile(`\[\[inputs\.[a-zA-Z_][a-zA-Z0-9_]*\]\]`),

	// === 编造的 ping 指标名 (ping_result_code 是真实指标, 不能拉黑) ===
	regexp.MustCompile(`\b(ping_duration_seconds|ping_packet_loss_ratio)\b`),

	// === 编造的 categraf 环境变量 ===
	regexp.MustCompile(`\bN9E_ADDR\b`),
	regexp.MustCompile(`\bCATEGRAF_(?:GLOBAL_LABELS|HOSTNAME|WRITERS)_[A-Z_]*\b`),

	// === 编造的 categraf websocket 插件 / 指标 ===
	regexp.MustCompile(`\binput\.websocket\b`),
	regexp.MustCompile(`\bwebsocket_(?:connect_duration_ms|status\b|error_count)\b`),

	// === 编造的 n9e API 路径 ===
	regexp.MustCompile(`/api/n9e/target-bindings\b|/api/n9e/target/bindings\b`),
}

// ValidateRestrictedGCOutput 扫描 LLM 输出, 命中 ForbiddenPatterns 时返回
// clean=false 和 hits 命中字面量。
func ValidateRestrictedGCOutput(answer string) (clean bool, hits []string) {
	for _, p := range ForbiddenPatterns {
		if matches := p.FindAllString(answer, -1); len(matches) > 0 {
			hits = append(hits, matches...)
		}
	}
	return len(hits) == 0, hits
}

// BuildHallucinationStamp 生成追加到 LLM 原文末尾的警告 stamp。
// 追加而非替换: 流式已发出, 替换骗不了用户; 且 Turn 2 LLM 看原文+警告才能
// 定位自己上次错在哪。
func BuildHallucinationStamp(hits []string) string {
	var sb strings.Builder
	sb.WriteString("\n\n---\n\n")
	sb.WriteString("⚠️ **后置校验提示**：上述回答命中已知 hallucination 模式")
	if len(hits) > 0 {
		seen := make(map[string]struct{}, len(hits))
		uniq := make([]string, 0, len(hits))
		for _, h := range hits {
			if _, ok := seen[h]; ok {
				continue
			}
			seen[h] = struct{}{}
			uniq = append(uniq, h)
		}
		sb.WriteString("（`")
		sb.WriteString(strings.Join(uniq, "`, `"))
		sb.WriteString("`）")
	}
	sb.WriteString("，未经 n9e 官方文档验证，请到 [V9 文档](https://flashcat.cloud/docs/) 或 [GitHub Issues](https://github.com/ccfos/nightingale/issues) 核对。\n\n")
	sb.WriteString("**下一轮请勿基于上述具体标识符继续。**\n")
	return sb.String()
}
