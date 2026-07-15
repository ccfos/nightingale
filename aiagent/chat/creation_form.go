package chat

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// 本文件是 creation 动词 fast-path 的表单流（原 preflight.go）：关键词匹配出
// 创建类 skill 后，先尝试从自然语言解析业务组/数据源、再跨轮回填，仍缺必填项
// 才以 form_select 表单中止本轮。载荷构造在共享的 aiagent.BuildCreationForm
// （写工具的缺参门产出字节级同构的表单）。

// PreflightCreation is the preflight hook for the "creation" action_key.
// It keyword-matches the user input to an create-* skill and, if ANY
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

// backfillCreationContext inherits required keys the user already submitted
// earlier in the same chat (e.g. a prior form_select), so a follow-up turn that
// keyword-matches a creation skill won't re-ask. Absent-only (current turn
// wins); a brand-new creation request never inherits (cross-flow guard).
func backfillCreationContext(deps *aiagent.ToolDeps, req *AIChatRequest, required []string) {
	if req.ChatID == "" {
		return
	}
	if HasCreationIntent(req.UserInput) || HasImportIntent(req.UserInput) {
		return // 新发起的创建/导入从零开始，不继承上一次的数据源（导入 Redis 包不该粘上次 MySQL 的源）
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
	body := aiagent.BuildCreationForm(deps, user, skillName, required, aiagent.FormPreselect{
		BusiGroupID:  ctxInt64Get(req.Context, "busi_group_id"),
		DatasourceID: ctxInt64Get(req.Context, "datasource_id"),
		TeamIDs:      ctxInt64Slice(req.Context, "team_ids"),
	})
	return true, []models.AssistantMessageResponse{
		{ContentType: models.ContentTypeFormSelect, Content: body},
	}, nil
}
