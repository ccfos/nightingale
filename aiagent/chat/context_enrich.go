package chat

import (
	"regexp"
	"strconv"

	"github.com/toolkits/pkg/logger"
)

// 本文件是确定性上下文富化与 Context map 读取工具集：router 在 action.param
// 合并后对每轮调用 EnrichContextFromText，把用户粘贴的资源 URL 提取成结构化
// id 注入 Context；hasContext / ctxInt64Get 等是 Context map 的通用读取助手，
// 被表单流（creation_form.go）与各 action handler 共用。

// alertRuleEditURLRe pulls the rule id out of an /alert-rules/edit/<id> link
// the user may have pasted (e.g. "http://host/alert-rules/edit/178 改成20").
var alertRuleEditURLRe = regexp.MustCompile(`/alert-rules/edit/(\d+)`)

// dashboardURLRe pulls the dashboard id out of a /dashboards/<id> (or
// /dashboard/<id>) link the user may have pasted when asking to modify a board.
var dashboardURLRe = regexp.MustCompile(`/dashboards?/(\d+)`)

// EnrichContextFromText does the cheap, deterministic part of edit-target
// resolution — lifting a rule/dashboard id out of a pasted URL — and injects
// it into req.Context so the agent can skip straight to get_*_detail.
//
// 原为 edit action 的 PreflightEdit；路由收缩后开放输入默认进通用 agent，
// URL 提取对所有 action 生效（router 在 param 合并后统一调用）。纯正则零成本，
// 解析不出时由 agent 自己经工具解析（event_id → get_alert_event_detail 等）。
func EnrichContextFromText(req *AIChatRequest) {
	if !hasContext(req.Context, "rule_id") {
		if m := alertRuleEditURLRe.FindStringSubmatch(req.UserInput); m != nil {
			if id, err := strconv.ParseInt(m[1], 10, 64); err == nil && id > 0 {
				if req.Context == nil {
					req.Context = map[string]interface{}{}
				}
				req.Context["rule_id"] = id
				logger.Infof("[enrich] resolved rule_id=%d from URL in user text", id)
			}
		}
	}
	// Dashboard edits: lift a dashboard id out of a pasted /dashboards/<id> URL.
	// An explicitly pasted URL is an intentional target and OVERRIDES any
	// page-context dashboard_id: the user may have opened the Copilot on board A
	// but pasted board B's link asking to modify B — honoring the stale page
	// context would silently edit the wrong board. Guard on alertRuleEditURLRe
	// NOT matching so an /alert-rules/edit/<id> link (which also contains digits)
	// never gets misread as a dashboard id.
	if !alertRuleEditURLRe.MatchString(req.UserInput) {
		if m := dashboardURLRe.FindStringSubmatch(req.UserInput); m != nil {
			if id, err := strconv.ParseInt(m[1], 10, 64); err == nil && id > 0 {
				if req.Context == nil {
					req.Context = map[string]interface{}{}
				}
				if prev := ctxInt64Get(req.Context, "dashboard_id"); prev > 0 && prev != id {
					logger.Infof("[enrich] pasted dashboard URL id=%d overrides page-context dashboard_id=%d", id, prev)
				} else {
					logger.Infof("[enrich] resolved dashboard_id=%d from URL in user text", id)
				}
				req.Context["dashboard_id"] = id
			}
		}
	}
}

// hasContext tests presence AND non-zero — team_ids needs to be a non-empty
// slice; busi_group_id / datasource_id must be a positive number.
func hasContext(reqCtx map[string]interface{}, key string) bool {
	v, ok := reqCtx[key]
	if !ok {
		return false
	}
	return valueIsSet(v)
}

// valueIsSet reports whether a context value counts as "provided" — shared by
// hasContext and the backfill so both agree on "already set".
func valueIsSet(v interface{}) bool {
	switch typed := v.(type) {
	case int64:
		return typed > 0
	case int:
		return typed > 0
	case float64:
		return typed > 0
	case []int64:
		return len(typed) > 0
	case []interface{}:
		return len(typed) > 0
	}
	return true
}

// ctxInt64Get coerces common int-shaped values in a generic context map into
// int64. Returns 0 when key is absent or unparseable.
func ctxInt64Get(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

// ctxInt64Slice coerces []int64 / []interface{} of ints from a generic
// context map. Returns nil when absent or mistyped.
func ctxInt64Slice(m map[string]interface{}, key string) []int64 {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch arr := v.(type) {
	case []int64:
		return arr
	case []interface{}:
		out := make([]int64, 0, len(arr))
		for _, it := range arr {
			switch n := it.(type) {
			case int64:
				out = append(out, n)
			case int:
				out = append(out, int64(n))
			case float64:
				out = append(out, int64(n))
			}
		}
		return out
	}
	return nil
}
