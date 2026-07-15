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

	// OnAuthInvalid, if set, is invoked when the stored credential is DEFINITIVELY
	// rejected: the token endpoint answers invalid_grant/invalid_client, or the
	// resource server answers a 401/403 that refreshing could not resolve. Callers
	// use it to mark the authorization dead and ask the user to reconnect — a local
	// pre-check can't see either case (the ciphertext decrypts, the stored expiry
	// looks fine), so without this the server's tools silently vanish mid-turn.
	//
	// It is deliberately NOT fired for transient trouble (network errors, 5xx):
	// a false positive would discard a credential that still works.
	OnAuthInvalid func(reason error)
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
		h.src = &persistingTokenSource{src: base, onRefresh: h.cfg.OnRefresh, onAuthInvalid: h.cfg.OnAuthInvalid, last: tok}
	}
	return h.src, nil
}

// Authorize is called by the SDK on a 401/403 that the refreshing token source
// could not resolve. Interactive re-consent is out-of-band (prepare/callback),
// so we can only report that the server needs to be reconnected — and tell the
// caller the credential is dead, so it can offer the user an authorize button
// instead of just dropping this server's tools.
func (h *oauthHandler) Authorize(ctx context.Context, req *http.Request, resp *http.Response) error {
	err := fmt.Errorf("mcp oauth: authorization required, please reconnect the server")
	// The SDK reaches us only after refreshing already failed to resolve the
	// 401/403 — that's a definitive rejection, not a transient blip.
	if h.cfg.OnAuthInvalid != nil {
		h.cfg.OnAuthInvalid(err)
	}
	return err
}

// persistingTokenSource wraps a refreshing oauth2.TokenSource and calls onRefresh
// whenever the token changes, so callers can persist the rotated token. A refresh
// rejected as invalid_grant (revoked/expired refresh token) is reported via
// onAuthInvalid — that failure is otherwise invisible to any local pre-check.
type persistingTokenSource struct {
	src           oauth2.TokenSource
	onRefresh     func(*oauth2.Token)
	onAuthInvalid func(error)

	mu   sync.Mutex
	last *oauth2.Token
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		if p.onAuthInvalid != nil && isCredentialInvalid(err) {
			p.onAuthInvalid(err)
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

// isCredentialInvalid reports whether a token-endpoint failure definitively means
// the stored credential is dead, as opposed to a transient blip a retry could
// resolve. Only the RFC 6749 error codes that say "this grant/client is no longer
// valid", plus an outright 401/403 from the token endpoint, qualify — a network
// error or a 5xx must NOT, since acting on it would discard a working credential.
// x/oauth2 surfaces token-endpoint errors as *oauth2.RetrieveError.
func isCredentialInvalid(err error) bool {
	var re *oauth2.RetrieveError
	if !errors.As(err, &re) {
		return false
	}
	switch re.ErrorCode {
	case "invalid_grant", "invalid_client", "unauthorized_client":
		return true
	}
	if re.Response != nil {
		return re.Response.StatusCode == http.StatusUnauthorized || re.Response.StatusCode == http.StatusForbidden
	}
	return false
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
