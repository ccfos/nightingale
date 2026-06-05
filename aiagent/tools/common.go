package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// =============================================================================
// Permission constants (matching router.go rt.perm() strings)
// =============================================================================

const (
	PermAlertRules        = "/alert-rules"
	PermAlertRulesAdd     = "/alert-rules/add"
	PermAlertRulesPut     = "/alert-rules/put"
	PermDashboards        = "/dashboards"
	PermDashboardsAdd     = "/dashboards/add"
	PermDashboardsPut     = "/dashboards/put"
	PermAlertMutes        = "/alert-mutes"
	PermAlertSubscribes   = "/alert-subscribes"
	PermJobTpls           = "/job-tpls"
	PermNotificationRules = "/notification-rules"
	PermUsers             = "/users"
)

// =============================================================================
// User & permission helpers
// =============================================================================

func getUser(deps *aiagent.ToolDeps, params map[string]string) (*models.User, error) {
	if deps == nil || deps.DBCtx == nil {
		return nil, fmt.Errorf("database context not configured")
	}

	var userId int64
	if uid, ok := params["user_id"]; ok {
		userId, _ = strconv.ParseInt(uid, 10, 64)
	}
	if userId == 0 {
		return nil, fmt.Errorf("user_id not found in params")
	}

	user, err := models.UserGetById(deps.DBCtx, userId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %v", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found: %d", userId)
	}
	return user, nil
}

func checkPerm(deps *aiagent.ToolDeps, user *models.User, operation string) error {
	if user.IsAdmin() {
		return nil
	}
	has, err := models.RoleHasOperation(deps.DBCtx, user.RolesLst, operation)
	if err != nil {
		return fmt.Errorf("failed to check permission: %v", err)
	}
	if !has {
		return fmt.Errorf("forbidden: no permission for %s", operation)
	}
	return nil
}

// checkBgRW mirrors the router's bgrw() middleware: a non-admin must belong to
// at least one user-group that has rw flag on this BusiGroup. Without this,
// a user with read-only group membership could create resources via AI tools.
func checkBgRW(deps *aiagent.ToolDeps, user *models.User, bg *models.BusiGroup) error {
	can, err := user.CanDoBusiGroup(deps.DBCtx, bg, "rw")
	if err != nil {
		return fmt.Errorf("failed to check busi group rw permission: %v", err)
	}
	if !can {
		return fmt.Errorf("forbidden: no rw permission on busi group %d", bg.Id)
	}
	return nil
}

func getUserBgids(deps *aiagent.ToolDeps, user *models.User) (bgids []int64, isAdmin bool, err error) {
	if user.IsAdmin() {
		return nil, true, nil
	}
	bgids, err = models.MyBusiGroupIds(deps.DBCtx, user.Id)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get busi group ids: %v", err)
	}
	return bgids, false, nil
}

func getUserGroupIds(deps *aiagent.ToolDeps, userId int64) ([]int64, error) {
	ids, err := models.MyGroupIds(deps.DBCtx, userId)
	if err != nil {
		return nil, fmt.Errorf("failed to get user group ids: %v", err)
	}
	return ids, nil
}

// =============================================================================
// Argument extraction helpers
// =============================================================================

func getArgString(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func getArgInt(args map[string]interface{}, key string, defaultVal int) int {
	if v, ok := args[key].(float64); ok && v > 0 {
		return int(v)
	}
	return defaultVal
}

// getArgBool extracts a boolean arg, accepting both real JSON bools and the
// string forms ("true"/"false") the LLM sometimes emits. Missing/unparseable → false.
func getArgBool(args map[string]interface{}, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(strings.TrimSpace(v))
		return b
	}
	return false
}

func getArgInt64(args map[string]interface{}, key string) int64 {
	switch v := args[key].(type) {
	case float64:
		return int64(v)
	case string:
		id, _ := strconv.ParseInt(v, 10, 64)
		return id
	}
	return 0
}

// getArgFloat extracts a numeric arg, returning (value, present). It accepts
// both JSON numbers (float64) and numeric strings — the LLM sometimes wraps
// numbers in quotes when copying from documentation. Returns present=false
// if the key is missing or unparseable so callers can distinguish "not set"
// from "set to zero".
func getArgFloat(args map[string]interface{}, key string) (float64, bool) {
	switch v := args[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if v == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// =============================================================================
// Tolerant JSON scalar decoders
//
// The LLM frequently quotes scalars inside nested JSON args ("step":"15",
// "instant":"true", "step":15.0). encoding/json rejects all of these into a
// typed int/bool field, which would abort the WHOLE panels/variables parse with
// "invalid JSON". These helpers, used from the patch types' custom UnmarshalJSON,
// accept the loose forms the same way getArgBool/getArgFloat do for flat args.
// A nil/"null"/empty RawMessage decodes to nil (field "not provided").
// =============================================================================

func flexBoolPtr(raw json.RawMessage) (*bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return &b, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		b, err := strconv.ParseBool(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("invalid boolean %q", s)
		}
		return &b, nil
	}
	return nil, fmt.Errorf("invalid boolean %s", string(raw))
}

// flexBool is the value-typed form (nil/absent → false).
func flexBool(raw json.RawMessage) (bool, error) {
	p, err := flexBoolPtr(raw)
	if err != nil || p == nil {
		return false, err
	}
	return *p, nil
}

func flexIntPtr(raw json.RawMessage) (*int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	// JSON number — float64 tolerates both 15 and 15.0.
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		n := int(f)
		return &n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", s)
		}
		n := int(f)
		return &n, nil
	}
	return nil, fmt.Errorf("invalid number %s", string(raw))
}

func getDatasourceId(params map[string]string) int64 {
	if params == nil {
		return 0
	}
	var dsId int64
	if dsIdStr, ok := params["datasource_id"]; ok {
		fmt.Sscanf(dsIdStr, "%d", &dsId)
	}
	return dsId
}

func getDatasourceType(params map[string]string) string {
	if params == nil {
		return ""
	}
	return params["datasource_type"]
}

// resolveDatasource picks a (datasource_id, plugin_type) pair from the
// three possible sources, in order of precedence:
//
//  1. explicit tool args — the caller knows exactly which datasource to
//     target. Used when the LLM discovered the id via list_datasources.
//  2. session params — injected by the router when the chat is opened from
//     a datasource-scoped page (explorer, datasource query page). The
//     explorer flow doesn't require the LLM to know the id.
//  3. DB lookup by id — if args supplied id but no type, fetch plugin_type
//     from models.GetDatasourceInfosByIds. Keeps the LLM from having to
//     remember which cate each id belongs to.
//
// Used by both SQL-class tools (list_databases/tables, describe_table) and
// generic query tools (query_timeseries/query_log), so the name intentionally
// doesn't include "SQL" — the resolver itself doesn't gate on plugin family.
//
// Returns a pre-formatted error if neither source yields a usable id, so
// callers can propagate it straight to the tool Observation.
func resolveDatasource(deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (int64, string, error) {
	dsId := getArgInt64(args, "datasource_id")
	if dsId == 0 {
		dsId = getDatasourceId(params)
	}
	if dsId == 0 {
		return 0, "", fmt.Errorf("datasource_id required: pass it as a tool argument (use list_datasources to find one), or open the chat from a datasource-scoped page")
	}

	dsType := getArgString(args, "datasource_type")
	if dsType == "" {
		dsType = getDatasourceType(params)
	}
	if dsType == "" {
		// DB fallback — lets the LLM pass only datasource_id without
		// having to know whether id=5 is mysql or doris.
		if deps != nil && deps.DBCtx != nil {
			infos, err := models.GetDatasourceInfosByIds(deps.DBCtx, []int64{dsId})
			if err != nil {
				return 0, "", fmt.Errorf("failed to resolve datasource type for id=%d: %v", dsId, err)
			}
			if len(infos) > 0 {
				dsType = infos[0].PluginType
			}
		}
	}
	if dsType == "" {
		return 0, "", fmt.Errorf("datasource_type not resolvable for id=%d: pass datasource_type explicitly or verify the datasource exists", dsId)
	}
	return dsId, dsType, nil
}

// =============================================================================
// Collection helpers
// =============================================================================

func marshalList(total int, items interface{}) string {
	bytes, _ := json.Marshal(map[string]interface{}{
		"total": total,
		"items": items,
	})
	return string(bytes)
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func int64SliceContains(slice []int64, val int64) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func int64SlicesOverlap(a, b []int64) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[int64]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	for _, v := range b {
		if _, ok := set[v]; ok {
			return true
		}
	}
	return false
}

// int64SliceIntersect 返回 a 和 b 都包含的元素（保持 b 的顺序、去重）。
// 用于权限交集场景：target 在 {A,B,C} 三个组，用户能看 {A,C}，查询应该限制到 {A,C}。
func int64SliceIntersect(a, b []int64) []int64 {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	set := make(map[int64]struct{}, len(a))
	for _, v := range a {
		set[v] = struct{}{}
	}
	seen := make(map[int64]struct{}, len(b))
	out := make([]int64, 0, len(b))
	for _, v := range b {
		if _, ok := set[v]; !ok {
			continue
		}
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// =============================================================================
// Time helpers
// =============================================================================

func formatUnixTime(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

func parseTimeRange(tr string) (int64, int64) {
	tr = strings.TrimSpace(strings.ToLower(tr))
	if tr == "" {
		return 0, 0
	}
	now := time.Now()
	etime := now.Unix()
	var num int
	var unit string
	for i, c := range tr {
		if c < '0' || c > '9' {
			num, _ = strconv.Atoi(tr[:i])
			unit = tr[i:]
			break
		}
	}
	if num <= 0 {
		return 0, 0
	}
	var duration time.Duration
	switch unit {
	case "m", "min":
		duration = time.Duration(num) * time.Minute
	case "h", "hour":
		duration = time.Duration(num) * time.Hour
	case "d", "day":
		duration = time.Duration(num) * 24 * time.Hour
	case "w", "week":
		duration = time.Duration(num) * 7 * 24 * time.Hour
	default:
		duration = time.Duration(num) * time.Hour
	}
	stime := now.Add(-duration).Unix()
	return stime, etime
}

// =============================================================================
// Registration shorthand
// =============================================================================

func register(def aiagent.AgentTool, handler aiagent.BuiltinToolFunc) {
	aiagent.RegisterBuiltinTool(def.Name, &aiagent.BuiltinTool{
		Definition: def,
		Handler:    handler,
	})
}
