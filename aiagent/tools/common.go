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
	PermDashboards        = "/dashboards"
	PermDashboardsAdd     = "/dashboards/add"
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
// SQL identifier validation
// =============================================================================

func isValidIdentifier(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if c == ';' || c == '\'' || c == '"' || c == '`' || c == '\\' || c == 0 {
			return false
		}
	}
	return !strings.ContainsAny(s, "/*")
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
