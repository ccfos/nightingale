package tools

import (
	"context"
	"fmt"
	"sort"
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
	register(defs.ListMCPServers, listMCPServers)
	register(defs.CreateMCPServer, createMCPServer)
	register(defs.UpdateMCPServer, updateMCPServer)
}

// mcpServerResult is the AI-surface projection of an MCP server. Header VALUES are
// deliberately absent — they carry credentials (Authorization: Bearer …) and the
// runtime reads them server-side from the DB, so the model never needs them; only
// the key names are surfaced, enough for the author to reason about an edit.
type mcpServerResult struct {
	Name           string   `json:"name"`
	URL            string   `json:"url"`
	Description    string   `json:"description,omitempty"`
	Enabled        bool     `json:"enabled"`
	AuthMode       string   `json:"auth_mode"`
	HeaderKeys     []string `json:"header_keys,omitempty"`
	Private        int      `json:"private"`
	UserGroupIds   []int64  `json:"user_group_ids"`
	CanManage      bool     `json:"can_manage"`
	OAuthConnected bool     `json:"oauth_connected"`
}

// listMCPServers returns the MCP servers the user may see, so the author can find
// the one to edit (and spot an oauth server that still needs authorizing).
func listMCPServers(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAIMCPServers); err != nil {
		return "", err
	}

	lst, err := models.MCPServerGets(deps.DBCtx)
	if err != nil {
		return "", fmt.Errorf("failed to query mcp servers: %v", err)
	}
	gids, err := getUserGroupIds(deps, user.Id)
	if err != nil {
		return "", fmt.Errorf("failed to load user groups: %v", err)
	}
	query := strings.TrimSpace(getArgString(args, "query"))
	results := make([]mcpServerResult, 0, len(lst))
	for _, s := range lst {
		if !s.CanBeUsedBy(user, gids) {
			continue
		}
		if query != "" && !containsIgnoreCase(s.Name, query) && !containsIgnoreCase(s.Description, query) {
			continue
		}
		results = append(results, mcpServerResult{
			Name:           s.Name,
			URL:            s.URL,
			Description:    s.Description,
			Enabled:        s.Enabled,
			AuthMode:       s.EffectiveAuthMode(),
			HeaderKeys:     headerKeys(s.Headers),
			Private:        s.Private,
			UserGroupIds:   s.UserGroupIds,
			CanManage:      s.CanBeManagedBy(user, gids),
			OAuthConnected: mcpOAuthConnected(deps, s),
		})
	}
	return marshalList(len(results), results), nil
}

// updateMCPServer patches an existing MCP server config (two-phase confirmed).
// Only the provided fields change. Unlike create it never pops the auth form —
// team/visibility move only when the user explicitly asks — but it enforces the
// same rules: non-admins may only authorize their own teams and may not change
// visibility at all.
func updateMCPServer(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAIMCPServers); err != nil {
		return "", err
	}

	name := strings.TrimSpace(getArgString(args, "name"))
	if name == "" {
		return "", fmt.Errorf("name is required: 要修改的 MCP Server 名称（用 list_mcp_servers 查）")
	}
	obj, err := models.MCPServerGetByName(deps.DBCtx, name)
	if err != nil {
		return "", fmt.Errorf("failed to load mcp server: %v", err)
	}
	gids, err := getUserGroupIds(deps, user.Id)
	if err != nil {
		return "", fmt.Errorf("failed to load user groups: %v", err)
	}
	// 不可见的 server 一律当作不存在：AI 工具面不能成为探测私有配置是否存在的旁路。
	if obj == nil || !obj.CanBeUsedBy(user, gids) {
		return fmt.Sprintf("MCP Server %q 不存在；如需新建请用 create_mcp_server。", name), nil
	}
	if !obj.CanBeManagedBy(user, gids) {
		return fmt.Sprintf("MCP Server %q 无权修改：仅管理团队成员可编辑。", name), nil
	}

	// 冲突基线必须在 merge 前取：确认腿据此拒绝已被他人改过的陈旧提案。
	baseline := updateBaselineHash(obj)

	ref := *obj
	var changes []string
	if v := strings.TrimSpace(getArgString(args, "new_name")); v != "" && v != obj.Name {
		ref.Name = v
		changes = append(changes, fmt.Sprintf("名称 → %s", v))
	}
	if v := strings.TrimSpace(getArgString(args, "url")); v != "" && v != obj.URL {
		ref.URL = v
		changes = append(changes, fmt.Sprintf("地址 → %s", v))
	}
	if _, ok := args["description"]; ok {
		if v := strings.TrimSpace(getArgString(args, "description")); v != obj.Description {
			ref.Description = v
			changes = append(changes, "用途描述")
		}
	}
	if _, ok := args["headers"]; ok {
		// oauth 模式的 server 根本不会带上这些头：mcp.Client.buildHTTPClient 在
		// AuthMode==oauth 时整个跳过 staticHeaders（Authorization 由 OAuthHandler 设）。
		// 存下去只会给用户一个「改成功了」的回执、而请求永远不带这些头——宁可明确拒绝。
		if obj.EffectiveAuthMode() == "oauth" {
			return "", fmt.Errorf("MCP Server %q 是 OAuth 授权模式，不支持自定义请求头：OAuth 模式下运行时只发送授权流程颁发的 Authorization，配置的请求头不会被携带。如确实需要自定义头，请到「AI 配置 → MCP」把认证方式改为「自定义 Header」", name)
		}
		h, herr := parseMCPHeaders(args)
		if herr != nil {
			return "", herr
		}
		ref.Headers = h
		ref.AuthMode = deriveMCPAuthMode(h)
		changes = append(changes, fmt.Sprintf("请求头 → %d 项（整体替换）", len(h)))
	}
	if _, ok := args["enabled"]; ok {
		if v := getArgBool(args, "enabled"); v != obj.Enabled {
			ref.Enabled = v
			changes = append(changes, fmt.Sprintf("启用 → %v", v))
		}
	}
	if _, ok := args["team_ids"]; ok {
		teams, terr := argInt64Slice(args, "team_ids")
		if terr != nil {
			return "", terr
		}
		if len(teams) == 0 {
			return "", fmt.Errorf("team_ids 不能为空：MCP Server 必须有管理团队")
		}
		// 与 create 同一条越权闸：非 admin 只能授权自己所属团队（子集），既防越权授权
		// 给外部团队，也防把自己踢出管理范围。
		if !user.IsAdmin() && !int64SliceSubset(gids, teams) {
			return "", fmt.Errorf("forbidden: you can only authorize teams you belong to")
		}
		ref.UserGroupIds = teams
		changes = append(changes, "管理团队 → "+strings.Join(teamNamesByIds(deps, teams), "、"))
	}
	if _, ok := args["private"]; ok {
		p, present, perr := argIntPresent(args, "private")
		if perr != nil {
			return "", perr
		}
		if present {
			// 可见范围是管理员特权（与 create 一致：非 admin 建的资源恒为仅团队可见）。
			if !user.IsAdmin() {
				return "", fmt.Errorf("forbidden: only admins can change the visibility of an mcp server")
			}
			if p != int(aiagent.VisibilityPublic) && p != int(aiagent.VisibilityTeamScope) {
				return "", fmt.Errorf("private flag must be 0 or 1")
			}
			if p != obj.Private {
				ref.Private = p
				changes = append(changes, "可见范围 → "+visibilityLabel(params["lang"], p))
			}
		}
	}

	if len(changes) == 0 {
		return "", fmt.Errorf("nothing to update: 没有提供任何要修改的字段（可改 new_name/url/description/headers/enabled/team_ids/private）")
	}

	ref.UpdatedBy = user.Username
	if err := ref.Verify(); err != nil {
		return "", err
	}

	if !getArgBool(args, "confirmed") {
		return proposeUpdate(ctx, deps, params, &updateProposal{
			Kind:         proposalKindMCP,
			TargetID:     obj.Id,
			BaselineHash: baseline,
			Changes:      changes,
		}, renderUpdateProposalPrompt(params["lang"], fmt.Sprintf("MCP Server **%s**", obj.Name), changes), args)
	}
	if _, err := confirmUpdateGate(ctx, deps, params, "update_mcp_server", proposalKindMCP, obj.Id, getArgString(args, "proposal_id"), true, baseline); err != nil {
		return "", err
	}

	if err := obj.Update(deps.DBCtx, ref); err != nil {
		return "", fmt.Errorf("failed to update mcp server: %v", err)
	}
	logger.Infof("update_mcp_server: user=%s name=%s id=%d changes=%v", user.Username, obj.Name, obj.Id, changes)

	return fmt.Sprintf("已更新 MCP Server **%s**。\n\n你可以在「AI 配置 → MCP」里查看它。", ref.Name), nil
}

// mcpOAuthConnected reports whether an oauth server's authorization is usable,
// via the host-injected checker (same definition the runtime assembly and the
// status API use — it actually decrypts the token rather than trusting a non-empty
// column). Non-oauth servers are never "connected"; a missing checker (CLI/tests)
// reads as not connected, so we never claim an unverified server works.
func mcpOAuthConnected(deps *aiagent.ToolDeps, s *models.MCPServer) bool {
	if s.EffectiveAuthMode() != "oauth" || deps.MCPOAuthUsable == nil {
		return false
	}
	return deps.MCPOAuthUsable(s.Id)
}

// headerKeys returns the sorted header names (values withheld — they're secrets).
func headerKeys(h map[string]string) []string {
	if len(h) == 0 {
		return nil
	}
	out := make([]string, 0, len(h))
	for k := range h {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// deriveMCPAuthMode picks the auth mode implied by the headers: bearing auth
// headers means "header" mode, otherwise "none". oauth is never derived — it
// requires the separate authorization flow (see router_mcp_server_oauth.go).
func deriveMCPAuthMode(headers map[string]string) string {
	if len(headers) > 0 {
		return "header"
	}
	return "none"
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
	// 提案生成前就校验地址：obj.Verify() 要等到确认腿才跑，不在这里拦的话用户会先
	// 批准一个注定写不进去的提案。与 MCPServer.Verify 同一份校验。
	if err := models.ValidateMCPServerURL(url); err != nil {
		return "", err
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
	scope, authIntr, err := resolveCreationAuth(deps, user, args, params, "mcp-server-copilot")
	if err != nil {
		return "", err
	}
	if authIntr != nil {
		return "", authIntr
	}

	// auth_mode 由 headers 是否存在推导。oauth 模式需独立的授权连接流程
	// （见 router_mcp_server_oauth.go），不在对话创建路径内暴露。
	authMode := deriveMCPAuthMode(headers)

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
	// 明确点出「还要绑定」：创建只是注册，运行时只加载绑定到当前 Agent 的 server
	// （buildMCPConfigForAgent 只看 agent.MCPServerIds），不说的话用户会等一个永远
	// 不出现的工具。本工具无权代改 Agent 配置，只能指路。
	return fmt.Sprintf("已创建 MCP Server **%s**（已启用：%v）。\n\n"+
		"注意：注册完成后它还**没有绑定到任何 AI Agent**，其工具暂时不会出现在对话里。"+
		"请到「AI 配置 → Agent」里把它绑定到需要用它的 Agent 上；也可以在「AI 配置 → MCP」里查看和管理它。", name, enabled)
}
