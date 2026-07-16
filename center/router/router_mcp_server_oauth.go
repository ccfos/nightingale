package router

// Outbound MCP client OAuth 2.1 (Authorization Code + PKCE).
//
// n9e is the OAuth *client* here (dialing external OAuth-protected MCP servers).
// The interactive authorization is server-mediated:
//   prepare  (protected) → discover + DCR/manual → returns authorize_url + state
//   callback (public)    → exchange code → persist encrypted tokens → postMessage
// At agent runtime the persisted tokens are loaded by mcpServerConfig and the SDK
// transport injects/refreshes the Bearer automatically. See router_mcp_oauth.go
// for the opposite direction (n9e as MCP *server* / Authorization Server).

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/mcp"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/secu"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
	"golang.org/x/oauth2"
)

const (
	mcpOAuthStatePrefix        = "/mcp-client-oauth/state/"
	mcpOAuthStateTTL           = 10 * time.Minute
	mcpServerOAuthCallbackPath = "/api/n9e/mcp-server-oauth/callback"
)

// mcpOAuthState is the transient, Redis-stored context that survives the
// prepare → browser → callback hops (keyed by an opaque `state`).
type mcpOAuthState struct {
	ServerId     int64              `json:"server_id"`
	Endpoints    mcp.OAuthEndpoints `json:"endpoints"`
	ClientID     string             `json:"client_id"`
	ClientSecret string             `json:"client_secret"`
	RedirectURI  string             `json:"redirect_uri"`
	Scope        string             `json:"scope"`
	Verifier     string             `json:"verifier"`
	ConnectedBy  string             `json:"connected_by"`
}

// ---- encryption helpers (symmetric AES at-rest) ----
//
// OAuth access/refresh tokens are JWTs (hundreds of bytes) that exceed RSA
// PKCS#1 v1.5's ~245-byte block limit, so — unlike the short git PAT — they must
// be encrypted with a symmetric cipher. We reuse secu's AES-CBC helper (the same
// one used for config secrets) with a key derived from the MCP/session JWT
// signing key, a secret that is always present when MCPAuth is enabled.

// mcpSecretAESKey returns a stable 32-byte AES-256 key (raw sha256 digest — NOT
// hex, since aes.NewCipher requires a 16/24/32-byte key).
func (rt *Router) mcpSecretAESKey() string {
	src := strings.TrimSpace(rt.HTTP.MCPAuth.SigningKey)
	if src == "" {
		src = rt.HTTP.JWTAuth.SigningKey
	}
	sum := sha256.Sum256([]byte(src))
	return string(sum[:])
}

func (rt *Router) encryptMCPSecret(v string) (string, error) {
	if v == "" || strings.HasPrefix(v, "{{cipher}}") {
		return v, nil
	}
	return secu.DealWithEncrypt(v, rt.mcpSecretAESKey())
}

func (rt *Router) decryptMCPSecret(v string) (string, error) {
	if v == "" || !strings.HasPrefix(v, "{{cipher}}") {
		return v, nil
	}
	return secu.DealWithDecrypt(v, rt.mcpSecretAESKey())
}

// ---- runtime config: DB row -> mcp.ServerConfig (loads+decrypts OAuth) ----

// mcpCredVersion pins the exact stored access-token ciphertext a running MCP
// client is using, so a later "this credential is dead" verdict can be applied
// with a conditional UPDATE that touches only that version. It MUST be captured
// from the same DB read that produced the OAuthConfig — a second query could
// straddle a concurrent OAuth callback and pin a token the client never used,
// which is precisely how a stale rejection ends up wiping a fresh authorization.
// Refreshes rotate the stored ciphertext, so persistRefreshedMCPToken moves the
// pin along with them; otherwise a genuinely dead credential would stop matching
// and never get cleaned up.
type mcpCredVersion struct {
	mu     sync.Mutex
	cipher string
}

func (v *mcpCredVersion) get() string {
	if v == nil {
		return ""
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cipher
}

func (v *mcpCredVersion) set(cipher string) {
	if v == nil {
		return
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	v.cipher = cipher
}

// mcpServerConfig builds the client config. For oauth servers it also returns the
// credential version pinned by the same read (nil for non-oauth servers).
func (rt *Router) mcpServerConfig(obj *models.MCPServer) (*mcp.ServerConfig, *mcpCredVersion, error) {
	cfg := &mcp.ServerConfig{
		Name:      obj.Name,
		Transport: mcp.MCPTransportHTTP,
		URL:       obj.URL,
		Headers:   obj.Headers,
		AuthMode:  obj.EffectiveAuthMode(),
	}
	if cfg.AuthMode == mcp.MCPAuthOAuth {
		oc, ver, err := rt.loadMCPOAuthConfig(obj.Id)
		if err != nil {
			return nil, nil, err
		}
		cfg.OAuth = oc
		return cfg, ver, nil
	}
	return cfg, nil, nil
}

func (rt *Router) loadMCPOAuthConfig(serverId int64) (*mcp.OAuthConfig, *mcpCredVersion, error) {
	rec, err := models.MCPServerOAuthGetByServerId(rt.Ctx, serverId)
	if err != nil {
		return nil, nil, err
	}
	if rec == nil || rec.AccessToken == "" {
		return nil, nil, fmt.Errorf("mcp server is not connected; complete the OAuth authorization first")
	}
	sec, err := rt.decryptMCPSecret(rec.ClientSecret)
	if err != nil {
		return nil, nil, err
	}
	acc, err := rt.decryptMCPSecret(rec.AccessToken)
	if err != nil {
		return nil, nil, err
	}
	ref, err := rt.decryptMCPSecret(rec.RefreshToken)
	if err != nil {
		return nil, nil, err
	}
	var expiry time.Time
	if rec.Expiry > 0 {
		expiry = time.Unix(rec.Expiry, 0)
	}
	// Pinned from THIS read — the very record whose token we're about to hand to
	// the client. No second query.
	ver := &mcpCredVersion{cipher: rec.AccessToken}
	return &mcp.OAuthConfig{
		Endpoints: mcp.OAuthEndpoints{
			Issuer:                rec.Issuer,
			AuthorizationEndpoint: rec.AuthorizationEndpoint,
			TokenEndpoint:         rec.TokenEndpoint,
			RegistrationEndpoint:  rec.RegistrationEndpoint,
			Resource:              rec.Resource,
		},
		ClientID:     rec.ClientId,
		ClientSecret: sec,
		RedirectURI:  rec.RedirectURI,
		Scope:        rec.Scope,
		AccessToken:  acc,
		RefreshToken: ref,
		TokenType:    rec.TokenType,
		Expiry:       expiry,
		OnRefresh:    func(t *oauth2.Token) { rt.persistRefreshedMCPToken(serverId, t, ver) },
	}, ver, nil
}

// persistRefreshedMCPToken writes a rotated access/refresh token back to the DB
// (encrypted). Invoked by the token source whenever the SDK refreshes.
func (rt *Router) persistRefreshedMCPToken(serverId int64, t *oauth2.Token, ver *mcpCredVersion) {
	// Persist failures must be loud: with refresh-token rotation the old token in
	// the DB is already revoked, so losing the new one breaks the server for good.
	rec, err := models.MCPServerOAuthGetByServerId(rt.Ctx, serverId)
	if err != nil {
		logger.Warningf("[MCP-OAuth] persist refreshed token failed (server=%d): load record: %v", serverId, err)
		return
	}
	if rec == nil {
		logger.Warningf("[MCP-OAuth] persist refreshed token failed (server=%d): oauth record not found", serverId)
		return
	}
	accEnc, err := rt.encryptMCPSecret(t.AccessToken)
	if err != nil {
		logger.Warningf("[MCP-OAuth] persist refreshed token failed (server=%d): encrypt access token: %v", serverId, err)
		return
	}
	rec.AccessToken = accEnc
	if t.RefreshToken != "" {
		refEnc, err := rt.encryptMCPSecret(t.RefreshToken)
		if err != nil {
			logger.Warningf("[MCP-OAuth] persist refreshed token failed (server=%d): encrypt refresh token: %v", serverId, err)
			return
		}
		rec.RefreshToken = refEnc
	}
	if t.TokenType != "" {
		rec.TokenType = t.TokenType
	}
	if !t.Expiry.IsZero() {
		rec.Expiry = t.Expiry.Unix()
	}
	if err := rec.Save(rt.Ctx); err != nil {
		logger.Warningf("[MCP-OAuth] persist refreshed token failed (server=%d): save: %v", serverId, err)
		return
	}
	// The stored ciphertext just rotated — move the pin, or a later invalidation
	// would target a version that no longer exists and silently skip.
	ver.set(accEnc)
}

// ---- endpoints ----

// mcpServerOAuthPrepare discovers the AS, (dynamically) registers the client and
// returns the browser authorization URL. The server row must already exist.
func (rt *Router) mcpServerOAuthPrepare(c *gin.Context) {
	var body struct {
		Id           int64  `json:"id"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		Scope        string `json:"scope"`
	}
	ginx.BindJSON(c, &body)

	obj := rt.mcpServerLoadForManage(c, body.Id)

	me := c.MustGet("user").(*models.User)
	hc := mcp.DefaultOAuthHTTPClient()

	endpoints, err := mcp.Discover(c.Request.Context(), obj.URL, hc)
	ginx.Dangerous(err)

	redirectURI := rt.mcpBaseURL(c) + mcpServerOAuthCallbackPath

	clientID := strings.TrimSpace(body.ClientID)
	clientSecret := strings.TrimSpace(body.ClientSecret)
	scope := strings.TrimSpace(body.Scope)

	// Re-authorization (the chat's authorize button, or "重新连接" on the form) posts
	// only the id. Reuse the client registration already stored for this server
	// before considering DCR: a server that never supported DCR — whose client was
	// configured by hand on the management page — would otherwise fail here with
	// "does not support dynamic client registration", i.e. the button could never
	// work for exactly the servers that need it most.
	//
	// Reuse is safe because invalidateMCPOAuthCredential only keeps the registration
	// when the verdict was invalid_grant (the client was never at fault). When the
	// token endpoint rejected the CLIENT it clears client_id too, so we fall through
	// to DCR below instead of handing back a client already known to be refused.
	if clientID == "" {
		if rec, rerr := models.MCPServerOAuthGetByServerId(rt.Ctx, obj.Id); rerr == nil && rec != nil && rec.ClientId != "" {
			clientID = rec.ClientId
			if sec, derr := rt.decryptMCPSecret(rec.ClientSecret); derr == nil {
				clientSecret = sec
			} else {
				logger.Warningf("[MCP] server=%d stored client_secret undecryptable, re-registering: %v", obj.Id, derr)
				clientID = ""
			}
			if clientID != "" && scope == "" {
				scope = rec.Scope
			}
		}
	}
	if clientID == "" {
		cid, csec, rerr := mcp.Register(c.Request.Context(), endpoints.RegistrationEndpoint,
			"Nightingale ("+obj.Name+")", redirectURI, endpoints.Scopes, hc)
		ginx.Dangerous(rerr)
		clientID, clientSecret = cid, csec
	}

	if scope == "" {
		scope = strings.Join(endpoints.Scopes, " ")
	}

	verifier := oauth2.GenerateVerifier()
	state := uuid.NewString()

	st := mcpOAuthState{
		ServerId: obj.Id, Endpoints: *endpoints, ClientID: clientID, ClientSecret: clientSecret,
		RedirectURI: redirectURI, Scope: scope, Verifier: verifier, ConnectedBy: me.Username,
	}
	raw, _ := json.Marshal(st)
	if err := rt.Redis.Set(c.Request.Context(), mcpOAuthStatePrefix+state, string(raw), mcpOAuthStateTTL).Err(); err != nil {
		ginx.Bomb(http.StatusInternalServerError, "failed to store oauth state: %v", err)
	}

	cfg := &mcp.OAuthConfig{Endpoints: *endpoints, ClientID: clientID, ClientSecret: clientSecret, RedirectURI: redirectURI, Scope: scope}
	ginx.NewRender(c).Data(gin.H{
		"authorize_url": mcp.BuildAuthorizeURL(cfg, state, verifier),
		"state":         state,
		"redirect_uri":  redirectURI,
	}, nil)
}

// mcpServerOAuthCallback is the vendor's redirect target (PUBLIC). It exchanges
// the code, persists encrypted tokens and returns a tiny page that postMessages
// the result to the opener (the SPA) and closes itself.
func (rt *Router) mcpServerOAuthCallback(c *gin.Context) {
	if e := c.Query("error"); e != "" {
		desc := c.Query("error_description")
		rt.mcpOAuthCallbackHTML(c, "error", 0, strings.TrimSpace(e+" "+desc))
		return
	}
	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		rt.mcpOAuthCallbackHTML(c, "error", 0, "missing code or state")
		return
	}

	key := mcpOAuthStatePrefix + state
	raw, err := rt.Redis.Get(c.Request.Context(), key).Result()
	if err != nil || raw == "" {
		rt.mcpOAuthCallbackHTML(c, "error", 0, "invalid or expired authorization state")
		return
	}
	rt.Redis.Del(c.Request.Context(), key)

	var st mcpOAuthState
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		rt.mcpOAuthCallbackHTML(c, "error", 0, "corrupt authorization state")
		return
	}

	cfg := &mcp.OAuthConfig{Endpoints: st.Endpoints, ClientID: st.ClientID, ClientSecret: st.ClientSecret, RedirectURI: st.RedirectURI, Scope: st.Scope}
	token, err := mcp.Exchange(c.Request.Context(), cfg, code, st.Verifier, mcp.DefaultOAuthHTTPClient())
	if err != nil {
		rt.mcpOAuthCallbackHTML(c, "error", st.ServerId, "token exchange failed: "+err.Error())
		return
	}

	secEnc, err := rt.encryptMCPSecret(st.ClientSecret)
	if err != nil {
		rt.mcpOAuthCallbackHTML(c, "error", st.ServerId, err.Error())
		return
	}
	accEnc, err := rt.encryptMCPSecret(token.AccessToken)
	if err != nil {
		rt.mcpOAuthCallbackHTML(c, "error", st.ServerId, err.Error())
		return
	}
	refEnc, err := rt.encryptMCPSecret(token.RefreshToken)
	if err != nil {
		rt.mcpOAuthCallbackHTML(c, "error", st.ServerId, err.Error())
		return
	}

	var expiry int64
	if !token.Expiry.IsZero() {
		expiry = token.Expiry.Unix()
	}
	rec := &models.MCPServerOAuth{
		ServerId:              st.ServerId,
		Issuer:                st.Endpoints.Issuer,
		AuthorizationEndpoint: st.Endpoints.AuthorizationEndpoint,
		TokenEndpoint:         st.Endpoints.TokenEndpoint,
		RegistrationEndpoint:  st.Endpoints.RegistrationEndpoint,
		Scope:                 st.Scope,
		Resource:              st.Endpoints.Resource,
		RedirectURI:           st.RedirectURI,
		ClientId:              st.ClientID,
		ClientSecret:          secEnc,
		AccessToken:           accEnc,
		RefreshToken:          refEnc,
		TokenType:             token.TokenType,
		Expiry:                expiry,
		ConnectedBy:           st.ConnectedBy,
	}
	if err := rec.Save(rt.Ctx); err != nil {
		rt.mcpOAuthCallbackHTML(c, "error", st.ServerId, "failed to save tokens")
		return
	}

	// Flip the server to oauth mode so the runtime path uses the tokens.
	if obj, _ := models.MCPServerGetById(rt.Ctx, st.ServerId); obj != nil && obj.AuthMode != mcp.MCPAuthOAuth {
		ref := *obj
		ref.AuthMode = mcp.MCPAuthOAuth
		ref.UpdatedBy = st.ConnectedBy
		_ = obj.Update(rt.Ctx, ref)
	}

	rt.mcpOAuthCallbackHTML(c, "success", st.ServerId, "")
}

// mcpOAuthUsable reports whether an oauth server's stored authorization is
// actually usable, returning the reason it isn't so callers can say "go
// (re)authorize". This is THE single definition of "connected", shared by the
// runtime assembly (buildMCPConfigForAgent), this status API, and the
// list_mcp_servers tool — they must not be able to disagree.
//
// Judging by a non-empty access_token (what every caller used to do) is wrong in
// both directions that matter: a token we can no longer decrypt after a key
// rotation, or one that expired with no refresh token, is a non-empty string that
// reads as "connected" while the runtime cannot use it at all — so the tools
// vanish AND the authorize button hides, which is exactly the situation the button
// exists to fix.
func (rt *Router) mcpOAuthUsable(serverId int64) error {
	cfg, _, err := rt.loadMCPOAuthConfig(serverId)
	if err != nil {
		// Record missing, token empty, or ciphertext undecryptable.
		return err
	}
	// Expired with nothing to refresh with: the token source has no way back, so
	// the next call is a guaranteed 401. Treat it as needing reauthorization now
	// rather than after the user watches the tools silently disappear.
	if !cfg.Expiry.IsZero() && cfg.Expiry.Before(time.Now()) && cfg.RefreshToken == "" {
		return fmt.Errorf("oauth token expired and no refresh token is stored; reauthorization required")
	}
	return nil
}

func (rt *Router) mcpServerOAuthStatus(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	rt.mcpServerLoadForManage(c, id)

	rec, err := models.MCPServerOAuthGetByServerId(rt.Ctx, id)
	ginx.Dangerous(err)
	if rec == nil {
		ginx.NewRender(c).Data(gin.H{"connected": false}, nil)
		return
	}
	// connected 必须与运行时「能不能真的用」同义：非空但解密不出/已过期且无 refresh
	// 的 token 一律算未连接，否则前端会显示「已连接」并藏起授权按钮，而工具其实用不了。
	usable := rt.mcpOAuthUsable(id)
	if usable != nil {
		logger.Infof("[MCP] server=%d oauth stored but unusable, reporting disconnected: %v", id, usable)
	}
	ginx.NewRender(c).Data(gin.H{
		"connected":    usable == nil,
		"expiry":       rec.Expiry,
		"scope":        rec.Scope,
		"client_id":    rec.ClientId,
		"connected_by": rec.ConnectedBy,
		"updated_at":   rec.UpdatedAt,
	}, nil)
}

func (rt *Router) mcpServerOAuthDisconnect(c *gin.Context) {
	var body struct {
		Id int64 `json:"id"`
	}
	ginx.BindJSON(c, &body)
	rt.mcpServerLoadForManage(c, body.Id)
	ginx.NewRender(c).Message(models.MCPServerOAuthDelByServerId(rt.Ctx, body.Id))
}

// mcpOAuthCallbackHTML renders the popup-closer page. The result travels to the
// opener via postMessage (JSON payload — no HTML injection surface).
func (rt *Router) mcpOAuthCallbackHTML(c *gin.Context, status string, serverId int64, message string) {
	if status == "error" {
		logger.Warningf("[MCP-OAuth] callback failed (server=%d): %s", serverId, message)
	}
	payload, _ := json.Marshal(gin.H{
		"source":   "n9e-mcp-oauth",
		"status":   status,
		"serverId": serverId,
		"message":  message,
	})
	page := `<!doctype html><html><head><meta charset="utf-8"><title>MCP OAuth</title></head>
<body style="font-family:sans-serif;padding:24px">
<script>
(function(){
  try { if (window.opener) window.opener.postMessage(` + string(payload) + `, "*"); } catch (e) {}
  setTimeout(function(){ try { window.close(); } catch (e) {} }, 100);
})();
</script>
<p>Authorization ` + status + `. You can close this window.</p>
</body></html>`
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, page)
}
