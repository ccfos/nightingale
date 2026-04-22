package chat

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// --- form_select payload shapes ---
//
// The JSON surface below is part of the frontend/backend contract: the
// frontend renders fields progressively and posts the user's picks back via
// action.param. If you add a new field key, teach both emitFormSelect
// (candidate loader) and the frontend form selector to handle it.

type formCandidate struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default,omitempty"`
	// Extra carries a type hint the frontend may render next to the name
	// (e.g. "prometheus" for a datasource). Optional.
	Extra string `json:"extra,omitempty"`
}

type creationFormField struct {
	Key        string          `json:"key"`  // "busi_group_id" | "datasource_id" | "team_ids"
	Type       string          `json:"type"` // "single" | "multi"
	Candidates []formCandidate `json:"candidates"`
}

type creationFormPayload struct {
	SkillName string              `json:"skill_name"`
	Fields    []creationFormField `json:"fields"`
}

// PreflightCreation is the preflight hook for the "creation" action_key.
// It keyword-matches the user input to an n9e-create-* skill and, if ANY
// required context key is missing, emits a single form_select response
// containing EVERY required field — not just the missing ones.
//
// Why "all fields, not just missing ones": some fields (notably datasource_id)
// can arrive pre-populated from the page context (e.g. the user opened the
// Copilot from a Prometheus explorer page, so actionContext.datasource_id was
// auto-attached to action.param). That page default is a hint, not a user
// commitment — the alert rule the user is creating may target a different
// datasource. So we always surface the field in the form, pre-selecting the
// current value via is_default, and let the user confirm or change it before
// the halted turn proceeds.
//
// When every required field is already in req.Context (which happens after
// the form is submitted and resent with sessionParam values), preflight stays
// out of the way and the agent runs normally.
func PreflightCreation(ctx context.Context, deps *aiagent.ToolDeps, req *AIChatRequest, user *models.User) (bool, []models.AssistantMessageResponse, error) {
	spec := matchCreationSkill(req.UserInput)
	if spec == nil {
		logger.Debugf("[preflight] creation: no keyword match for %q, skipping", req.UserInput)
		return false, nil, nil
	}

	anyMissing := false
	for _, key := range spec.requiredContexts {
		if !hasContext(req.Context, key) {
			anyMissing = true
			break
		}
	}
	if !anyMissing {
		return false, nil, nil
	}
	return emitFormSelect(deps, req, user, spec.skillName, spec.requiredContexts)
}

// hasContext tests presence AND non-zero — team_ids needs to be a non-empty
// slice; busi_group_id / datasource_id must be a positive number.
func hasContext(reqCtx map[string]interface{}, key string) bool {
	v, ok := reqCtx[key]
	if !ok {
		return false
	}
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

// emitFormSelect builds a form_select response covering every required field
// of the skill. Fields whose value is already present in req.Context get that
// value pre-selected via is_default=true on the matching candidate, so the
// user's typical "confirm the default" path is one click.
func emitFormSelect(deps *aiagent.ToolDeps, req *AIChatRequest, user *models.User, skillName string, required []string) (bool, []models.AssistantMessageResponse, error) {
	fields := make([]creationFormField, 0, len(required))
	for _, key := range required {
		switch key {
		case "busi_group_id":
			fields = append(fields, loadBusiGroupField(deps, user, ctxInt64Get(req.Context, "busi_group_id")))
		case "datasource_id":
			fields = append(fields, loadDatasourceField(deps, user, ctxInt64Get(req.Context, "datasource_id")))
		case "team_ids":
			fields = append(fields, loadTeamField(deps, user, ctxInt64Slice(req.Context, "team_ids")))
		default:
			logger.Warningf("[preflight] unknown required context key %q for skill %s", key, skillName)
		}
	}
	payload := creationFormPayload{SkillName: skillName, Fields: fields}
	body, _ := json.Marshal(payload)
	return true, []models.AssistantMessageResponse{
		{ContentType: models.ContentTypeFormSelect, Content: string(body)},
	}, nil
}

// loadBusiGroupField lists the user's accessible busi groups. If preselectedID
// is > 0 and matches one of the candidates, that candidate wins is_default.
// Otherwise the name-heuristic default ("Default" / "默认") wins.
func loadBusiGroupField(deps *aiagent.ToolDeps, user *models.User, preselectedID int64) creationFormField {
	field := creationFormField{Key: "busi_group_id", Type: "single", Candidates: []formCandidate{}}
	groups, err := user.BusiGroups(deps.DBCtx, 200, "")
	if err != nil {
		logger.Warningf("[preflight] load busi groups failed: %v", err)
		return field
	}
	for _, g := range groups {
		isDefault := false
		if preselectedID > 0 {
			isDefault = g.Id == preselectedID
		} else {
			isDefault = isDefaultBusiGroupName(g.Name)
		}
		field.Candidates = append(field.Candidates, formCandidate{
			ID:        g.Id,
			Name:      g.Name,
			IsDefault: isDefault,
		})
	}
	return field
}

// loadDatasourceField lists datasources visible to the user. If preselectedID
// is > 0 (typically the one auto-attached from the Copilot's page context),
// that candidate is marked is_default so the form opens with it selected.
func loadDatasourceField(deps *aiagent.ToolDeps, user *models.User, preselectedID int64) creationFormField {
	field := creationFormField{Key: "datasource_id", Type: "single", Candidates: []formCandidate{}}
	dsList, err := models.GetDatasourcesGetsBy(deps.DBCtx, "", "", "", "")
	if err != nil {
		logger.Warningf("[preflight] load datasources failed: %v", err)
		return field
	}
	filtered := deps.FilterDatasources(dsList, user)
	for _, ds := range filtered {
		field.Candidates = append(field.Candidates, formCandidate{
			ID:        ds.Id,
			Name:      ds.Name,
			Extra:     ds.PluginType,
			IsDefault: preselectedID > 0 && ds.Id == preselectedID,
		})
	}
	return field
}

// loadTeamField lists user-group memberships (teams) the user can reference
// in a notify rule. preselectedIDs marks matching candidates is_default for
// the multi-select case.
func loadTeamField(deps *aiagent.ToolDeps, user *models.User, preselectedIDs []int64) creationFormField {
	field := creationFormField{Key: "team_ids", Type: "multi", Candidates: []formCandidate{}}
	dbCtx := deps.DBCtx

	var groups []models.UserGroup
	if user.IsAdmin() {
		all, err := models.UserGroupGetAll(dbCtx)
		if err != nil {
			logger.Warningf("[preflight] load all user groups failed: %v", err)
			return field
		}
		for _, g := range all {
			if g != nil {
				groups = append(groups, *g)
			}
		}
	} else {
		ids, err := models.MyGroupIds(dbCtx, user.Id)
		if err != nil {
			logger.Warningf("[preflight] load user-group memberships failed: %v", err)
			return field
		}
		lst, err := models.UserGroupGetByIds(dbCtx, ids)
		if err != nil {
			logger.Warningf("[preflight] load user groups by ids failed: %v", err)
			return field
		}
		groups = lst
	}
	preselect := make(map[int64]struct{}, len(preselectedIDs))
	for _, id := range preselectedIDs {
		preselect[id] = struct{}{}
	}
	for _, g := range groups {
		_, isDefault := preselect[g.Id]
		field.Candidates = append(field.Candidates, formCandidate{ID: g.Id, Name: g.Name, IsDefault: isDefault})
	}
	return field
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

// isDefaultBusiGroupName mirrors aiagent/tools/busi_group.go's helper of the
// same name so the is_default hint stays consistent between the
// list_busi_groups tool and the preflight selector. Duplicated here rather
// than exported from the tools package to avoid a router → tools compile
// dependency.
func isDefaultBusiGroupName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "default") || strings.Contains(name, "默认")
}
