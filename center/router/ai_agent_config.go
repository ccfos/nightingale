package router

import (
	"sync"

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
// The second return value lists the bound servers that are waiting on their OAuth
// authorization. They're reported instead of being silently dropped: their tools
// would just vanish from the turn and the user would have no way to learn that the
// fix is "go authorize it". The caller surfaces them as an authorize button.
//
// A server is reported only to users who can actually complete the authorization,
// which takes BOTH conditions the /mcp-server-oauth/prepare route enforces:
// membership of a managing team AND the /ai-config/mcp-servers RBAC permission.
// The team check alone is not enough — UserGroupIds doubles as the visibility
// scope, so every member of an owning team passes it, but the chat entry point
// requires no RBAC permission at all; handing those users a button would only
// yield a 403 they can't act on.
func (rt *Router) buildMCPConfigForAgent(agent *models.AIAgent, me *models.User) (*mcp.Config, *mcpOAuthWatch) {
	watch := newMCPOAuthWatch()
	if len(agent.MCPServerIds) == 0 {
		return nil, watch
	}
	servers, err := models.MCPServersByIds(rt.Ctx, agent.MCPServerIds)
	if err != nil {
		logger.Warningf("[AIAgent] load mcp servers for agent=%d failed: %v", agent.Id, err)
		return nil, watch
	}
	if len(servers) == 0 {
		return nil, watch
	}

	// Resolve the user's team ids once, only when a non-admin actually needs the
	// check (admins short-circuit in mcpCanManage).
	var gids []int64
	if me != nil && !me.IsAdmin() {
		gids, err = models.MyGroupIds(rt.Ctx, me.Id)
		if err != nil {
			logger.Warningf("[AIAgent] load group ids for user=%d failed: %v", me.Id, err)
			return nil, watch
		}
	}

	// Resolved once, outside the loop: the same RBAC gate /mcp-server-oauth/prepare
	// hangs off (rt.perm). CheckPerm short-circuits to true for admins.
	canPrepareOAuth := false
	if me != nil {
		if ok, perr := me.CheckPerm(rt.Ctx, "/ai-config/mcp-servers"); perr != nil {
			logger.Warningf("[AIAgent] check mcp-servers perm for user=%d failed: %v", me.Id, perr)
		} else {
			canPrepareOAuth = ok
		}
	}

	out := make([]mcp.ServerConfig, 0, len(servers))
	for _, s := range servers {
		if !mcpCanUse(me, gids, s) {
			logger.Infof("[AIAgent] skip private mcp server=%s id=%d for agent=%d: user has no team access", s.Name, s.Id, agent.Id)
			continue
		}
		isOAuth := s.EffectiveAuthMode() == mcp.MCPAuthOAuth
		canAuthorize := isOAuth && canPrepareOAuth && mcpCanManage(me, gids, s)

		// Local pre-check, same definition the status API and list_mcp_servers use:
		// missing record, empty token, undecryptable ciphertext, or expired with no
		// refresh token. It is only a pre-check — a revoked-but-decryptable token
		// passes here and is caught at connect time by the watch wired below.
		if isOAuth {
			if uerr := rt.mcpOAuthUsable(s.Id); uerr != nil {
				if canAuthorize {
					watch.track(s, true)
				}
				logger.Infof("[AIAgent] mcp server=%s id=%d awaits oauth authorization: %v", s.Name, s.Id, uerr)
				continue
			}
		}
		cfg, credVer, cerr := rt.mcpServerConfig(s)
		if cerr != nil {
			// A malformed config unrelated to oauth — skip rather than fail the run.
			logger.Warningf("[AIAgent] skip mcp server=%s id=%d: %v", s.Name, s.Id, cerr)
			continue
		}
		// Runtime watch: the credential looks fine locally, but the provider may
		// still reject it (revoked token, refresh answered invalid_grant). That only
		// surfaces once the MCP client connects/refreshes — inside the agent run,
		// after this function returned. Wire a callback so such a server still lands
		// in the OAuth card instead of having its tools silently disappear.
		// cfg is copied into `out` by value, but OAuth is a pointer, so the callback
		// survives.
		if cfg.OAuth != nil && canAuthorize {
			sid := s.Id
			name := s.Name
			watch.track(s, false)
			// 非破坏性：resource server 回 401/403 只说明「本轮用不了」，原因可能与凭据
			// 无关（scope 不足、网关、IP 白名单）。只弹按钮；下一轮本地预检会自愈。
			cfg.OAuth.OnAuthRequired = func(reason error) {
				if watch.markFailed(sid) {
					logger.Infof("[AIAgent] mcp server=%s id=%d rejected with 401/403, surfacing authorize button: %v", name, sid, reason)
				}
			}
			// 破坏性：只有 token endpoint 回明确的 OAuth 错误码才走到这里。凭据版本
			// credVer 来自「产出本 cfg 的那一次读取」（且随 refresh 轮换），在失败时刻取
			// 值做条件更新——避免把并发授权刚存下的新 token 抹掉。
			cfg.OAuth.OnCredentialInvalid = func(kind mcp.CredentialInvalidKind, reason error) {
				watch.markFailed(sid)
				clearClient := kind == mcp.CredentialInvalidClient
				logger.Infof("[AIAgent] mcp server=%s id=%d credential definitively rejected (clear_client=%v): %v", name, sid, clearClient, reason)
				rt.invalidateMCPOAuthCredential(sid, credVer.get(), clearClient)
			}
		}
		out = append(out, *cfg)
	}
	if len(out) == 0 {
		return nil, watch
	}
	return &mcp.Config{Servers: out}, watch
}

// mcpOAuthWatch collects the MCP servers whose OAuth authorization the user must
// (re)establish — both the ones a local pre-check already ruled out, and the ones
// the provider only rejects once the client actually connects. Only servers the
// caller may actually authorize are tracked, so the card never offers a button
// that would 403. Safe for concurrent marking: the token source may be exercised
// from the SDK's goroutines.
type mcpOAuthWatch struct {
	mu   sync.Mutex
	seen []*models.MCPServer // stable order = the agent's binding order
	need map[int64]struct{}
}

func newMCPOAuthWatch() *mcpOAuthWatch {
	return &mcpOAuthWatch{need: make(map[int64]struct{})}
}

// track registers a server the caller could authorize. needNow marks it as already
// known to need authorization (the local pre-check failed); otherwise it's merely
// a candidate that markFailed may promote later.
func (w *mcpOAuthWatch) track(s *models.MCPServer, needNow bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.seen = append(w.seen, s)
	if needNow {
		w.need[s.Id] = struct{}{}
	}
}

// markFailed records a runtime credential rejection, reporting whether this is the
// first one for the server so the caller invalidates the stored token only once.
func (w *mcpOAuthWatch) markFailed(serverId int64) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, dup := w.need[serverId]; dup {
		return false
	}
	w.need[serverId] = struct{}{}
	return true
}

// servers returns the servers needing (re)authorization, in binding order. Called
// after the agent run so runtime-detected failures are included.
func (w *mcpOAuthWatch) servers() []*models.MCPServer {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]*models.MCPServer, 0, len(w.need))
	for _, s := range w.seen {
		if _, ok := w.need[s.Id]; ok {
			out = append(out, s)
		}
	}
	return out
}

// invalidateMCPOAuthCredential clears the stored credential of a server the token
// endpoint definitively rejected. Without it the local checks (status API,
// list_mcp_servers, the next turn's pre-check) would go on reporting "connected"
// off material we already know is dead.
//
// clearClient follows the verdict: invalid_grant kills only the grant (keep the
// client so re-consent can reuse it — a non-DCR server could not be re-registered),
// while invalid_client rejects the client itself and keeping it would make every
// subsequent authorize attempt fail the same way.
//
// usedToken scopes the write to the exact credential version that failed, so a late
// rejection carrying an old token can't wipe one a concurrent re-authorization just
// saved.
func (rt *Router) invalidateMCPOAuthCredential(serverId int64, usedToken string, clearClient bool) {
	if usedToken == "" {
		return
	}
	n, err := models.MCPServerOAuthInvalidateTokens(rt.Ctx, serverId, usedToken, clearClient)
	if err != nil {
		logger.Warningf("[AIAgent] invalidate oauth credential for server=%d failed: %v", serverId, err)
		return
	}
	if n == 0 {
		logger.Infof("[AIAgent] skip invalidating oauth credential for server=%d: a newer authorization already replaced it", serverId)
	}
}
