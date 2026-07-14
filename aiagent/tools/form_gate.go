package tools

import (
	"fmt"
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

// creationFormInterrupt 构造创建类工具的 input 中断：Form 为与 preflight 字节级
// 同构的 form_select 载荷，Prompt 为纯文本客户端（A2A）的兜底文案。
// lang 取 params["lang"]（router 注入的 UI 语言）。
func creationFormInterrupt(lang string, deps *aiagent.ToolDeps, user *models.User, skillName string, required []string) *aiagent.ToolInterrupt {
	return &aiagent.ToolInterrupt{
		Kind: aiagent.InterruptKindInput,
		Prompt: aiagent.LangText(lang,
			"创建前需要先确认目标业务组等必填信息。请在下方表单中选择后提交（也可以直接在回复里写明业务组名称或 ID），我会接着完成创建。",
			"Before creating, the required fields (e.g. the target business group) need to be confirmed. Please pick and submit in the form below (or just state the business group name or ID in your reply), and I'll continue with the creation."),
		Form: aiagent.BuildCreationForm(deps, user, lang, skillName, required, aiagent.FormPreselect{}),
	}
}

// =============================================================================
// 创建 MCP / 技能的授权收集（管理团队 + 可见范围）
//
// create_mcp_server 与 create_skill 共用这一条授权路径，保证两个创建入口在"谁能管、
// 谁能看"上的行为一致：
//   - 非 admin：只让其选管理团队（表单只列出自己所属团队）；可见范围恒为「仅团队可见」
//     （private=1，最小暴露），不让其选。
//   - admin：既选管理团队又设可见范围（公开 / 仅团队可见）。
// 团队/可见范围经 form_select 表单收集（团队多选 + 可见范围单选），提交值经 params
// （team_ids / private）注入，与两阶段确认的重放同一通道（PendingInterrupt.Params）。
// =============================================================================

// creationAuthScope 是解析完成、可直接落库的管理团队与可见范围。
type creationAuthScope struct {
	UserGroupIds []int64
	Private      int
}

// resolveCreationAuth 解析创建 MCP/技能时的管理团队与可见范围。信息不全时返回一个
// input 中断（*aiagent.ToolInterrupt）让用户补选——调用方拿到非 nil 中断应直接
// `return "", intr`。formSkillName 仅作 form 载荷的展示/排查标签。
//
// 团队/可见范围双通道解析：优先取工具参数 args（模型把用户的明确回复——含「直接在
// 回复里写明团队名」经 list_teams 解析出的 ID——转成结构化参数），回退表单经 params
// 注入的值。缺一不可：只认 params 会让「直接文本回复」和「纯文本 A2A 客户端」永远补
// 不上参数，反复弹同一张表单成死循环。
//
// 返回约定：intr != nil 表示需要用户补选（团队/可见范围缺失）；err != nil 表示鉴权
// 失败（如非 admin 越权授权别的团队）；两者皆 nil 时 scope 可用。
func resolveCreationAuth(deps *aiagent.ToolDeps, user *models.User, args map[string]interface{}, params map[string]string, formSkillName string) (creationAuthScope, *aiagent.ToolInterrupt, error) {
	teamIDs := resolveCreationTeamIDs(argInt64Slice(args, "team_ids"), params)
	isAdmin := user.IsAdmin()

	// 非 admin 恒为「仅团队可见」（也是缺省），绝不读 args/params——防止越权放开可见性；
	// admin 优先取参数、回退表单 params 提交的 private。
	private := int(aiagent.VisibilityTeamScope)
	privateProvided := true
	if isAdmin {
		private, privateProvided = resolveCreationPrivate(args, params)
	}

	// 始终弹全部 required 字段（与 BuildCreationForm 的哲学一致）：非 admin 只需团队，
	// admin 需团队 + 可见范围。任一缺失即中止本轮弹表单，绝不替用户默认团队/可见范围。
	required := []string{"team_ids"}
	if isAdmin {
		required = append(required, "visibility")
	}
	if len(teamIDs) == 0 || (isAdmin && !privateProvided) {
		return creationAuthScope{}, creationAuthInterrupt(params["lang"], deps, user, formSkillName, required), nil
	}

	if private != int(aiagent.VisibilityPublic) && private != int(aiagent.VisibilityTeamScope) {
		return creationAuthScope{}, nil, fmt.Errorf("private flag must be 0 or 1")
	}

	// 非 admin 只能授权自己所属团队：表单本就只列出自己团队，此处再兜底一次防越权
	// （篡改的 action.param 可绕过表单直接注入 team_ids）。与 HTTP 路由 resolveSkillAuth
	// 的子集校验同义。
	if !isAdmin {
		gids, err := getUserGroupIds(deps, user.Id)
		if err != nil {
			return creationAuthScope{}, nil, err
		}
		if !int64SliceSubset(gids, teamIDs) {
			return creationAuthScope{}, nil, fmt.Errorf("forbidden: you can only authorize teams you belong to")
		}
	}

	return creationAuthScope{UserGroupIds: teamIDs, Private: private}, nil, nil
}

// resolveCreationPrivate 解析可见范围：优先工具参数 args["private"]（模型据用户回复
// 回填），回退表单经 params 注入的 private("0"/"1")。第二个返回值报告是否已提交（区分
// "未提交"与"显式提交 0"——getArgInt 那套 v>0 语义会把 0 当缺省，这里必须按存在性判断）。
func resolveCreationPrivate(args map[string]interface{}, params map[string]string) (int, bool) {
	if p, ok := argIntPresent(args, "private"); ok {
		return p, true
	}
	s, ok := params["private"]
	if !ok || strings.TrimSpace(s) == "" {
		return 0, false
	}
	p, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return p, true
}

// creationAuthInterrupt 构造 create_mcp_server / create_skill 的授权补选中断：Form 是
// 与 preflight 同构的 form_select（团队多选 + admin 的可见范围单选），供 n9e 前端渲染；
// Prompt 是纯文本客户端（A2A）的兜底文案。文案承诺「也可以直接在回复里说明」——这条承诺
// 由两个工具的 team_ids/private 参数兑现（模型据回复回填，缺 ID 时先 list_teams 解析），
// 不再是无结构化表单客户端补不上的空头支票。
func creationAuthInterrupt(lang string, deps *aiagent.ToolDeps, user *models.User, formSkillName string, required []string) *aiagent.ToolInterrupt {
	prompt := aiagent.LangText(lang,
		"创建前请先指定管理团队：在下方表单里选择后提交，或直接在回复里写明团队名称（我来解析对应 ID）。",
		"Before creating, choose the managing team: pick in the form below and submit, or just tell me the team name in your reply (I'll resolve its ID).")
	if user.IsAdmin() {
		prompt = aiagent.LangText(lang,
			"创建前请先指定管理团队与可见范围：在下方表单里选择后提交，或直接在回复里说明（团队可写名称，我来解析 ID；可见范围填「公开」或「仅团队可见」）。",
			"Before creating, set the managing team and visibility: pick in the form below and submit, or just tell me in your reply (team by name is fine—I'll resolve the ID; visibility = public or team-scoped).")
	}
	return &aiagent.ToolInterrupt{
		Kind:   aiagent.InterruptKindInput,
		Prompt: prompt,
		Form:   aiagent.BuildCreationForm(deps, user, lang, formSkillName, required, aiagent.FormPreselect{}),
	}
}

// authScopeLines 渲染「管理团队 / 可见范围」两行确认文案（团队名经 DB 反查，范围经
// lang 本地化），create_mcp_server 与 create_skill 的提案确认共用，让用户在确认前看到
// 资源最终归属与可见性。
func authScopeLines(deps *aiagent.ToolDeps, lang string, scope creationAuthScope) []string {
	sep := aiagent.LangText(lang, "、", ", ")
	visibility := aiagent.LangText(lang, "仅管理团队可见", "team-scoped (managing teams only)")
	if scope.Private == int(aiagent.VisibilityPublic) {
		visibility = aiagent.LangText(lang, "公开（所有人可见可用）", "public (visible to everyone)")
	}
	return []string{
		aiagent.LangText(lang, "管理团队：", "Managing teams: ") + strings.Join(teamNamesByIds(deps, scope.UserGroupIds), sep),
		aiagent.LangText(lang, "可见范围：", "Visibility: ") + visibility,
	}
}

// teamNamesByIds 把团队 ID 反查成名称（保持入参顺序）；查不到的 ID 退化为数字，
// 确认文案永不因一次查询失败而空缺。
func teamNamesByIds(deps *aiagent.ToolDeps, ids []int64) []string {
	if len(ids) == 0 {
		return nil
	}
	nameByID := map[int64]string{}
	if groups, err := models.UserGroupGetByIds(deps.DBCtx, ids); err == nil {
		for _, g := range groups {
			nameByID[g.Id] = g.Name
		}
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := nameByID[id]; ok {
			out = append(out, n)
		} else {
			out = append(out, fmt.Sprintf("%d", id))
		}
	}
	return out
}
