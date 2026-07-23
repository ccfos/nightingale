package aiagent

import (
	"encoding/json"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// 本文件是 form_select 表单载荷的共享构造器。同一份 JSON 契约被两个调用方使用，
// 保证前端看到字节级同构的表单：
//   - chat 的 creation preflight（动词 fast-path 的零 LLM 表单）；
//   - 写工具的缺参门（ToolInterrupt kind=input，任何路径下兜底）。
//
// JSON 形状是前端/后端契约：前端按 fields 顺序渐进渲染、把选择经 action.param
// 回传。新增字段 key 需要同时教会 loadFormField 和前端表单选择器。

// FormCandidate 表单候选项。
type FormCandidate struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default,omitempty"`
	// Extra 是前端可在名称旁渲染的类型提示（如数据源的 "prometheus"）。可选。
	Extra string `json:"extra,omitempty"`
}

// FormField 表单字段。
type FormField struct {
	Key        string          `json:"key"`  // "busi_group_id" | "datasource_id" | "team_ids" | "skill_scope"
	Type       string          `json:"type"` // "single" | "multi"
	Candidates []FormCandidate `json:"candidates"`
}

// FormPayload form_select response 的 Content 载荷。
type FormPayload struct {
	SkillName string      `json:"skill_name"`
	Fields    []FormField `json:"fields"`
}

// FormPreselect 表单字段的预选值（已知值在表单中以 is_default 预选，用户"确认
// 默认"路径一次点击）。零值字段 = 无预选。
type FormPreselect struct {
	BusiGroupID  int64
	DatasourceID int64
	TeamIDs      []int64
}

// BuildCreationForm 构造覆盖 required 全部字段的 form_select 载荷 JSON。
// 始终弹全部 required 字段而非只弹缺失字段：页面上下文带来的值是提示不是承诺
// （用户要建的告警规则可能指向另一个数据源），以 is_default 预选让用户确认或改选。
func BuildCreationForm(deps *ToolDeps, user *models.User, skillName string, required []string, pre FormPreselect) string {
	fields := make([]FormField, 0, len(required))
	for _, key := range required {
		switch key {
		case "busi_group_id":
			fields = append(fields, loadBusiGroupField(deps, user, pre.BusiGroupID))
		case "datasource_id":
			fields = append(fields, loadDatasourceField(deps, user, pre.DatasourceID))
		case "team_ids":
			fields = append(fields, loadTeamField(deps, user, key, pre.TeamIDs))
		default:
			logger.Warningf("[form] unknown required context key %q for skill %s", key, skillName)
		}
	}
	body, _ := json.Marshal(FormPayload{SkillName: skillName, Fields: fields})
	return string(body)
}

// loadBusiGroupField 列出用户可见业务组。preselectedID>0 且命中候选时该候选
// is_default；否则名称启发式默认（"Default"/"默认"）胜出。
func loadBusiGroupField(deps *ToolDeps, user *models.User, preselectedID int64) FormField {
	field := FormField{Key: "busi_group_id", Type: "single", Candidates: []FormCandidate{}}
	groups, err := user.BusiGroups(deps.DBCtx, 200, "")
	if err != nil {
		logger.Warningf("[form] load busi groups failed: %v", err)
		return field
	}
	for _, g := range groups {
		isDefault := false
		if preselectedID > 0 {
			isDefault = g.Id == preselectedID
		} else {
			isDefault = IsDefaultBusiGroupName(g.Name)
		}
		field.Candidates = append(field.Candidates, FormCandidate{
			ID:        g.Id,
			Name:      g.Name,
			IsDefault: isDefault,
		})
	}
	return field
}

// loadDatasourceField 列出用户可见数据源。preselectedID（通常来自 Copilot 页面
// 上下文）命中时 is_default 预选。
func loadDatasourceField(deps *ToolDeps, user *models.User, preselectedID int64) FormField {
	field := FormField{Key: "datasource_id", Type: "single", Candidates: []FormCandidate{}}
	dsList, err := models.GetDatasourcesGetsBy(deps.DBCtx, "", "", "", "")
	if err != nil {
		logger.Warningf("[form] load datasources failed: %v", err)
		return field
	}
	filtered := deps.FilterDatasources(dsList, user)
	for _, ds := range filtered {
		field.Candidates = append(field.Candidates, FormCandidate{
			ID:        ds.Id,
			Name:      ds.Name,
			Extra:     ds.PluginType,
			IsDefault: preselectedID > 0 && ds.Id == preselectedID,
		})
	}
	return field
}

// loadTeamField 列出用户可引用的团队（通知规则接收组 / 技能管理团队）。preselectedIDs
// 命中的候选 is_default（多选）。key 由调用方给：不同场景用不同的 action.param 键，
// 表单提交值才不会串场（见 SkillTeamsFieldKey）。
func loadTeamField(deps *ToolDeps, user *models.User, key string, preselectedIDs []int64) FormField {
	field := FormField{Key: key, Type: "multi", Candidates: []FormCandidate{}}
	dbCtx := deps.DBCtx

	var groups []models.UserGroup
	if user.IsAdmin() {
		all, err := models.UserGroupGetAll(dbCtx)
		if err != nil {
			logger.Warningf("[form] load all user groups failed: %v", err)
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
			logger.Warningf("[form] load user-group memberships failed: %v", err)
			return field
		}
		lst, err := models.UserGroupGetByIds(dbCtx, ids)
		if err != nil {
			logger.Warningf("[form] load user groups by ids failed: %v", err)
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
		field.Candidates = append(field.Candidates, FormCandidate{ID: g.Id, Name: g.Name, IsDefault: isDefault})
	}
	return field
}

// IsDefaultBusiGroupName 报告业务组名是否像默认组（"Default"/"默认"）——
// list_busi_groups 工具与表单预选共用同一启发式。
func IsDefaultBusiGroupName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "default") || strings.Contains(name, "默认")
}

// ==================== 技能授权表单（创建技能的缺参门） ====================

// SkillTeamsFieldKey 是创建技能时「管理团队」字段的 action.param 键。刻意不复用通知
// 规则的 team_ids：那个键是共享的（通知规则缺参门和 preflight 都会经 params 注入
// team_ids），而 create_skill 要靠"本轮带没带自己的表单值"来判断这一轮是不是授权表单的
// 续跑。共用一个键的话，用户在同一会话里提交通知规则的团队表单，就会被误判成技能表单
// 的续跑，进而因为找不到技能草稿而中止一次本来合法的创建。
const SkillTeamsFieldKey = "skill_team_ids"

// SkillScopeFieldKey 是创建技能时「可见范围」字段的 action.param 键。候选 ID 用 1/2
// 而不是 private 的 0/1：前端把候选 ID 当实体 ID 处理（真值判断），0 会被当成"没选"。
const SkillScopeFieldKey = "skill_scope"

// 可见范围候选 ID → models.AISkill.Private 的映射（前后端契约）。
const (
	SkillScopeCandidatePublic  int64 = 1 // 全员可见 → Private=0
	SkillScopeCandidatePrivate int64 = 2 // 仅管理团队可见 → Private=1
)

// BuildSkillAuthForm 构造创建技能的授权表单：管理团队（多选，候选与通知规则同源）
// + 可见范围（仅 admin，非 admin 一律私有，与技能页面表单口径一致）。与 BuildCreationForm
// 同一 form_select 契约，但可见范围是固定枚举而非平台实体，候选名按 lang 预制
// （同 BuildApprovalForm），故单独构造。
// prePrivate 是模型已给出的可见范围（0/1），以 is_default 预选让用户确认或改选。
func BuildSkillAuthForm(deps *ToolDeps, user *models.User, lang string, preTeamIDs []int64, prePrivate int) string {
	fields := []FormField{loadTeamField(deps, user, SkillTeamsFieldKey, preTeamIDs)}
	if user.IsAdmin() {
		fields = append(fields, FormField{
			Key:  SkillScopeFieldKey,
			Type: "single",
			Candidates: []FormCandidate{
				{ID: SkillScopeCandidatePublic, Name: LangText(lang, "全员可见", "Visible to everyone"), IsDefault: prePrivate == 0},
				{ID: SkillScopeCandidatePrivate, Name: LangText(lang, "仅管理团队可见", "Visible to the managing teams only"), IsDefault: prePrivate != 0},
			},
		})
	}
	body, _ := json.Marshal(FormPayload{SkillName: "skill-creator", Fields: fields})
	return string(body)
}

// ==================== Approval 表单（结构化确认通道） ====================

// ApprovalParamKey 是结构化确认通道的 action.param 键：FE 的 approve/reject
// 按钮（本文件 BuildApprovalForm 产出的 form_select 字段）和 A2A 上游的
// metadata.action_param 都落到这里。结构化通道零 NLP，是确认的主路径；文本
// 分类只是没有结构化能力的客户端的兜底。
const ApprovalParamKey = "approval"

// Approval 表单候选 ID。form_select 候选经 action.param 回传的是数值 ID，
// router 按本对常量解码；A2A 上游无表单，直接回字符串 "approve"/"reject"。
const (
	ApprovalCandidateApprove int64 = 1
	ApprovalCandidateReject  int64 = 2
)

// LangText 选取预制文案：不经 LLM 的固定文案只维护 zh/en 两版，zh_* 与未设置
// 走中文（历史默认），其余语言码（含 ja_JP/ru_RU 等）统一英文兜底。语言码取值
// 见 chat.LanguageDirective 的映射表。同一轮 UI 的全部预制文案（resume 提示、
// approval 按钮）必须经此函数选取，保证语言一致。
func LangText(lang, zh, en string) string {
	if lang == "" || strings.HasPrefix(lang, "zh") {
		return zh
	}
	return en
}

// BuildApprovalForm 构造 approval 类工具中断的二选一 form_select 载荷，与
// creation 表单同一 JSON 契约，前端渲染成按钮。skillName 位填工具名，仅作
// 展示/排查。两个候选都不预选——写操作的确认不能替用户默认。
func BuildApprovalForm(lang, toolName string) string {
	approve := LangText(lang, "确认执行", "Apply")
	reject := LangText(lang, "取消", "Cancel")
	body, _ := json.Marshal(FormPayload{
		SkillName: toolName,
		Fields: []FormField{{
			Key:  ApprovalParamKey,
			Type: "single",
			Candidates: []FormCandidate{
				{ID: ApprovalCandidateApprove, Name: approve},
				{ID: ApprovalCandidateReject, Name: reject},
			},
		}},
	})
	return string(body)
}
