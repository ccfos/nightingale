package tools

import (
	"strconv"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// 本文件实现写工具的缺参门：创建类
// 工具发现 group_id 缺失时，返回 input 类 ToolInterrupt——结构化业务组表单经
// router 渲染为 form_select（与 creation preflight 同一前端契约），用户提交后
// 带着补全的上下文重跑 agent。
//
// 这是 preflight 表单的执行层兜底：动词 fast-path 命中时 preflight 仍零 LLM 即时
// 弹表单；fast-path 漏网（意图绕弯的表达、通用路径下的创建）由本门接住，写操作
// 因此可以安全暴露给通用 agent，不再依赖"先分类对才有门"。

// resolveCreationGroupID 解析创建类工具的目标业务组：优先工具参数（模型显式
// 传入），回退 preflight/表单经 inputs 注入的 params["busi_group_id"]。
func resolveCreationGroupID(args map[string]interface{}, params map[string]string) int64 {
	if id := getArgInt64(args, "group_id"); id > 0 {
		return id
	}
	if s, ok := params["busi_group_id"]; ok {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return 0
}

// resolveCreationTeamIDs 解析创建通知规则的目标团队(用户组)：优先工具已从 config 解析出的
// user_group_ids，回退 preflight/表单经 inputs 注入的 params["team_ids"]("1,2,3")。
// 通知规则没有业务组维度，权限/收件人都挂在团队(UserGroup)上，故缺参门收的是 team_ids
// 而非 busi_group_id——与 BuildCreationForm 对 "team_ids" 的多选表单一一对应。
func resolveCreationTeamIDs(fromConfig []int64, params map[string]string) []int64 {
	if len(fromConfig) > 0 {
		return fromConfig
	}
	s, ok := params["team_ids"]
	if !ok {
		return nil
	}
	var ids []int64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := strconv.ParseInt(part, 10, 64); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

// resolveSkillPrivate 解析创建技能的可见范围（models.AISkill.Private）：非管理员一律
// 私有（仅管理团队可见）——技能页面同样不给非管理员渲染这一项，提交时默认私有。
// 管理员优先取表单提交的 skill_scope：那是用户刚在表单里做的选择，比模型草拟工具参数时
// 的 private 新；都没有则默认私有，绝不默认成公共。
func resolveSkillPrivate(user *models.User, args map[string]interface{}, params map[string]string) int {
	if !user.IsAdmin() {
		return 1
	}
	if id, err := strconv.ParseInt(strings.TrimSpace(params[aiagent.SkillScopeFieldKey]), 10, 64); err == nil {
		switch id {
		case aiagent.SkillScopeCandidatePublic:
			return 0
		case aiagent.SkillScopeCandidatePrivate:
			return 1
		}
	}
	if f, ok := getArgFloat(args, "private"); ok && f == 0 {
		return 0
	}
	return 1
}

// skillAuthFormInterrupt 构造创建技能的授权缺参门中断：管理团队必填，缺失时弹表单让
// 用户选（管理员同时选可见范围），而不是替用户挑一个团队——与 create_notify_rule 的
// 团队缺参门同一套前端契约。
func skillAuthFormInterrupt(lang string, deps *aiagent.ToolDeps, user *models.User, private int) *aiagent.ToolInterrupt {
	return &aiagent.ToolInterrupt{
		Kind: aiagent.InterruptKindInput,
		Prompt: aiagent.LangText(lang,
			"创建技能前需要先确认管理团队（团队成员可编辑该技能）。请在下方表单中选择后提交，我会接着完成创建。",
			"Before creating the skill, its managing teams (whose members may edit it) need to be confirmed. Please pick and submit in the form below, and I'll continue with the creation."),
		Form: aiagent.BuildSkillAuthForm(deps, user, lang, nil, private),
	}
}

// creationFormInterrupt 构造创建类工具的 input 中断：Form 为与 preflight 字节级
// 同构的 form_select 载荷，Prompt 为纯文本客户端（A2A）的兜底文案。
// lang 取 params["lang"]（router 注入的 UI 语言）。
func creationFormInterrupt(lang string, deps *aiagent.ToolDeps, user *models.User, skillName string, required []string) *aiagent.ToolInterrupt {
	return &aiagent.ToolInterrupt{
		Kind: aiagent.InterruptKindInput,
		Prompt: aiagent.LangText(lang,
			"创建前需要先确认目标业务组等必填信息。请在下方表单中选择后提交（也可以直接在回复里写明业务组名称或 ID），我会接着完成创建。",
			"Before creating, the required fields (e.g. the target business group) need to be confirmed. Please pick and submit in the form below (or just state the business group name or ID in your reply), and I'll continue with the creation."),
		Form: aiagent.BuildCreationForm(deps, user, skillName, required, aiagent.FormPreselect{}),
	}
}
