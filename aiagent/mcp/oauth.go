package mcp

// OAuth 2.1 (Authorization Code + PKCE) support for the outbound MCP client.
//
// Direction: n9e is the OAuth *client* dialing external OAuth-protected MCP
// servers (Notion, Linear, Sentry, Azure ARM, Google Cloud, ...). The one-time
// interactive authorization (discovery → DCR/manual → browser consent → code
// exchange) is driven out-of-band by the router (prepare/callback endpoints)
// using the helpers here. At connect time NewOAuthHandler plugs a persisting
// token source into the SDK's StreamableClientTransport, which then injects the
// Bearer header, refreshes near expiry (x/oauth2) and retries once on 401.
//
// We do discovery/DCR/exchange ourselves (rather than the SDK's
// auth.NewAuthorizationCodeHandler) because that handler is built for a local
// CLI: it keeps tokens in memory and blocks on a fetcher waiting for the browser
// redirect — neither fits a server-mediated web flow with DB-persisted tokens.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

// OAuthEndpoints is the discovered OAuth 2.1 metadata for a remote MCP server.
type OAuthEndpoints struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	RegistrationEndpoint  string   `json:"registration_endpoint"`
	Scopes                []string `json:"scopes"`
	Resource              string   `json:"resource"`
}

// OAuthConfig is the per-server OAuth material used at connect time (populated
// from models.MCPServerOAuth, with tokens already decrypted).
type OAuthConfig struct {
	Endpoints    OAuthEndpoints
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scope        string
	AccessToken  string
	RefreshToken string
	TokenType    string
	Expiry       time.Time

	// OnRefresh, if set, is invoked whenever the token source mints a new token,
	// so the caller can persist it (encrypted) back to the DB.
	OnRefresh func(*oauth2.Token)

	// OnAuthRequired, if set, is invoked when this server's tools could not be used
	// this turn for an authorization-shaped reason — the resource server answered
	// 401/403. NON-DESTRUCTIVE by contract: callers may surface an "authorize"
	// prompt, but must NOT discard the stored credential on it.
	//
	// A bare 401/403 says nothing about the credential itself. The SDK calls
	// Authorize() on any 401/403 (mcp/streamable.go: `if resp.StatusCode == 401 ||
	// resp.StatusCode == 403`), with no refresh attempted in between, so this fires
	// just as readily for reasons unrelated to the token: insufficient_scope on one
	// tool, a gateway/WAF, an IP allowlist. Treating those as "credential dead"
	// would destroy a working refresh token — see OnCredentialInvalid.
	OnAuthRequired func(reason error)

	// OnCredentialInvalid, if set, is invoked only when the token endpoint answers
	// with an explicit RFC 6749 code saying the stored credential is dead (see
	// classifyCredentialInvalid). DESTRUCTIVE: callers may clear stored material on
	// it, and kind says how much — the grant only, or the client registration too.
	//
	// Deliberately NOT fired for transient trouble (network errors, 5xx), for a
	// resource-server 401/403, nor for a bare 401/403 from the token endpoint with
	// no OAuth error body: a false positive discards a credential that still works,
	// and mcp_server_oauth is a server-level, org-wide record — everyone would have
	// to redo the browser consent flow.
	OnCredentialInvalid func(kind CredentialInvalidKind, reason error)
}

// DefaultOAuthHTTPClient is the http.Client used for discovery/DCR/exchange.
func DefaultOAuthHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

// Discover follows the 401 WWW-Authenticate → RFC 9728 protected-resource
// metadata → RFC 8414 authorization-server metadata chain and returns the OAuth
// endpoints for an MCP server URL. hc may be nil.
func Discover(ctx context.Context, serverURL string, hc *http.Client) (*OAuthEndpoints, error) {
	if hc == nil {
		hc = DefaultOAuthHTTPClient()
	}

	// 1) Probe: an initialize POST is expected to yield 401 with a
	// WWW-Authenticate challenge pointing at the protected-resource metadata.
	prmURL := ""
	if resp, err := probeForChallenge(ctx, serverURL, hc); err == nil && resp != nil {
		if chals, perr := oauthex.ParseWWWAuthenticate(resp.Header.Values("WWW-Authenticate")); perr == nil {
			for _, ch := range chals {
				if v := ch.Params["resource_metadata"]; v != "" {
					prmURL = v
					break
				}
			}
		}
	}

	// 2) Protected-resource metadata (RFC 9728). Fall back to the well-known path
	// when the challenge did not carry a resource_metadata URL.
	var asURL, resource string
	var scopes []string
	origin := originOf(serverURL)
	for _, candidate := range dedupeNonEmpty(prmURL, strings.TrimRight(origin, "/")+"/.well-known/oauth-protected-resource") {
		prm, err := oauthex.GetProtectedResourceMetadata(ctx, candidate, serverURL, hc)
		if err == nil && prm != nil {
			resource = prm.Resource
			scopes = prm.ScopesSupported
			if len(prm.AuthorizationServers) > 0 {
				asURL = prm.AuthorizationServers[0]
			}
			break
		}
	}
	if asURL == "" {
		asURL = origin // last-resort: treat the server origin as the issuer
	}

	// 3) Authorization-server metadata (RFC 8414 / OIDC).
	as, err := discoverAuthServer(ctx, asURL, hc)
	if err != nil {
		return nil, fmt.Errorf("discover authorization server: %w", err)
	}

	ep := &OAuthEndpoints{
		Issuer:                as.Issuer,
		AuthorizationEndpoint: as.AuthorizationEndpoint,
		TokenEndpoint:         as.TokenEndpoint,
		RegistrationEndpoint:  as.RegistrationEndpoint,
		Resource:              resource,
	}
	if len(scopes) > 0 {
		ep.Scopes = scopes
	} else {
		ep.Scopes = as.ScopesSupported
	}
	if ep.AuthorizationEndpoint == "" || ep.TokenEndpoint == "" {
		return nil, fmt.Errorf("authorization server metadata missing authorize/token endpoint")
	}
	return ep, nil
}

func discoverAuthServer(ctx context.Context, asURL string, hc *http.Client) (*oauthex.AuthServerMeta, error) {
	base := strings.TrimRight(asURL, "/")
	var lastErr error
	for _, metaURL := range []string{
		base + "/.well-known/oauth-authorization-server",
		base + "/.well-known/openid-configuration",
	} {
		as, err := oauthex.GetAuthServerMeta(ctx, metaURL, asURL, hc)
		if err == nil && as != nil {
			return as, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no authorization server metadata at %s", asURL)
	}
	return nil, lastErr
}

// Register performs RFC 7591 Dynamic Client Registration and returns the issued
// client_id (+ optional client_secret). Used only when the user did not supply
// a pre-registered client.
func Register(ctx context.Context, registrationEndpoint, clientName, redirectURI string, scopes []string, hc *http.Client) (clientID, clientSecret string, err error) {
	if hc == nil {
		hc = DefaultOAuthHTTPClient()
	}
	if registrationEndpoint == "" {
		return "", "", fmt.Errorf("server does not support dynamic client registration; supply client_id/secret manually")
	}
	meta := &oauthex.ClientRegistrationMetadata{
		RedirectURIs:            []string{redirectURI},
		ClientName:              clientName,
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "none",
		Scope:                   strings.Join(scopes, " "),
	}
	resp, err := oauthex.RegisterClient(ctx, registrationEndpoint, meta, hc)
	if err != nil {
		return "", "", err
	}
	return resp.ClientID, resp.ClientSecret, nil
}

func (cfg *OAuthConfig) oauth2Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  cfg.Endpoints.AuthorizationEndpoint,
			TokenURL: cfg.Endpoints.TokenEndpoint,
		},
		RedirectURL: cfg.RedirectURI,
		Scopes:      splitScope(cfg.Scope),
	}
}

// BuildAuthorizeURL returns the browser authorization URL for the code+PKCE flow.
func BuildAuthorizeURL(cfg *OAuthConfig, state, verifier string) string {
	opts := []oauth2.AuthCodeOption{oauth2.S256ChallengeOption(verifier)}
	if cfg.Endpoints.Resource != "" {
		opts = append(opts, oauth2.SetAuthURLParam("resource", cfg.Endpoints.Resource))
	}
	return cfg.oauth2Config().AuthCodeURL(state, opts...)
}

// Exchange swaps an authorization code (+PKCE verifier) for tokens.
func Exchange(ctx context.Context, cfg *OAuthConfig, code, verifier string, hc *http.Client) (*oauth2.Token, error) {
	if hc != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	}
	opts := []oauth2.AuthCodeOption{oauth2.VerifierOption(verifier)}
	if cfg.Endpoints.Resource != "" {
		opts = append(opts, oauth2.SetAuthURLParam("resource", cfg.Endpoints.Resource))
	}
	return cfg.oauth2Config().Exchange(ctx, code, opts...)
}

// NewOAuthHandler builds an auth.OAuthHandler for the SDK transport. The
// underlying x/oauth2 token source auto-refreshes near expiry; refreshed tokens
// are written back via cfg.OnRefresh.
func NewOAuthHandler(cfg *OAuthConfig, hc *http.Client) auth.OAuthHandler {
	return &oauthHandler{cfg: cfg, hc: hc}
}

type oauthHandler struct {
	cfg *OAuthConfig
	hc  *http.Client

	mu  sync.Mutex
	src oauth2.TokenSource
}

// TokenSource lazily builds the refreshing source on first use and binds it to
// the ctx the SDK hands us here — a session-scoped, cancel-detached context
// (streamable.go's connCtx). We deliberately bind to that, not the short-lived
// connect ctx: the connect ctx is cancelled right after tool discovery, so a
// source bound to it would fail every later refresh with "context canceled".
func (h *oauthHandler) TokenSource(ctx context.Context) (oauth2.TokenSource, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.src == nil {
		if h.hc != nil {
			ctx = context.WithValue(ctx, oauth2.HTTPClient, h.hc)
		}
		tok := &oauth2.Token{
			AccessToken:  h.cfg.AccessToken,
			RefreshToken: h.cfg.RefreshToken,
			TokenType:    h.cfg.TokenType,
			Expiry:       h.cfg.Expiry,
		}
		base := h.cfg.oauth2Config().TokenSource(ctx, tok)
		h.src = &persistingTokenSource{src: base, onRefresh: h.cfg.OnRefresh, onCredentialInvalid: h.cfg.OnCredentialInvalid, last: tok}
	}
	return h.src, nil
}

// Authorize is called by the SDK whenever a request comes back 401/403. Interactive
// re-consent is out-of-band (prepare/callback), so all we can do is report that the
// server needs reconnecting and let the caller offer an authorize button, instead
// of this server's tools just silently disappearing.
//
// It must NOT conclude the credential is dead. The SDK's contract is "called when
// an HTTP request results in an error that may be addressed by the authorization
// flow (currently 401 and 403)" — mcp/streamable.go dispatches on the status code
// alone, with no refresh attempted first (the token source only refreshes near
// expiry, and returns a cached token otherwise). So a perfectly valid token reaches
// us here whenever the resource server 403s for an unrelated reason:
// insufficient_scope on one tool, a gateway/WAF, an IP allowlist. Destroying the
// org-wide refresh token on that would be unrecoverable — only the token endpoint
// answering with an explicit OAuth error code proves the credential itself is dead
// (see persistingTokenSource.Token / classifyCredentialInvalid).
func (h *oauthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	err := fmt.Errorf("mcp oauth: authorization required, please reconnect the server")
	if h.cfg.OnAuthRequired != nil {
		h.cfg.OnAuthRequired(err)
	}
	return err
}

// persistingTokenSource wraps a refreshing oauth2.TokenSource and calls onRefresh
// whenever the token changes, so callers can persist the rotated token. This is the
// ONLY path that may declare a credential dead: a refresh rejected as invalid_grant
// means the refresh token itself is gone, which no local pre-check can see.
type persistingTokenSource struct {
	src                 oauth2.TokenSource
	onRefresh           func(*oauth2.Token)
	onCredentialInvalid func(CredentialInvalidKind, error)

	mu   sync.Mutex
	last *oauth2.Token
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		if p.onCredentialInvalid != nil {
			if kind, dead := classifyCredentialInvalid(err); dead {
				p.onCredentialInvalid(kind, err)
			}
		}
		return nil, err
	}
	p.mu.Lock()
	changed := p.last == nil || tok.AccessToken != p.last.AccessToken || tok.RefreshToken != p.last.RefreshToken
	p.last = tok
	onRefresh := p.onRefresh
	p.mu.Unlock()
	if changed && onRefresh != nil {
		onRefresh(tok)
	}
	return tok, nil
}

// CredentialInvalidKind says WHICH part of the stored credential the token
// endpoint rejected, because that decides how much has to be thrown away.
type CredentialInvalidKind int

const (
	// CredentialInvalidGrant: the refresh token is dead (invalid_grant). The client
	// registration is still good, so re-consent can reuse it.
	CredentialInvalidGrant CredentialInvalidKind = iota
	// CredentialInvalidClient: the CLIENT itself was rejected (invalid_client /
	// unauthorized_client). Keeping the registration would make every retry — and
	// every press of the authorize button — fail identically, so it has to go too.
	CredentialInvalidClient
)

// classifyCredentialInvalid reports whether a token-endpoint failure definitively
// means the stored credential is dead, and which part of it.
//
// ONLY an explicitly parsed RFC 6749 error code counts. There is deliberately no
// fallback on the HTTP status: a WAF, IP allowlist or a hiccuping gateway sitting
// in front of the token endpoint can answer a bare 401/403 with no OAuth error
// body, and acting on that would irreversibly wipe an org-wide refresh token that
// was never the problem. Same reason a network error or a 5xx never counts.
// x/oauth2 surfaces token-endpoint errors as *oauth2.RetrieveError.
func classifyCredentialInvalid(err error) (CredentialInvalidKind, bool) {
	var re *oauth2.RetrieveError
	if !errors.As(err, &re) {
		return 0, false
	}
	switch re.ErrorCode {
	case "invalid_grant":
		return CredentialInvalidGrant, true
	case "invalid_client", "unauthorized_client":
		return CredentialInvalidClient, true
	}
	return 0, false
}

// probeForChallenge sends an initialize request and returns the response so the
// caller can read its WWW-Authenticate header. A non-2xx status is expected and
// not an error here.
func probeForChallenge(ctx context.Context, serverURL string, hc *http.Client) (*http.Response, error) {
	const body = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"nightingale","version":"1.0.0"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	return resp, nil
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

func splitScope(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return strings.Fields(s)
}

func dedupeNonEmpty(vals ...string) []string {
	seen := make(map[string]struct{}, len(vals))
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
