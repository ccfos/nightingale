package router

import (
	"github.com/ccfos/nightingale/v6/aiagent"
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
