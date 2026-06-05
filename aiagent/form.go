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
	Key        string          `json:"key"`  // "busi_group_id" | "datasource_id" | "team_ids"
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
			fields = append(fields, loadTeamField(deps, user, pre.TeamIDs))
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

// loadTeamField 列出用户可引用的团队（通知规则接收组）。preselectedIDs 命中的
// 候选 is_default（多选）。
func loadTeamField(deps *ToolDeps, user *models.User, preselectedIDs []int64) FormField {
	field := FormField{Key: "team_ids", Type: "multi", Candidates: []FormCandidate{}}
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
