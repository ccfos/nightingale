package skillgateway

import (
	"github.com/ccfos/nightingale/v6/models"
)

// ops.go is the gateway's allow-list of callable operations (§12.3). Phase 1 is
// READ-ONLY: every op maps to a grant token (checked against the grantable
// envelope + hard-deny) and an optional n9e RBAC operation string (checked
// against the bound user). Results are always scoped to the user's busi groups
// and capped, so an op can never return more than the launching user could see.
// Write/delete ops are intentionally absent — they would be hard-denied (`*:write`
// / `*:delete`) and, when added, must route through the existing two-stage
// confirmation gate (§11.3/§12.2), not live here.

// maxRows caps every list response so a granted skill cannot bulk-exfiltrate or
// DoS the internal API (paired with the per-exec rate limiter, §12.4).
const maxRows = 200

type opSpec struct {
	// Grant is the capability token gated by grantable_n9e_api / deny (§12.2).
	// Empty means "identity only" (whoami): always allowed, touches no data.
	Grant string
	// Operation is the n9e RBAC operation checked via user.CheckPerm. Empty skips
	// the op-level check — busi-group scoping in the handler is then the gate.
	Operation string
	Handler   func(g *Gateway, args map[string]any) (any, error)
}

var ops = map[string]opSpec{
	"whoami":           {Grant: "", Handler: opWhoami},
	"list_alert_rules": {Grant: "alert:read", Operation: "/alert-rules", Handler: opListAlertRules},
	"list_datasources": {Grant: "datasource:read", Handler: opListDatasources},
	"list_targets":     {Grant: "target:read", Handler: opListTargets},
	"query_cur_events": {Grant: "event:read", Handler: opQueryCurEvents},
}

func listResult(total int, items any) map[string]any {
	return map[string]any{"total": total, "items": items}
}

// scopeBgids returns the busi-group filter for a list query: nil for admins (the
// models layer treats an empty slice as "all"), the user's groups otherwise, and
// ok=false when a non-admin belongs to no group (→ caller returns an empty list
// instead of accidentally querying everything).
func (g *Gateway) scopeBgids() (bgids []int64, ok bool) {
	if g.isAdmin {
		return nil, true
	}
	if len(g.bgids) == 0 {
		return nil, false
	}
	return g.bgids, true
}

func opWhoami(g *Gateway, _ map[string]any) (any, error) {
	return map[string]any{
		"user_id":        g.user.Id,
		"username":       g.user.Username,
		"nickname":       g.user.Nickname,
		"is_admin":       g.isAdmin,
		"busi_group_ids": g.bgids, // nil for admin → "all"
	}, nil
}

func opListAlertRules(g *Gateway, _ map[string]any) (any, error) {
	bgids, ok := g.scopeBgids()
	if !ok {
		return listResult(0, []any{}), nil
	}
	rules, err := models.AlertRuleGetsByBGIds(g.dbctx, bgids)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rules))
	for i := range rules {
		if i >= maxRows {
			break
		}
		r := &rules[i]
		items = append(items, map[string]any{
			"id":       r.Id,
			"group_id": r.GroupId,
			"name":     r.Name,
			"cate":     r.Cate,
			"severity": r.Severity,
			"disabled": r.Disabled,
		})
	}
	return listResult(len(rules), items), nil
}

// opListDatasources returns only non-secret projection fields (id/name/type/
// category/status) — never Settings/Auth/HTTP, which carry credentials (§12.4).
func opListDatasources(g *Gateway, _ map[string]any) (any, error) {
	dss, err := models.GetDatasources(g.dbctx)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(dss))
	for i := range dss {
		if i >= maxRows {
			break
		}
		d := &dss[i]
		items = append(items, map[string]any{
			"id":          d.Id,
			"name":        d.Name,
			"plugin_type": d.PluginType,
			"category":    d.Category,
			"status":      d.Status,
		})
	}
	return listResult(len(dss), items), nil
}

func opListTargets(g *Gateway, _ map[string]any) (any, error) {
	bgids, ok := g.scopeBgids()
	if !ok {
		return listResult(0, []any{}), nil
	}
	var opts []models.BuildTargetWhereOption
	if len(bgids) > 0 {
		opts = append(opts, models.BuildTargetWhereWithBgids(bgids))
	}
	targets, err := models.TargetGets(g.dbctx, maxRows, 0, "ident", false, opts...)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		items = append(items, map[string]any{
			"ident":    t.Ident,
			"group_id": t.GroupId,
			"note":     t.Note,
		})
	}
	return listResult(len(targets), items), nil
}

func opQueryCurEvents(g *Gateway, _ map[string]any) (any, error) {
	bgids, ok := g.scopeBgids()
	if !ok {
		return listResult(0, []any{}), nil
	}
	events, err := models.AlertCurEventsGet(g.dbctx, nil, bgids, 0, 0, nil, nil, nil, 0, "", maxRows, 0, nil)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(events))
	for i := range events {
		e := &events[i]
		items = append(items, map[string]any{
			"id":            e.Id,
			"rule_id":       e.RuleId,
			"rule_name":     e.RuleName,
			"group_id":      e.GroupId,
			"severity":      e.Severity,
			"trigger_time":  e.TriggerTime,
			"cate":          e.Cate,
			"datasource_id": e.DatasourceId,
			"tags":          e.TagsJSON,
		})
	}
	return listResult(len(events), items), nil
}
