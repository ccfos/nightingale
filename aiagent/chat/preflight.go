package chat

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

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

	// 用户常在自然语言里点名业务组（`在 "aadddd" 业务组创建仪表盘`、`业务组 256`），
	// 但该信息不在结构化 action.param 里。弹全量选择器前，先从输入文本把业务组解析成
	// ID 注入 Context：唯一命中即免去用户重选，下游 buildCreationPrompt/Inputs 会据此
	// 让 LLM 直接用该 group_id（do NOT call list_busi_groups）。解析不出或有歧义则
	// 落到下面的选择器表单（零风险回退）。
	if requiresContext(spec.requiredContexts, "busi_group_id") && !hasContext(req.Context, "busi_group_id") {
		if id, ok := resolveBusiGroupFromText(deps, user, req.UserInput); ok {
			if req.Context == nil {
				req.Context = map[string]interface{}{}
			}
			req.Context["busi_group_id"] = id
		}
	}

	// 数据源同理：编排/子代理常把数据源以自然语言带进来（`数据源：demo-vm`、
	// `datasource_id: 694`），却不放进 action.param。唯一命中即注入 ID，免去重复弹表单；
	// 解析不出或有歧义则落到下面的选择器表单（零风险回退）。
	if requiresContext(spec.requiredContexts, "datasource_id") && !hasContext(req.Context, "datasource_id") {
		if id, ok := resolveDatasourceFromText(deps, user, req.UserInput); ok {
			if req.Context == nil {
				req.Context = map[string]interface{}{}
			}
			req.Context["datasource_id"] = id
		}
	}

	// 跨回合继承：本轮仍缺的 required key 用同一会话更早提交过的值补上，避免重复弹表单。
	backfillCreationContext(deps, req, spec.requiredContexts)

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

// alertRuleEditURLRe pulls the rule id out of an /alert-rules/edit/<id> link
// the user may have pasted (e.g. "http://host/alert-rules/edit/178 改成20").
var alertRuleEditURLRe = regexp.MustCompile(`/alert-rules/edit/(\d+)`)

// PreflightEdit is the preflight hook for the "edit" action_key. It does the
// cheap, deterministic part of target resolution — lifting a rule id out of a
// pasted /alert-rules/edit/<id> URL — and injects it as rule_id so the agent
// can skip straight to get_alert_rule_detail.
//
// It never halts: when the rule can't be resolved deterministically here, the
// edit agent still resolves it via tools (event_id → get_alert_event_detail,
// or list_alert_rules by name) per buildEditPrompt. Business group / datasource
// are read off the rule, so — unlike creation — edit never needs a form.
func PreflightEdit(ctx context.Context, deps *aiagent.ToolDeps, req *AIChatRequest, user *models.User) (bool, []models.AssistantMessageResponse, error) {
	if !hasContext(req.Context, "rule_id") {
		if m := alertRuleEditURLRe.FindStringSubmatch(req.UserInput); m != nil {
			if id, err := strconv.ParseInt(m[1], 10, 64); err == nil && id > 0 {
				if req.Context == nil {
					req.Context = map[string]interface{}{}
				}
				req.Context["rule_id"] = id
				logger.Infof("[preflight] edit: resolved rule_id=%d from URL in user text", id)
			}
		}
	}
	return false, nil, nil
}

// requiresContext 报告 key 是否在该 skill 的必填上下文里。
func requiresContext(required []string, key string) bool {
	for _, k := range required {
		if k == key {
			return true
		}
	}
	return false
}

// busiGroupRef 是匹配器使用的轻量 (id,name) 对，与 models.BusiGroup 解耦，
// 让 pickBusiGroup 保持纯函数、便于单测。
type busiGroupRef struct {
	ID   int64
	Name string
}

var (
	// 关键词在前："业务组 256"、"业务组id=256"、"group_id：256"、"busi_group_id 256"
	busiGroupIDAfter = regexp.MustCompile(`(?:业务组|busi_?group_?id|busi_?group|group_?id)\s*(?:id)?\s*[#:：=＝是为]*\s*(\d+)`)
	// 数字在前："256 业务组"、"256号业务组"、"256 的业务组"
	busiGroupIDBefore = regexp.MustCompile(`(\d+)\s*号?\s*(?:的)?\s*业务组`)
)

// resolveBusiGroupFromText 取用户可见业务组并委托 pickBusiGroup 做匹配，记录结果供审计。
func resolveBusiGroupFromText(deps *aiagent.ToolDeps, user *models.User, userInput string) (int64, bool) {
	groups, err := user.BusiGroups(deps.DBCtx, 200, "")
	if err != nil {
		logger.Warningf("[preflight] resolve busi group: load groups failed: %v", err)
		return 0, false
	}
	refs := make([]busiGroupRef, 0, len(groups))
	for _, g := range groups {
		refs = append(refs, busiGroupRef{ID: g.Id, Name: g.Name})
	}
	id, ok := pickBusiGroup(refs, userInput)
	if ok {
		logger.Infof("[preflight] resolved busi_group_id=%d from user text %q", id, userInput)
	} else {
		logger.Debugf("[preflight] no unique busi group resolved from %q, deferring to form", userInput)
	}
	return id, ok
}

// pickBusiGroup 解析用户在自然语言里点名的业务组，同时支持「名称」和「ID」两种写法：
//   - 名称：业务组全名作为子串出现在文本里；命中名互为包含时取最具体（最长）的。
//   - ID：数字须紧邻"业务组/group_id"等关键词，且必须命中一个用户可见的真实业务组，
//     这样 "CPU>80%"、"持续1分钟" 等无关数字永远不会被采纳。
//
// 当且仅当文本最终唯一指向一个业务组时返回其 id；零命中、歧义或冲突返回 (0,false)。
// 纯函数（不碰 DB），resolveBusiGroupFromText 负责取数后委托它。
func pickBusiGroup(groups []busiGroupRef, userInput string) (int64, bool) {
	input := strings.ToLower(userInput)

	valid := make(map[int64]struct{}, len(groups))
	for _, g := range groups {
		valid[g.ID] = struct{}{}
	}

	matched := map[int64]struct{}{}

	// (a) 按 ID：数字须紧邻业务组关键词，且必须是真实可见业务组。
	for _, re := range []*regexp.Regexp{busiGroupIDAfter, busiGroupIDBefore} {
		for _, m := range re.FindAllStringSubmatch(input, -1) {
			if id, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				if _, ok := valid[id]; ok {
					matched[id] = struct{}{}
				}
			}
		}
	}

	// (b) 按名称：全名子串命中；命中名互为包含时取最长（最具体）的。
	type hit struct {
		id   int64
		name string
	}
	var nameHits []hit
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		if utf8.RuneCountInString(name) < 2 { // 跳过 1 字符超短名，避免巧合命中
			continue
		}
		if strings.Contains(input, strings.ToLower(name)) {
			nameHits = append(nameHits, hit{g.ID, name})
		}
	}
	for _, h := range nameHits {
		nested := false
		for _, o := range nameHits {
			lh, lo := strings.ToLower(h.name), strings.ToLower(o.name)
			if h.id != o.id && len(o.name) > len(h.name) && strings.Contains(lo, lh) {
				nested = true
				break
			}
		}
		if !nested {
			matched[h.id] = struct{}{}
		}
	}

	if len(matched) != 1 {
		return 0, false
	}
	for id := range matched {
		return id, true
	}
	return 0, false
}

// datasourceRef is the lightweight (id,name) pair pickDatasource matches
// against — decoupled from models.Datasource to keep pickDatasource pure.
type datasourceRef struct {
	ID   int64
	Name string
}

var (
	// 关键词在前："数据源 694"、"datasource_id: 694"、"datasource：694"
	datasourceIDAfter = regexp.MustCompile(`(?:数据源|datasource_?id|datasource)\s*(?:id)?\s*[#:：=＝是为]*\s*(\d+)`)
	// 数字在前："694 数据源"、"694号数据源"
	datasourceIDBefore = regexp.MustCompile(`(\d+)\s*号?\s*(?:的)?\s*数据源`)
)

// resolveDatasourceFromText loads the user's visible datasources and delegates
// to pickDatasource. Mirrors resolveBusiGroupFromText so datasource gets the
// same "name or id in free text → id" treatment the busi group already has —
// needed because orchestrator/sub-agent turns ship the datasource in prose
// (`数据源：demo-vm`) rather than in action.param.
func resolveDatasourceFromText(deps *aiagent.ToolDeps, user *models.User, userInput string) (int64, bool) {
	dsList, err := models.GetDatasourcesGetsBy(deps.DBCtx, "", "", "", "")
	if err != nil {
		logger.Warningf("[preflight] resolve datasource: load datasources failed: %v", err)
		return 0, false
	}
	filtered := deps.FilterDatasources(dsList, user)
	refs := make([]datasourceRef, 0, len(filtered))
	for _, ds := range filtered {
		refs = append(refs, datasourceRef{ID: ds.Id, Name: ds.Name})
	}
	id, ok := pickDatasource(refs, userInput)
	if ok {
		logger.Infof("[preflight] resolved datasource_id=%d from user text %q", id, userInput)
	} else {
		logger.Debugf("[preflight] no unique datasource resolved from %q, deferring to form", userInput)
	}
	return id, ok
}

// pickDatasource resolves a datasource named in free text — same rules as
// pickBusiGroup: full-name substring (most specific/longest wins) or an id
// adjacent to a datasource keyword that hits a real visible datasource. Returns
// (id,true) only on a unique hit; zero/ambiguous/conflicting → (0,false), which
// defers to the form selector. Pure (no DB) for unit testing.
func pickDatasource(datasources []datasourceRef, userInput string) (int64, bool) {
	input := strings.ToLower(userInput)

	valid := make(map[int64]struct{}, len(datasources))
	for _, d := range datasources {
		valid[d.ID] = struct{}{}
	}

	matched := map[int64]struct{}{}

	// (a) 按 ID：数字须紧邻数据源关键词，且必须是真实可见数据源。
	for _, re := range []*regexp.Regexp{datasourceIDAfter, datasourceIDBefore} {
		for _, m := range re.FindAllStringSubmatch(input, -1) {
			if id, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				if _, ok := valid[id]; ok {
					matched[id] = struct{}{}
				}
			}
		}
	}

	// (b) 按名称：全名子串命中；命中名互为包含时取最长（最具体）的。
	type hit struct {
		id   int64
		name string
	}
	var nameHits []hit
	for _, d := range datasources {
		name := strings.TrimSpace(d.Name)
		if utf8.RuneCountInString(name) < 2 { // 跳过 1 字符超短名，避免巧合命中
			continue
		}
		if strings.Contains(input, strings.ToLower(name)) {
			nameHits = append(nameHits, hit{d.ID, name})
		}
	}
	for _, h := range nameHits {
		nested := false
		for _, o := range nameHits {
			lh, lo := strings.ToLower(h.name), strings.ToLower(o.name)
			if h.id != o.id && len(o.name) > len(h.name) && strings.Contains(lo, lh) {
				nested = true
				break
			}
		}
		if !nested {
			matched[h.id] = struct{}{}
		}
	}

	if len(matched) != 1 {
		return 0, false
	}
	for id := range matched {
		return id, true
	}
	return 0, false
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

// backfillCreationContext inherits required keys the user already submitted
// earlier in the same chat (e.g. a prior form_select), so a follow-up turn that
// keyword-matches a creation skill won't re-ask. Absent-only (current turn
// wins); a brand-new creation request never inherits (cross-flow guard).
func backfillCreationContext(deps *aiagent.ToolDeps, req *AIChatRequest, required []string) {
	if req.ChatID == "" {
		return
	}
	if HasCreationIntent(req.UserInput) {
		return // 新发起的创建从零开始，不继承上一次创建的数据源
	}

	missing := make([]string, 0, len(required))
	for _, k := range required {
		if !hasContext(req.Context, k) {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return
	}

	msgs, err := models.AssistantMessageGetsByChat(deps.DBCtx, req.ChatID)
	if err != nil {
		logger.Warningf("[preflight] backfill: load chat %s failed: %v", req.ChatID, err)
		return
	}
	if req.Context == nil {
		req.Context = map[string]interface{}{}
	}
	applyBackfill(req.Context, missing, msgs, req.SeqID, req.ChatID)
}

// applyBackfill is the pure (DB-free) core: priorMsgs (seq-ascending) is scanned
// newest→oldest, filling each still-missing key from the first earlier turn that
// carries it. The scan stops AT — and does not inherit from — a turn that already
// produced a creation result card: that turn marks a completed prior creation,
// whose context belongs to a different flow. This is a hard boundary on purpose:
// a follow-up right after a one-turn creation re-asks rather than risk donating a
// finished rule's datasource to a new one — "ask when unsure" is the safe side.
func applyBackfill(dst map[string]interface{}, missing []string, priorMsgs []models.AssistantMessage, curSeq int64, chatID string) {
	for i := len(priorMsgs) - 1; i >= 0 && len(missing) > 0; i-- {
		m := priorMsgs[i]
		if curSeq > 0 && m.SeqID >= curSeq {
			continue // 跳过在途/更新的回合
		}
		if hasCreationResult(m.Response) {
			break // 到此即上一次已完成的创建：其值不继承，也不再向更早回溯
		}

		remain := make([]string, 0, len(missing))
		for _, k := range missing {
			if v, ok := m.Query.Action.Param[k]; ok && valueIsSet(v) {
				dst[k] = v
				logger.Debugf("[preflight] backfill %s=%v from chat=%s seq=%d", k, v, chatID, m.SeqID)
			} else {
				remain = append(remain, k)
			}
		}
		missing = remain
	}
}

// hasCreationResult flags a completed-creation card — the flow boundary.
func hasCreationResult(resps []models.AssistantMessageResponse) bool {
	for _, r := range resps {
		switch r.ContentType {
		case models.ContentTypeAlertRule, models.ContentTypeDashboard:
			return true
		}
	}
	return false
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
