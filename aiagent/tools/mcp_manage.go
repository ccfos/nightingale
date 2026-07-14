package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
)

// =============================================================================
// MCP server authoring tool — let the agent register an external MCP server
// mid-conversation, mirroring the skill authoring tools (skill_manage.go).
//
// Persistence reuses the exact model layer the HTTP route uses (MCPServer.Create),
// so the AI surface and the management UI converge on one write path. Writes are
// doubly guarded: the /ai-config/mcp-servers permission (same as the HTTP route)
// AND the shared two-phase confirmation gate (proposeUpdate / confirmUpdateGate).
//
// The managing team + visibility are collected via resolveCreationAuth
// (form_gate.go), shared with create_skill: non-admins pick only a managing team
// (visibility fixed to team-scoped), admins pick both team and visibility.
// =============================================================================

// PermAIMCPServers mirrors the rt.perm("/ai-config/mcp-servers") guard on the
// HTTP MCP server CRUD routes — registering a server the agent will call is a
// privileged operation, held to the same bar as the management UI.
const PermAIMCPServers = "/ai-config/mcp-servers"

// proposalKindMCP namespaces MCP server proposals in the shared confirmation gate.
const proposalKindMCP = "mcp_server"

func init() {
	register(defs.CreateMCPServer, createMCPServer)
}

// createMCPServer persists a brand-new MCP server config (two-phase confirmed).
func createMCPServer(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAIMCPServers); err != nil {
		return "", err
	}

	name := strings.TrimSpace(getArgString(args, "name"))
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	url := strings.TrimSpace(getArgString(args, "url"))
	if url == "" {
		return "", fmt.Errorf("url is required: MCP Server 的 http/https 访问地址")
	}
	description := strings.TrimSpace(getArgString(args, "description"))
	headers, err := parseMCPHeaders(args)
	if err != nil {
		return "", err
	}
	enabled := true
	if _, ok := args["enabled"]; ok {
		enabled = getArgBool(args, "enabled")
	}

	// Pre-gate collision check so we never propose a create that can't land.
	if exist, err := models.MCPServerGetByName(deps.DBCtx, name); err != nil {
		return "", fmt.Errorf("failed to check existing mcp server: %v", err)
	} else if exist != nil {
		return "", fmt.Errorf("MCP Server %q 已存在；换个名字，或去「AI 配置 → MCP」编辑它", name)
	}

	// 管理团队 + 可见范围收集：非 admin 只选团队（可见范围恒为仅团队可见），admin 二者皆选。
	// 信息不全时中止本轮弹表单，与 create_skill 共用同一条授权路径。
	scope, authIntr, err := resolveCreationAuth(deps, user, args, params, "mcp-server")
	if err != nil {
		return "", err
	}
	if authIntr != nil {
		return "", authIntr
	}

	// auth_mode 由 headers 是否存在推导：带鉴权头即 header 模式，否则 none。oauth 模式需
	// 独立的授权连接流程（见 router_mcp_server_oauth.go），不在对话创建路径内暴露。
	authMode := "none"
	if len(headers) > 0 {
		authMode = "header"
	}

	// Two-phase confirmation. Propose leg ends the turn with a deterministic
	// approval prompt; the confirm leg is replayed with confirmed=true (zero LLM
	// involvement, team/visibility carried through the replayed params).
	if !getArgBool(args, "confirmed") {
		prompt := renderMCPProposal(deps, params["lang"], name, url, description, enabled, scope)
		return proposeUpdate(ctx, deps, params, &updateProposal{
			Kind:         proposalKindMCP,
			TargetID:     0,
			BaselineHash: "",
			Changes:      []string{"create mcp server " + name},
		}, prompt, args)
	}
	if _, err := confirmUpdateGate(ctx, deps, params, "create_mcp_server", proposalKindMCP, 0, getArgString(args, "proposal_id"), true, ""); err != nil {
		return "", err
	}

	obj := models.MCPServer{
		Name:         name,
		URL:          url,
		Headers:      headers,
		Description:  description,
		Enabled:      enabled,
		AuthMode:     authMode,
		UserGroupIds: scope.UserGroupIds,
		Private:      scope.Private,
		CreatedBy:    user.Username,
		UpdatedBy:    user.Username,
	}
	if err := obj.Verify(); err != nil {
		return "", err
	}
	if err := obj.Create(deps.DBCtx); err != nil {
		return "", fmt.Errorf("failed to create mcp server: %v", err)
	}

	logger.Infof("create_mcp_server: user=%s name=%s id=%d teams=%v private=%d", user.Username, name, obj.Id, obj.UserGroupIds, obj.Private)

	return renderMCPResult(name, obj.Enabled), nil
}

// parseMCPHeaders coerces the headers arg (a JSON object of string→string) into a
// map. Non-string values are stringified so a header the model quoted as a number
// still lands as text.
func parseMCPHeaders(args map[string]interface{}) (map[string]string, error) {
	raw, ok := args["headers"]
	if !ok || raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("headers 必须是对象（键值对），如 {\"Authorization\": \"Bearer xxx\"}")
	}
	if len(m) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = asString(v)
	}
	return out, nil
}

// renderMCPProposal builds the deterministic confirmation copy shown before an
// MCP server is created — the tool renders it, never the model, so what the user
// approves is exactly what will be written.
func renderMCPProposal(deps *aiagent.ToolDeps, lang, name, url, description string, enabled bool, scope creationAuthScope) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("即将创建 MCP Server **%s**：\n", name))
	sb.WriteString(fmt.Sprintf("\n- 地址：%s", url))
	if description != "" {
		sb.WriteString(fmt.Sprintf("\n- 用途：%s", truncateRunes(description, 200)))
	}
	sb.WriteString(fmt.Sprintf("\n- 启用：%v", enabled))
	for _, line := range authScopeLines(deps, lang, scope) {
		sb.WriteString("\n- ")
		sb.WriteString(line)
	}
	sb.WriteString("\n\n以上尚未写入。回复「确认」立即生效，回复「取消」放弃，也可以直接提出调整。")
	return sb.String()
}

func renderMCPResult(name string, enabled bool) string {
	return fmt.Sprintf("已创建 MCP Server **%s**（已启用：%v）。\n\n你可以在「AI 配置 → MCP」里查看和管理它。", name, enabled)
}
