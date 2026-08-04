package oidcx

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	oidc "github.com/coreos/go-oidc"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
	"golang.org/x/oauth2"
)

// idpTimeout 限制与 IdP 的单次 HTTP 交互耗时（discovery、取 JWKS、换 token），避免拖住定期 reload
const idpTimeout = 10 * time.Second

type SsoClient struct {
	Enable          bool
	Verifier        *oidc.IDTokenVerifier
	Config          oauth2.Config
	SsoAddr         string
	SsoLogoutAddr   string
	CallbackAddr    string
	CoverAttributes bool
	DisplayName     string
	Attributes      struct {
		Username string
		Nickname string
		Phone    string
		Email    string
	}
	DefaultRoles []string
	DefaultTeams []int64

	Ctx      context.Context
	Provider *oidc.Provider
	// reloadMu 串行化整个 Reload（含 discovery），细节见 Reload 注释
	reloadMu sync.Mutex
	sync.RWMutex
}

type Config struct {
	Enable          bool
	DisplayName     string
	RedirectURL     string
	SsoAddr         string
	SsoLogoutAddr   string
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
	DefaultRoles []string
	DefaultTeams []int64
	Scopes       []string
}

func New(cf Config) (*SsoClient, error) {
	var s = &SsoClient{}
	if !cf.Enable {
		return s, nil
	}
	err := s.Reload(cf)
	return s, err
}

// Reload 用新配置重建 OIDC 客户端。与 IdP 的 discovery 交互放在加写锁之前完成：一是
// 避免网络 I/O 期间阻塞正在进行的登录请求，二是失败时不会把半初始化的状态写进 s ——
// Enable 为真而 Provider 为空会让登录页出现一个点了跳错地址的入口。IdP 不可达时把
// Enable 置回 false，由调用方按周期重试，恢复后无需重启进程。
//
// discovery 在写锁之外做，所以要另用一把锁把「discovery + 发布」串起来：定期 reload 和
// 管理员保存配置会并发调进来，否则慢的旧配置可能在快的新配置（甚至「关闭 OIDC」）之后
// 才发布，把新配置覆盖回去。
func (s *SsoClient) Reload(cf Config) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()

	if !cf.Enable {
		s.Lock()
		defer s.Unlock()
		s.Enable = cf.Enable
		return nil
	}

	if cf.Attributes.Username == "" {
		cf.Attributes.Username = "sub"
	}

	// 默认 http.Client 没有超时，IdP 地址是黑洞时会把调用方（定期 reload 协程）永久挂住，
	// 连带其他 SSO 配置也热更新不了。超时只能落在 client 上，不能用 context.WithTimeout：
	// oidc.NewProvider 会把这里的 ctx 一直存进 Provider 的远程 JWKS，后续验签取密钥还要用它，
	// 用完就 cancel 的 ctx 会让登录回调和 token 校验直接报 context canceled
	client := &http.Client{Timeout: idpTimeout}
	if cf.SkipTlsVerify {
		// 从 DefaultTransport 克隆而不是新建一个零值 Transport：零值没有 IdleConnTimeout，
		// IdP 长期返回错误时，每个 reload 周期一次的重试会把空闲连接一直攒下去
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		client.Transport = transport
	}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)

	provider, err := oidc.NewProvider(ctx, cf.SsoAddr)
	if err != nil {
		// 这次创建的连接不会再有人用，重试很频繁，别留着占 fd
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}

		s.Lock()
		s.Enable = false
		s.Unlock()
		return err
	}
	oidcConfig := &oidc.Config{
		ClientID: cf.ClientId,
	}

	s.Lock()
	defer s.Unlock()
	s.Enable = cf.Enable
	s.SsoAddr = cf.SsoAddr
	s.SsoLogoutAddr = cf.SsoLogoutAddr
	s.CallbackAddr = cf.RedirectURL
	s.CoverAttributes = cf.CoverAttributes
	s.Attributes.Username = cf.Attributes.Username
	s.Attributes.Nickname = cf.Attributes.Nickname
	s.Attributes.Phone = cf.Attributes.Phone
	s.Attributes.Email = cf.Attributes.Email
	s.DisplayName = cf.DisplayName
	s.DefaultRoles = cf.DefaultRoles
	s.DefaultTeams = cf.DefaultTeams
	s.Ctx = ctx
	s.Verifier = provider.Verifier(oidcConfig)
	s.Provider = provider
	s.Config = oauth2.Config{
		ClientID:     cf.ClientId,
		ClientSecret: cf.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cf.RedirectURL,
		Scopes:       cf.Scopes,
	}

	if len(s.Config.Scopes) == 0 {
		s.Config.Scopes = []string{oidc.ScopeOpenID, "profile", "email", "phone"}
	}

	return nil
}

// Ready 表示 OIDC 已开启且与 IdP 完成了 discovery，可以真正承接登录。调用方据此决定
// 是否展示 OIDC 登录入口。空接收者也安全：SSO 客户端的初始化可能因 IdP 不可达而失败。
func (s *SsoClient) Ready() bool {
	if s == nil {
		return false
	}

	s.RLock()
	defer s.RUnlock()
	return s.ready()
}

// ready 要求调用方已持有锁
func (s *SsoClient) ready() bool {
	return s.Enable && s.Provider != nil
}

func (s *SsoClient) GetDisplayName() string {
	if s == nil {
		return ""
	}

	s.RLock()
	defer s.RUnlock()
	if !s.ready() {
		return ""
	}

	return s.DisplayName
}

func (s *SsoClient) GetSsoLogoutAddr(idToken string) string {
	if s == nil {
		return ""
	}

	s.RLock()
	defer s.RUnlock()
	if !s.ready() {
		return ""
	}

	return s.replaceIdTokenTemplate(s.SsoLogoutAddr, idToken)
}

// replaceIdTokenTemplate 替换登出 URL 中的 {{$__id_token__}} 模板变量
func (s *SsoClient) replaceIdTokenTemplate(logoutAddr, idToken string) string {
	if idToken == "" {
		return logoutAddr
	}
	return strings.ReplaceAll(logoutAddr, "{{$__id_token__}}", idToken)
}

func wrapStateKey(key string) string {
	return "n9e_oidc_" + key
}

// Authorize return the sso authorize location with state
func (s *SsoClient) Authorize(redis storage.Redis, redirect string) (string, error) {
	s.RLock()
	defer s.RUnlock()

	state := uuid.New().String()
	ctx := context.Background()

	err := redis.Set(ctx, wrapStateKey(state), redirect, time.Duration(300*time.Second)).Err()
	if err != nil {
		return "", err
	}

	return s.Config.AuthCodeURL(state), nil
}

func fetchRedirect(redis storage.Redis, ctx context.Context, state string) (string, error) {
	return redis.Get(ctx, wrapStateKey(state)).Result()
}

func deleteRedirect(redis storage.Redis, ctx context.Context, state string) error {
	return redis.Del(ctx, wrapStateKey(state)).Err()
}

// Callback 用 code 兑换 accessToken 以及 用户信息,
func (s *SsoClient) Callback(redis storage.Redis, ctx context.Context, code, state string) (*CallbackOutput, error) {
	ret, err := s.exchangeUser(code)
	if err != nil {
		return nil, fmt.Errorf("sso_exchange_user fail. code:%s, error:%v", code, err)
	}

	ret.Redirect, err = fetchRedirect(redis, ctx, state)
	if err != nil {
		logger.Errorf("get redirect err:%v code:%s state:%s", code, state, err)
	}

	err = deleteRedirect(redis, ctx, state)
	if err != nil {
		logger.Errorf("delete redirect err:%v code:%s state:%s", code, state, err)
	}
	return ret, nil
}

type CallbackOutput struct {
	Redirect    string `json:"redirect"`
	Msg         string `json:"msg"`
	AccessToken string `json:"accessToken"`
	IdToken     string `json:"idToken"`
	Username    string `json:"username"`
	Nickname    string `json:"nickname"`
	Phone       string `yaml:"phone"`
	Email       string `yaml:"email"`
}

func (s *SsoClient) exchangeUser(code string) (*CallbackOutput, error) {
	s.RLock()
	defer s.RUnlock()

	oauth2Token, err := s.Config.Exchange(s.Ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %v", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		rerr := fmt.Errorf("sso_exchange_user: no id_token field in oauth2 token %v", oauth2Token)
		logger.Error(rerr)
		return nil, rerr
	}

	idToken, err := s.Verifier.Verify(s.Ctx, rawIDToken)
	if err != nil {
		rerr := fmt.Errorf("sso_exchange_user: failed to verify id_token: %s, error:%v", rawIDToken, err)
		logger.Error(rerr)
		return nil, rerr
	}

	logger.Infof("sso_exchange_user: verify id_token success. token:%s", rawIDToken)

	data := map[string]interface{}{}
	if err := idToken.Claims(&data); err != nil {
		rerr := fmt.Errorf("sso_exchange_user: failed to parse id_token: %s, error:%+v", rawIDToken, err)
		logger.Error(rerr)
		return nil, rerr
	}

	for k, v := range data {
		logger.Debugf("sso_exchange_user: oidc info key:%s value:%v", k, v)
	}

	output := &CallbackOutput{
		AccessToken: oauth2Token.AccessToken,
		IdToken:     rawIDToken,
		Username:    extractClaim(data, s.Attributes.Username),
		Nickname:    extractClaim(data, s.Attributes.Nickname),
		Phone:       extractClaim(data, s.Attributes.Phone),
		Email:       extractClaim(data, s.Attributes.Email),
	}

	userInfo, err := s.Provider.UserInfo(s.Ctx, oauth2.StaticTokenSource(oauth2Token))
	if err != nil {
		logger.Errorf("sso_exchange_user: failed to get userinfo: %v", err)
		return output, nil
	}

	if userInfo == nil {
		logger.Errorf("sso_exchange_user: userinfo is nil")
		return output, nil
	}

	logger.Debugf("sso_exchange_user: userinfo subject:%s email:%s profile:%s", userInfo.Subject, userInfo.Email, userInfo.Profile)
	if output.Email == "" {
		output.Email = userInfo.Email
	}

	data = map[string]interface{}{}
	userInfo.Claims(&data)
	logger.Debugf("sso_exchange_user: userinfo claims:%+v", data)

	if output.Nickname == "" {
		output.Nickname = extractClaim(data, s.Attributes.Nickname)
	}

	if output.Phone == "" {
		output.Phone = extractClaim(data, s.Attributes.Phone)
	}

	return output, nil
}

// VerifyAccessToken validates an external IdP-issued OAuth access token in
// Resource Server mode and maps it to the configured user attributes. It reuses
// the OIDC login provider's JWKS to check the token signature, the issuer and
// the expiry, and requires the token's `aud` to contain audience (this n9e
// service's resource identifier) so a token minted for another application
// cannot be replayed here. The returned CallbackOutput carries only the
// user-identity fields; the token fields are left empty.
func (s *SsoClient) VerifyAccessToken(ctx context.Context, rawToken, audience string) (*CallbackOutput, error) {
	s.RLock()
	defer s.RUnlock()

	if !s.Enable || s.Provider == nil {
		return nil, fmt.Errorf("oidc is not enabled")
	}
	if audience == "" {
		return nil, fmt.Errorf("rs audience is not configured")
	}

	// A verifier bound to the resource audience — separate from s.Verifier,
	// whose audience is the OIDC login client id. The provider's remote JWKS is
	// shared, so this allocation does no network I/O.
	verifier := s.Provider.Verifier(&oidc.Config{ClientID: audience})
	token, err := verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, err
	}

	data := map[string]interface{}{}
	if err := token.Claims(&data); err != nil {
		return nil, fmt.Errorf("failed to parse access token claims: %v", err)
	}

	username := extractClaim(data, s.Attributes.Username)
	if username == "" {
		return nil, fmt.Errorf("username claim %q is empty in access token", s.Attributes.Username)
	}

	return &CallbackOutput{
		Username: username,
		Nickname: extractClaim(data, s.Attributes.Nickname),
		Phone:    extractClaim(data, s.Attributes.Phone),
		Email:    extractClaim(data, s.Attributes.Email),
	}, nil
}

func extractClaim(data map[string]interface{}, key string) string {
	if value, ok := data[key]; ok {
		if strValue, ok := value.(string); ok {
			return strValue
		}
	}
	return ""
}
