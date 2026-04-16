package router

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// creationSkillSpec declares, for one n9e-create-* skill, the keywords that
// identify it from the user's raw input and the context keys the skill
// requires before the agent can usefully run. The preflight for the
// "creation" action_key walks this table: first match by keyword, then halt
// the turn with a single form_select response covering every missing context
// key. The frontend renders one progressive form and submits all picks at
// once, avoiding a multi-round ping-pong.
//
// The table is append-only — add a new skill by appending a row. When
// multiple entries keyword-match the same input the first match wins, so put
// more specific skills before broader ones.
//
// requiredContexts ordering is meaningful: it dictates the order fields
// appear in the form, and the frontend locks later fields until earlier ones
// are picked (progressive disclosure).
type creationSkillSpec struct {
	skillName        string
	keywords         []string
	requiredContexts []string // supported keys: "busi_group_id" | "datasource_id" | "team_ids"
}

var creationSkills = []creationSkillSpec{
	// Alert subscribe / mute must come before the generic "告警" keyword used
	// by alert-rule, otherwise "创建订阅" would route to alert-rule.
	{"n9e-create-alert-subscribe", []string{"告警订阅", "订阅规则", "subscribe"}, []string{"busi_group_id"}},
	{"n9e-create-alert-mute", []string{"屏蔽", "静默", "mute"}, []string{"busi_group_id"}},
	{"n9e-create-notify-rule", []string{"通知规则", "notify rule", "notify"}, []string{"team_ids"}},
	// Dashboard 只需业务组——面板可以跨数据源，数据源交给 LLM 从 page context
	// 或 list_datasources 自行解决，preflight 强制选一个反而限制了后续灵活性。
	{"n9e-create-dashboard", []string{"仪表盘", "dashboard", "面板"}, []string{"busi_group_id"}},
	{"n9e-create-alert-rule", []string{"告警规则", "告警", "alert rule"}, []string{"busi_group_id", "datasource_id"}},
}

// matchCreationSkill returns the first creationSkillSpec whose keyword is
// contained in userInput. Match is case-insensitive.
// Returns nil when no keyword hits — callers treat this as "don't intervene,
// let the agent's own skill auto-selection handle it".
func matchCreationSkill(userInput string) *creationSkillSpec {
	lower := strings.ToLower(userInput)
	for i := range creationSkills {
		for _, kw := range creationSkills[i].keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				return &creationSkills[i]
			}
		}
	}
	return nil
}

// creationVerbs and queryVerbs drive the intent fast-path. Kept conservative:
// false negatives are fine (we fall back to the LLM classifier), but false
// positives would mis-route queries like "查询已创建的告警规则" into creation.
var (
	creationVerbs = []string{"创建", "新建", "添加", "加一条", "加一个", "建一条", "建一个", "创一条", "创一个", "create ", "add ", "build ", "make "}
	// queryVerbs act as an anti-signal — if either a query verb or a past-tense
	// creation phrase is present, we refuse the fast-path even when other
	// signals align.
	queryVerbs = []string{"查看", "查询", "查一下", "查下", "看一下", "看下", "列出", "有哪些", "显示", "已创建", "已新建", "已添加", " show ", " list ", " get ", " view "}
)

// hasCreationIntent returns true when the user input unambiguously asks to
// create a new resource. Requires all three signals:
//  1. A creation verb ("创建", "新建", "create", ...).
//  2. A creationSkills keyword (告警规则, 仪表盘, 屏蔽, ...).
//  3. No query verb ("查看", "已创建", "list", ...) that would flip the intent.
//
// This is used as a routing fast-path in processAssistantMessage to bypass
// the LLM classifier (which has been timing out at 15s and falling back to
// general_chat — see WARNING.log "intent inference failed").
func hasCreationIntent(userInput string) bool {
	if !hasAnyKeyword(userInput, creationVerbs) {
		return false
	}
	if matchCreationSkill(userInput) == nil {
		return false
	}
	if hasAnyKeyword(userInput, queryVerbs) {
		return false
	}
	return true
}

func hasAnyKeyword(s string, kws []string) bool {
	lower := strings.ToLower(s)
	for _, kw := range kws {
		if strings.Contains(lower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

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

// preflightCreation is the preflight hook for the "creation" action_key.
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
func preflightCreation(ctx context.Context, req *AIChatRequest, user *models.User) (bool, []models.AssistantMessageResponse, error) {
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
	return emitFormSelect(req, user, spec.skillName, spec.requiredContexts)
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
func emitFormSelect(req *AIChatRequest, user *models.User, skillName string, required []string) (bool, []models.AssistantMessageResponse, error) {
	fields := make([]creationFormField, 0, len(required))
	for _, key := range required {
		switch key {
		case "busi_group_id":
			fields = append(fields, loadBusiGroupField(user, ctxInt64Get(req.Context, "busi_group_id")))
		case "datasource_id":
			fields = append(fields, loadDatasourceField(user, ctxInt64Get(req.Context, "datasource_id")))
		case "team_ids":
			fields = append(fields, loadTeamField(user, ctxInt64Slice(req.Context, "team_ids")))
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
func loadBusiGroupField(user *models.User, preselectedID int64) creationFormField {
	field := creationFormField{Key: "busi_group_id", Type: "single", Candidates: []formCandidate{}}
	groups, err := user.BusiGroups(aiagent.GetDBCtx(), 200, "")
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
func loadDatasourceField(user *models.User, preselectedID int64) creationFormField {
	field := creationFormField{Key: "datasource_id", Type: "single", Candidates: []formCandidate{}}
	dbCtx := aiagent.GetDBCtx()
	dsList, err := models.GetDatasourcesGetsBy(dbCtx, "", "", "", "")
	if err != nil {
		logger.Warningf("[preflight] load datasources failed: %v", err)
		return field
	}
	filtered := aiagent.FilterDatasources(dsList, user)
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
func loadTeamField(user *models.User, preselectedIDs []int64) creationFormField {
	field := creationFormField{Key: "team_ids", Type: "multi", Candidates: []formCandidate{}}
	dbCtx := aiagent.GetDBCtx()

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
