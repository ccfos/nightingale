package router

import (
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/chat"
	"github.com/ccfos/nightingale/v6/aiagent/mcp"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
)

// buildSkillConfigForAgent translates agent.SkillIds into an aiagent.SkillConfig.
//
// Semantics:
//   - Empty SkillIds → no preload. The system prompt carries the always-present
//     skill catalog and the model self-loads via load_skill (渐进披露，无 LLM 预选).
//   - Non-empty SkillIds → explicit SkillNames list: the agent deterministically
//     preloads exactly what the operator bound. This is the mode that makes the
//     per-agent binding actually mean something.
//
// Disabled skills silently drop out of the result (see AISkillsByIds). The
// fallback when all bound skills are disabled is "no skills preloaded" rather
// than re-deriving anything — preserving operator intent; the catalog path
// still lets the model pull what it needs.
func (rt *Router) buildSkillConfigForAgent(agent *models.AIAgent) *aiagent.SkillConfig {
	cfg := &aiagent.SkillConfig{}
	if len(agent.SkillIds) == 0 {
		return cfg
	}

	names, err := models.AISkillNamesByIds(rt.Ctx, agent.SkillIds)
	if err != nil {
		// On DB error, fall back to the catalog path instead of failing the
		// turn. Log loud so the failure is visible.
		logger.Warningf("[AIAgent] load skill names for agent=%d failed, falling back to on-demand catalog: %v", agent.Id, err)
		return cfg
	}
	cfg.SkillNames = names
	return cfg
}

// resolveSkillConfig 决定本次 chat 用哪份 SkillConfig，优先级从高到低：
//
//  1. action handler 的 RequiredSkills——handler 返回 nil/空切片是"明确表态：
//     本次不预载 skill"（如 general_chat 防 agent 绑定 skill 膨胀默认路径
//     prompt），直接走 SkillNames。区别在于"未声明 RequiredSkills 字段"
//     （如 creation）——那时落到第 2 档。
//  2. agent 自带 SkillIds——运维在 agent 配置里显式绑定的 skill。
//  3. 兜底——不预载，目录常驻 + load_skill 模型自取（见 buildSkillConfigForAgent）。
//
// 设计取舍：action 的 RequiredSkills 覆盖 agent 绑定。理由是 action 反映"业务路径
// 需要什么"（代码事实），而 agent 绑定是"运维允许用哪些"（策略偏好）。
func (rt *Router) resolveSkillConfig(handler *chat.ActionHandler, req *chat.AIChatRequest, agent *models.AIAgent) *aiagent.SkillConfig {
	if handler != nil && handler.RequiredSkills != nil {
		names := handler.RequiredSkills(req)
		logger.Debugf("[Assistant] action %q declared RequiredSkills=%v, pinned preload", req.ActionKey, names)
		return &aiagent.SkillConfig{SkillNames: names}
	}
	return rt.buildSkillConfigForAgent(agent)
}

// hiddenSkillNamesForUser 算出请求用户在 AI 对话里无权访问的私有 skill 名字，用于
// 目录展示与 load_skill / run_skill_script / get_skill 加载层的统一拦截——私有
// skill 只对授权团队可见/可用。
//
// 返回 denyAll=true 表示无法算出隐藏名单（fail closed 兜底）：调用方应据此拒绝本轮
// 所有 skill（目录留空 + 拒绝所有按名加载/执行）。
//
// Fail closed 分层：只有「确定是管理员」才返回 (nil,false)（放行全部）；解析用户 /
// 团队的任一步出错或用户不存在，都以空团队集兜底（AISkillHiddenNames(nil) 会把所有
// 私有 skill 计为隐藏）；连私有 skill 列表本身都查不出来时，返回 denyAll=true，绝不
// 把「查询失败」误当成「不过滤」而泄露私有 skill。
func (rt *Router) hiddenSkillNamesForUser(userId int64) (hidden []string, denyAll bool) {
	me, err := models.UserGetById(rt.Ctx, userId)
	if err == nil && me != nil && me.IsAdmin() {
		return nil, false
	}

	var gids []int64
	switch {
	case err != nil:
		logger.Errorf("[AIAgent] load user=%d failed, hiding all private skills: %v", userId, err)
	case me == nil:
		logger.Warningf("[AIAgent] user=%d not found, hiding all private skills", userId)
	default:
		if g, gerr := models.MyGroupIds(rt.Ctx, userId); gerr != nil {
			logger.Errorf("[AIAgent] load groups for user=%d failed, hiding all private skills: %v", userId, gerr)
		} else {
			gids = g
		}
	}

	names, herr := models.AISkillHiddenNames(rt.Ctx, gids)
	if herr != nil {
		// 无法枚举私有 skill：fail closed —— 本轮拒绝所有 skill，而不是放行。
		logger.Errorf("[AIAgent] list private skills for user=%d failed, denying all skills this turn: %v", userId, herr)
		return nil, true
	}
	return names, false
}

// buildMCPConfigForAgent translates agent.MCPServerIds into mcp.Config for the
// chatting user `me`.
//
// Returns nil (not an empty Config) when the agent has no usable MCP bindings —
// the Agent's applyDefaults checks `cfg.MCP != nil && len(cfg.MCP.Servers) > 0`
// before standing up the MCP client manager, and we preserve that shortcut.
//
// Team scope (same rule as the management list): the user may only use public
// servers plus those a team they belong to owns; admins may use all. A private
// server bound to an agent is silently dropped for users without access so the
// agent can't leak its tools to users who couldn't otherwise see it — the agent
// therefore exposes different tools to different users, by design.
func (rt *Router) buildMCPConfigForAgent(agent *models.AIAgent, me *models.User) *mcp.Config {
	if len(agent.MCPServerIds) == 0 {
		return nil
	}
	servers, err := models.MCPServersByIds(rt.Ctx, agent.MCPServerIds)
	if err != nil {
		logger.Warningf("[AIAgent] load mcp servers for agent=%d failed: %v", agent.Id, err)
		return nil
	}
	if len(servers) == 0 {
		return nil
	}

	// Resolve the user's team ids once, only when a non-admin actually needs the
	// check (admins short-circuit in mcpCanManage).
	var gids []int64
	if me != nil && !me.IsAdmin() {
		gids, err = models.MyGroupIds(rt.Ctx, me.Id)
		if err != nil {
			logger.Warningf("[AIAgent] load group ids for user=%d failed: %v", me.Id, err)
			return nil
		}
	}

	out := make([]mcp.ServerConfig, 0, len(servers))
	for _, s := range servers {
		if !mcpCanUse(me, gids, s) {
			logger.Infof("[AIAgent] skip private mcp server=%s id=%d for agent=%d: user has no team access", s.Name, s.Id, agent.Id)
			continue
		}
		cfg, cerr := rt.mcpServerConfig(s)
		if cerr != nil {
			// e.g. an oauth server that hasn't been connected yet — skip it
			// rather than failing the whole agent.
			logger.Warningf("[AIAgent] skip mcp server=%s id=%d: %v", s.Name, s.Id, cerr)
			continue
		}
		out = append(out, *cfg)
	}
	if len(out) == 0 {
		return nil
	}
	return &mcp.Config{Servers: out}
}
