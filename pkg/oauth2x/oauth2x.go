package oauth2x

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/logger"

	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/oauth2"
)

type SsoClient struct {
	Enable          bool
	Config          oauth2.Config
	SsoAddr         string
	SsoLogoutAddr   string
	UserInfoAddr    string
	TranTokenMethod string
	CallbackAddr    string
	DisplayName     string
	CoverAttributes bool
	Attributes      struct {
		Username string
		Nickname string
		Phone    string
		Email    string
	}
	UserinfoIsArray bool
	UserinfoPrefix  string
	DefaultRoles    []string
	// DefaultTeams lists the user group ids a user created by OAuth2 login is
	// added to on first login.
	DefaultTeams []int64
	// RSVerifyMethod selects how this provider verifies a2a/mcp Resource Server
	// access tokens: "" (default) or "userinfo" validates the token via the
	// UserInfo endpoint and does NOT bind audience (any valid token from this
	// IdP is accepted); "introspect" uses RFC 7662 introspection and binds
	// audience. See VerifyAccessToken for the full security tradeoff.
	RSVerifyMethod string
	// IntrospectAddr is the IdP's RFC 7662 token introspection endpoint, used
	// when RSVerifyMethod is "introspect". Empty disables introspection-based
	// Resource Server verification.
	IntrospectAddr string
	// IntrospectCacheSeconds bounds how long a positive RS verification result
	// is reused (introspection further caps it by the token's own exp); 0
	// disables caching.
	IntrospectCacheSeconds int

	Ctx context.Context
	sync.RWMutex

	rsTokenCacheMu sync.Mutex
	rsTokenCache   map[string]rsTokenCacheEntry
}

type rsTokenCacheEntry struct {
	out    *CallbackOutput
	expire int64 // unix seconds
}

// rsTokenCacheSweepThreshold is the map size at which a Put first sweeps expired
// entries, bounding memory when many distinct tokens are seen over time.
const rsTokenCacheSweepThreshold = 1024

type Config struct {
	Enable          bool
	DisplayName     string
	RedirectURL     string
	SsoAddr         string
	SsoLogoutAddr   string
	TokenAddr       string
	UserInfoAddr    string
	TranTokenMethod string
	ClientId        string
	ClientSecret    string
	CoverAttributes bool
	SkipTlsVerify   bool
	Attributes      struct {
		Username string
		Nickname string
		Phone    string
		Email    string
	}
	DefaultRoles           []string
	DefaultTeams           []int64
	UserinfoIsArray        bool
	UserinfoPrefix         string
	Scopes                 []string
	RSVerifyMethod         string
	IntrospectAddr         string
	IntrospectCacheSeconds int
	// TokenAuthStyle selects how client credentials are sent to TokenAddr:
	// "params" (client_id/client_secret in the form body), "header" (HTTP
	// Basic), or empty to auto-detect. Auto-detect tries Basic first and
	// retries in the body on any failure, which reuses the authorization code;
	// IdPs that only accept body credentials and burn the code on the first
	// attempt need "params".
	TokenAuthStyle string
	// TokenResponsePrefix unwraps a non-standard token response such as
	// {"code":200,"data":{"access_token":...}} by lifting the object at this
	// path (same lookup rules as UserinfoPrefix) to the top level before it is
	// parsed. Empty means the token endpoint already follows RFC 6749 §5.1.
	TokenResponsePrefix string
}

func New(cf Config) *SsoClient {
	var s = &SsoClient{}
	if !cf.Enable {
		return s
	}
	s.Reload(cf)
	return s
}

func (s *SsoClient) Reload(cf Config) {
	s.Lock()
	defer s.Unlock()
	if !cf.Enable {
		s.Enable = cf.Enable
		return
	}

	s.Enable = cf.Enable
	s.SsoAddr = cf.SsoAddr
	s.SsoLogoutAddr = cf.SsoLogoutAddr
	s.UserInfoAddr = cf.UserInfoAddr
	s.TranTokenMethod = cf.TranTokenMethod
	s.CallbackAddr = cf.RedirectURL
	s.DisplayName = cf.DisplayName
	s.CoverAttributes = cf.CoverAttributes
	s.Attributes.Username = cf.Attributes.Username
	s.Attributes.Nickname = cf.Attributes.Nickname
	s.Attributes.Phone = cf.Attributes.Phone
	s.Attributes.Email = cf.Attributes.Email
	s.UserinfoIsArray = cf.UserinfoIsArray
	s.UserinfoPrefix = cf.UserinfoPrefix
	s.DefaultRoles = cf.DefaultRoles
	s.DefaultTeams = cf.DefaultTeams
	s.RSVerifyMethod = cf.RSVerifyMethod
	s.IntrospectAddr = cf.IntrospectAddr
	s.IntrospectCacheSeconds = cf.IntrospectCacheSeconds

	s.Ctx = context.Background()

	var transport http.RoundTripper
	if cf.SkipTlsVerify {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	if cf.TokenResponsePrefix != "" {
		transport = newTokenResponseTransport(transport, cf.TokenAddr, cf.TokenResponsePrefix)
	}
	if transport != nil {
		// Create an HTTP client that uses our custom transport
		client := &http.Client{Transport: transport}
		s.Ctx = context.WithValue(s.Ctx, oauth2.HTTPClient, client)
	}

	s.Config = oauth2.Config{
		ClientID:     cf.ClientId,
		ClientSecret: cf.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:   cf.SsoAddr,
			TokenURL:  cf.TokenAddr,
			AuthStyle: parseAuthStyle(cf.TokenAuthStyle),
		},
		RedirectURL: cf.RedirectURL,
		Scopes:      cf.Scopes,
	}
}

func (s *SsoClient) GetDisplayName() string {
	s.RLock()
	defer s.RUnlock()
	if !s.Enable {
		return ""
	}

	return s.DisplayName
}

// GetDefaultTeams returns the user group ids newly created users are added to.
// Reload rewrites the field every few seconds, so read it under the lock.
func (s *SsoClient) GetDefaultTeams() []int64 {
	s.RLock()
	defer s.RUnlock()
	if !s.Enable {
		return nil
	}

	return s.DefaultTeams
}

func (s *SsoClient) GetSsoLogoutAddr() string {
	s.RLock()
	defer s.RUnlock()
	if !s.Enable {
		return ""
	}

	return s.SsoLogoutAddr
}

func wrapStateKey(key string) string {
	return "n9e_oauth_" + key
}

// Authorize return the sso authorize location with state
func (s *SsoClient) Authorize(redis storage.Redis, redirect string) (string, error) {
	state := uuid.New().String()
	ctx := context.Background()

	err := redis.Set(ctx, wrapStateKey(state), redirect, time.Duration(300*time.Second)).Err()
	if err != nil {
		return "", err
	}

	s.RLock()
	defer s.RUnlock()
	return s.Config.AuthCodeURL(state), nil
}

func fetchRedirect(redis storage.Redis, ctx context.Context, state string) (string, error) {
	return redis.Get(ctx, wrapStateKey(state)).Result()
}

func deleteRedirect(redis storage.Redis, ctx context.Context, state string) error {
	return redis.Del(ctx, wrapStateKey(state)).Err()
}

// Callback 用 code 兑换 accessToken 以及 用户信息
func (s *SsoClient) Callback(redis storage.Redis, ctx context.Context, code, state string) (*CallbackOutput, error) {
	ret, err := s.exchangeUser(code)
	if err != nil {
		return nil, fmt.Errorf("illegal user:%v", err)
	}
	ret.Redirect, err = fetchRedirect(redis, ctx, state)
	if err != nil {
		logger.Errorf("get redirect err:%v code:%s state:%s", err, code, state)
	}

	err = deleteRedirect(redis, ctx, state)
	if err != nil {
		logger.Errorf("delete redirect err:%v code:%s state:%s", err, code, state)
	}
	return ret, nil
}

type CallbackOutput struct {
	Redirect    string `json:"redirect"`
	Msg         string `json:"msg"`
	AccessToken string `json:"accessToken"`
	Username    string `json:"Username"`
	Nickname    string `json:"Nickname"`
	Phone       string `yaml:"Phone"`
	Email       string `yaml:"Email"`
}

func (s *SsoClient) exchangeUser(code string) (*CallbackOutput, error) {
	s.RLock()
	defer s.RUnlock()

	oauth2Token, err := s.Config.Exchange(s.Ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %s", err)
	}
	userInfo, err := s.getUserInfo(s.Config.ClientID, s.UserInfoAddr, oauth2Token.AccessToken, s.TranTokenMethod)
	if err != nil {
		logger.Errorf("failed to get user info: %s", err)
		return nil, fmt.Errorf("failed to get user info: %s", err)
	}
	logger.Debugf("get userInfo: %s", string(userInfo))
	// Some IdPs report errors as HTTP 200 with a business code and no user
	// payload; an empty username must fail here, otherwise the callback would
	// create (and later log in as) a user with a blank username.
	ret, err := s.userAttrs().parse(userInfo)
	if err != nil {
		logger.Errorf("failed to parse user info: %s, body: %s", err, truncate(userInfo, 512))
		return nil, fmt.Errorf("failed to parse user info: %s", err)
	}
	ret.AccessToken = oauth2Token.AccessToken
	return ret, nil
}

// parseAuthStyle maps the TokenAuthStyle config onto oauth2.AuthStyle.
func parseAuthStyle(style string) oauth2.AuthStyle {
	switch strings.ToLower(strings.TrimSpace(style)) {
	case "params":
		return oauth2.AuthStyleInParams
	case "header":
		return oauth2.AuthStyleInHeader
	default:
		return oauth2.AuthStyleAutoDetect
	}
}

// buildUserInfoRequest builds the UserInfo request for the configured token
// transport method. Shared by interactive login (getUserInfo) and the Resource
// Server UserInfo fallback so both stay in lockstep on transport quirks.
func buildUserInfoRequest(ctx context.Context, clientID, userInfoAddr, accessToken, tranTokenMethod string) (*http.Request, error) {
	switch tranTokenMethod {
	case "formdata":
		body := bytes.NewBuffer([]byte("access_token=" + accessToken + "&client_id=" + clientID))
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, userInfoAddr, body)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		return r, nil
	case "querystring":
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoAddr+"?access_token="+accessToken+"&client_id="+clientID, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Authorization", "Bearer "+accessToken)
		return r, nil
	default:
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoAddr, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Authorization", "Bearer "+accessToken)
		r.Header.Add("client_id", clientID)
		return r, nil
	}
}

func (s *SsoClient) getUserInfo(ClientId, UserInfoAddr, accessToken string, TranTokenMethod string) ([]byte, error) {
	req, err := buildUserInfoRequest(context.Background(), ClientId, UserInfoAddr, accessToken, TranTokenMethod)
	if err != nil {
		return nil, err
	}

	client := http.DefaultClient
	c := s.Ctx.Value(oauth2.HTTPClient)
	if c != nil {
		client = c.(*http.Client)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("userinfo endpoint returned http %d: %s", resp.StatusCode, truncate(body, 512))
	}
	return body, nil
}

// truncate shortens an IdP response body for logs and error messages.
func truncate(body []byte, n int) string {
	if len(body) <= n {
		return string(body)
	}
	return string(body[:n]) + "..."
}

// getUserinfoField extracts a field from JSON userinfo response.
// prefix is resolved by prefixNode.
func getUserinfoField(input []byte, isArray bool, prefix, field string) string {
	if prefix == "" {
		if isArray {
			return jsoniter.Get(input, 0).Get(field).ToString()
		}
		return jsoniter.Get(input, field).ToString()
	}

	node := prefixNode(input, prefix)
	if isArray {
		node = node.Get(0)
	}
	return node.Get(field).ToString()
}

// prefixNode resolves prefix in a JSON document. prefix is first tried as a
// literal JSON key for backward compatibility (e.g. a key named "data.user").
// Only the literal prefix node itself is checked — not any field below it — so
// a present literal key never falls through to the nested path. If the literal
// key is absent and prefix contains dots, it falls back to dot-separated
// multi-level path navigation. An unresolved prefix yields an invalid node.
func prefixNode(input []byte, prefix string) jsoniter.Any {
	node := jsoniter.Get(input, prefix)
	if node.ValueType() != jsoniter.InvalidValue || !strings.Contains(prefix, ".") {
		return node
	}

	var path []interface{}
	for _, seg := range strings.Split(prefix, ".") {
		path = append(path, seg)
	}
	return jsoniter.Get(input, path...)
}

// rsAttrs is the snapshot of attribute-mapping config used to project an IdP
// JSON response (introspection or userinfo) onto a CallbackOutput.
type rsAttrs struct {
	username string
	nickname string
	phone    string
	email    string
	isArray  bool
	prefix   string
}

// userAttrs snapshots the attribute mapping; callers must hold s's lock.
func (s *SsoClient) userAttrs() rsAttrs {
	return rsAttrs{
		username: s.Attributes.Username,
		nickname: s.Attributes.Nickname,
		phone:    s.Attributes.Phone,
		email:    s.Attributes.Email,
		isArray:  s.UserinfoIsArray,
		prefix:   s.UserinfoPrefix,
	}
}

func (a rsAttrs) parse(body []byte) (*CallbackOutput, error) {
	username := getUserinfoField(body, a.isArray, a.prefix, a.username)
	if strings.TrimSpace(username) == "" {
		return nil, fmt.Errorf("username claim %q is empty in IdP response", a.username)
	}
	return &CallbackOutput{
		Username: username,
		Nickname: getUserinfoField(body, a.isArray, a.prefix, a.nickname),
		Phone:    getUserinfoField(body, a.isArray, a.prefix, a.phone),
		Email:    getUserinfoField(body, a.isArray, a.prefix, a.email),
	}, nil
}

// VerifyAccessToken validates an OAuth2 access token and resolves it to user
// attributes. It mirrors oidcx.VerifyAccessToken so the Resource Server
// middleware can treat both providers uniformly.
//
// RSVerifyMethod selects the strategy:
//   - "" (default) or "userinfo": the token is validated by a successful
//     UserInfo lookup. This is the smoother default since most OAuth2 servers
//     expose a UserInfo endpoint. SECURITY: UserInfo carries no `aud`, so the
//     audience is NOT enforced — any valid token from the same IdP is accepted.
//   - "introspect": RFC 7662 introspection. The token's `aud` MUST contain
//     audience — binding the token to this service; a response without a
//     matching audience is rejected. Use it when stronger assurance is required.
//
// Positive results are cached by token hash for IntrospectCacheSeconds
// (introspection further caps it by the token's own exp).
func (s *SsoClient) VerifyAccessToken(ctx context.Context, rawToken, audience string) (*CallbackOutput, error) {
	s.RLock()
	enable := s.Enable
	method := s.RSVerifyMethod
	introspectAddr := s.IntrospectAddr
	userInfoAddr := s.UserInfoAddr
	tranTokenMethod := s.TranTokenMethod
	clientID := s.Config.ClientID
	clientSecret := s.Config.ClientSecret
	attrs := s.userAttrs()
	cacheSeconds := s.IntrospectCacheSeconds
	httpClient := http.DefaultClient
	if c := s.Ctx.Value(oauth2.HTTPClient); c != nil {
		httpClient = c.(*http.Client)
	}
	s.RUnlock()

	if !enable {
		return nil, fmt.Errorf("oauth2 is not enabled")
	}

	if out := s.rsTokenCacheGet(rawToken); out != nil {
		return out, nil
	}

	var (
		out *CallbackOutput
		ttl int64
		err error
	)
	switch method {
	case "introspect":
		if introspectAddr == "" {
			return nil, fmt.Errorf("oauth2 introspection endpoint (IntrospectAddr) is not configured")
		}
		if audience == "" {
			return nil, fmt.Errorf("rs audience is not configured")
		}
		out, ttl, err = s.verifyByIntrospection(ctx, httpClient, introspectAddr, clientID, clientSecret, audience, rawToken, attrs, cacheSeconds)
	default: // "" (default) or "userinfo": UserInfo-based validation
		if userInfoAddr == "" {
			return nil, fmt.Errorf("oauth2 UserInfoAddr is not configured")
		}
		out, err = s.verifyByUserInfo(ctx, httpClient, userInfoAddr, clientID, tranTokenMethod, rawToken, attrs)
		ttl = int64(cacheSeconds)
	}
	if err != nil {
		return nil, err
	}

	s.rsTokenCachePut(rawToken, out, ttl)
	return out, nil
}

func (s *SsoClient) verifyByIntrospection(ctx context.Context, client *http.Client, introspectAddr, clientID, clientSecret, audience, rawToken string, attrs rsAttrs, cacheSeconds int) (*CallbackOutput, int64, error) {
	form := url.Values{}
	form.Set("token", rawToken)
	form.Set("token_type_hint", "access_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, introspectAddr, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	// RFC 7662 §2.1: the protected resource authenticates to the introspection
	// endpoint with its own client credentials.
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("introspection endpoint returned http %d", resp.StatusCode)
	}

	// active is the only REQUIRED field in an introspection response (RFC 7662
	// §2.2); a false/absent value means the token is invalid or expired.
	if !jsoniter.Get(body, "active").ToBool() {
		return nil, 0, fmt.Errorf("access token is not active")
	}
	if !audienceContains(body, audience) {
		return nil, 0, fmt.Errorf("access token audience does not contain %q", audience)
	}

	out, err := attrs.parse(body)
	if err != nil {
		return nil, 0, err
	}

	ttl := int64(0)
	if cacheSeconds > 0 {
		ttl = int64(cacheSeconds)
		if exp := jsoniter.Get(body, "exp").ToInt64(); exp > 0 {
			if remain := exp - time.Now().Unix(); remain < ttl {
				ttl = remain
			}
		}
	}
	return out, ttl, nil
}

func (s *SsoClient) verifyByUserInfo(ctx context.Context, client *http.Client, userInfoAddr, clientID, tranTokenMethod, rawToken string, attrs rsAttrs) (*CallbackOutput, error) {
	req, err := buildUserInfoRequest(ctx, clientID, userInfoAddr, rawToken, tranTokenMethod)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// A bad/expired token makes the IdP reject the UserInfo lookup; treat any
	// non-2xx as an invalid token. (No `aud` is available here — see the
	// security note on VerifyAccessToken.)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo endpoint returned http %d", resp.StatusCode)
	}
	return attrs.parse(body)
}

// audienceContains reports whether the introspection response's `aud` claim
// contains audience. RFC 7662 allows `aud` to be either a string or an array of
// strings; a missing claim returns false so verification fails closed.
func audienceContains(body []byte, audience string) bool {
	aud := jsoniter.Get(body, "aud")
	switch aud.ValueType() {
	case jsoniter.StringValue:
		return aud.ToString() == audience
	case jsoniter.ArrayValue:
		for i := 0; i < aud.Size(); i++ {
			if aud.Get(i).ToString() == audience {
				return true
			}
		}
	}
	return false
}

func tokenCacheKey(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func (s *SsoClient) rsTokenCacheGet(rawToken string) *CallbackOutput {
	key := tokenCacheKey(rawToken)
	now := time.Now().Unix()

	s.rsTokenCacheMu.Lock()
	defer s.rsTokenCacheMu.Unlock()
	e, ok := s.rsTokenCache[key]
	if !ok {
		return nil
	}
	if e.expire <= now {
		delete(s.rsTokenCache, key)
		return nil
	}
	return e.out
}

func (s *SsoClient) rsTokenCachePut(rawToken string, out *CallbackOutput, ttl int64) {
	if ttl <= 0 {
		return
	}
	key := tokenCacheKey(rawToken)
	now := time.Now().Unix()

	s.rsTokenCacheMu.Lock()
	defer s.rsTokenCacheMu.Unlock()
	if s.rsTokenCache == nil {
		s.rsTokenCache = make(map[string]rsTokenCacheEntry)
	}
	// Get only evicts a key when that exact token is looked up again; rotated
	// tokens never are, so their expired entries would pile up. Sweep them once
	// the map grows past a threshold to keep it bounded without a background
	// goroutine.
	if len(s.rsTokenCache) >= rsTokenCacheSweepThreshold {
		for k, e := range s.rsTokenCache {
			if e.expire <= now {
				delete(s.rsTokenCache, k)
			}
		}
	}
	s.rsTokenCache[key] = rsTokenCacheEntry{out: out, expire: now + ttl}
}
