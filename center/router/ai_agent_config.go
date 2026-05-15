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
//   - Empty SkillIds → AutoSelect across the full registry (preserves the
//     existing behaviour so no migration is forced on agents set up before the
//     binding feature landed).
//   - Non-empty SkillIds → explicit SkillNames list. We turn AutoSelect off so
//     the agent deterministically loads exactly what the operator bound. This
//     is the mode that makes the per-agent binding actually mean something.
//
// Disabled skills silently drop out of the result (see AISkillsByIds). The
// fallback when all bound skills are disabled is "no skills" rather than
// "autoselect across everything" — preserving operator intent.
func (rt *Router) buildSkillConfigForAgent(agent *models.AIAgent) *aiagent.SkillConfig {
	cfg := &aiagent.SkillConfig{MaxSkills: 2}
	if len(agent.SkillIds) == 0 {
		cfg.AutoSelect = true
		return cfg
	}

	names, err := models.AISkillNamesByIds(rt.Ctx, agent.SkillIds)
	if err != nil {
		// On DB error, fall back to AutoSelect instead of sending the user a
		// "no skills at all" experience. Log loud so the failure is visible.
		logger.Warningf("[AIAgent] load skill names for agent=%d failed, falling back to autoselect: %v", agent.Id, err)
		cfg.AutoSelect = true
		return cfg
	}
	cfg.SkillNames = names
	return cfg
}

// resolveSkillConfig 决定本次 chat 用哪份 SkillConfig，优先级从高到低：
//
//  1. action handler 的 RequiredSkills——前端入口已经把意图分流到具体 action 时，
//     对应 skill 几乎确定（如 notify_template_generator → n9e-generate-message-template），
//     直接走 SkillNames 路径，跳过 LLM autoselect 那一次 2-5s round-trip。
//     handler 返回 nil 切片也算"明确表态：本次不需要 skill"，依然跳过 autoselect，
//     好让 system prompt 保持精简。区别在于"未声明 RequiredSkills 字段"——那时落到第 2 档。
//  2. agent 自带 SkillIds——运维在 agent 配置里显式绑定的 skill。
//  3. 兜底——AutoSelect = true，让 LLM 在全量 registry 里挑。
//
// 设计取舍：action 的 RequiredSkills 覆盖 agent 绑定。理由是 action 反映"业务路径需要什么"
// （代码事实），而 agent 绑定是"运维允许用哪些"（策略偏好）。前端已经把用户引到这个 action
// 入口（如 FlashAI 按钮），说明用户就是要做这件事；若 agent 不该干这事，应该在前端禁用入口，
// 不该靠 skill scope 兜底。
func (rt *Router) resolveSkillConfig(handler *chat.ActionHandler, req *chat.AIChatRequest, agent *models.AIAgent) *aiagent.SkillConfig {
	if handler != nil && handler.RequiredSkills != nil {
		names := handler.RequiredSkills(req)
		logger.Debugf("[Assistant] action %q declared RequiredSkills=%v, bypass autoselect", req.ActionKey, names)
		return &aiagent.SkillConfig{
			SkillNames: names,
			AutoSelect: false,
			MaxSkills:  2,
		}
	}
	return rt.buildSkillConfigForAgent(agent)
}

// buildMCPConfigForAgent translates agent.MCPServerIds into mcp.Config.
//
// Returns nil (not an empty Config) when the agent has no MCP bindings — the
// Agent's applyDefaults checks `cfg.MCP != nil && len(cfg.MCP.Servers) > 0`
// before standing up the MCP client manager, and we preserve that shortcut.
//
// Transport defaults to "sse": the DB model only stores URL/Headers, which is
// the SSE shape. A future stdio-capable MCPServer model can diverge here.
func (rt *Router) buildMCPConfigForAgent(agent *models.AIAgent) *mcp.Config {
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

	out := make([]mcp.ServerConfig, 0, len(servers))
	for _, s := range servers {
		out = append(out, mcp.ServerConfig{
			Name:      s.Name,
			Transport: mcp.MCPTransportSSE,
			URL:       s.URL,
			Headers:   s.Headers,
		})
	}
	return &mcp.Config{Servers: out}
}
