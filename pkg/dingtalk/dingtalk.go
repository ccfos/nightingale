package dingtalk

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/dingtalk/contact_1_0"
	dingtalkoauth2_1_0 "github.com/alibabacloud-go/dingtalk/oauth2_1_0"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

const defaultAuthURL = "https://login.dingtalk.com/oauth2/auth"
const SsoTypeName = "dingtalk"

type SsoClient struct {
	Enable         bool
	DingTalkConfig *Config `json:"-"`
	Ctx            context.Context
	sync.RWMutex
}

type Config struct {
	Enable       bool   `json:"enable"`
	AuthURL      string `json:"auth_url"`
	DisplayName  string `json:"display_name"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_url"`
	// Scope 授权范围,授权页面显示的授权信息以应用注册时配置的为准
	// 当前只支持两种输入:openid：授权后可获得用户userid, openid corpid：授权后可获得用户id和登录过程中用户选择的组织id，空格分隔
	Scopes []string `json:"scopes"`
	// 值为consent时，会进入授权确认页，默认值为：consent
	Prompt          string   `json:"prompt"`
	Proxy           string   `json:"proxy"`
	SkipTlsVerify   bool     `json:"skip_tls_verify"`
	CoverAttributes bool     `json:"cover_attributes"`
	DefaultRoles    []string `json:"default_roles"`
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

func wrapStateKey(key string) string {
	return "n9e_dingtalk_oauth_" + key
}

// CreateClient
/**
 * 使用 Token 初始化账号Client
 * @return Client
 * @throws Exception
 */
func (c *Config) CreateClient() (*dingtalkoauth2_1_0.Client, error) {

	config := &openapi.Config{}
	config.Protocol = tea.String("https")
	config.RegionId = tea.String("central")
	err := c.setProxy(config)
	if err != nil {
		return nil, err
	}
	_result, _err := dingtalkoauth2_1_0.NewClient(config)

	return _result, _err

}

// ContactClient 联系人
func (c *Config) ContactClient() (*contact_1_0.Client, error) {

	config := &openapi.Config{}
	// 请求协议
	config.Protocol = tea.String("https")
	config.RegionId = tea.String("central")
	err := c.setProxy(config)
	if err != nil {
		return nil, err
	}
	_result, _err := contact_1_0.NewClient(config)
	return _result, _err
}

func (c *Config) setProxy(config *openapi.Config) error {
	// 解析 代理URL协议:http\https
	proxyURL, err := url.Parse(c.Proxy)
	if err != nil {
		return err
	}
	switch proxyURL.Scheme {
	case "https":
		config.HttpsProxy = tea.String(c.Proxy)
	default:
		config.HttpProxy = tea.String(c.Proxy)

	}
	return nil
}

func New(cf Config) *SsoClient {
	var s = &SsoClient{}
	if !cf.Enable {
		return s
	}
	s.Reload(cf)
	return s
}

func (s *SsoClient) AuthCodeURL() (string, error) {
	var buf bytes.Buffer
	dingtalkOauthAuthURl := defaultAuthURL
	if s.DingTalkConfig.AuthURL != "" {
		dingtalkOauthAuthURl = s.DingTalkConfig.AuthURL
	}
	buf.WriteString(dingtalkOauthAuthURl)
	v := url.Values{
		"response_type": {"code"},
		"client_id":     {s.DingTalkConfig.ClientID},
	}
	v.Set("redirect_uri", s.DingTalkConfig.RedirectURL)

	if s.DingTalkConfig.RedirectURL == "" {
		return "", errors.New("DingTalk OAuth RedirectURL is empty")
	}

	if len(s.DingTalkConfig.Scopes) == 0 {
		v.Set("scope", "openid")
	} else {
		v.Set("scope", strings.Join(s.DingTalkConfig.Scopes, " "))
	}

	v.Set("state", uuid.New().String())

	if s.DingTalkConfig.Prompt == "" {
		s.DingTalkConfig.Prompt = "consent"
	}

	v.Set("prompt", s.DingTalkConfig.Prompt)

	if strings.Contains(dingtalkOauthAuthURl, "?") {
		buf.WriteByte('&')
	} else {
		buf.WriteByte('?')
	}
	buf.WriteString(v.Encode())

	return buf.String(), nil

}

func (s *SsoClient) GetUserToken(code string) (string, error) {
	authClient, err := s.DingTalkConfig.CreateClient()
	getUserTokenRequest := &dingtalkoauth2_1_0.GetUserTokenRequest{
		ClientId:     tea.String(s.DingTalkConfig.ClientID),
		ClientSecret: tea.String(s.DingTalkConfig.ClientSecret),
		Code:         tea.String(code),
		RefreshToken: tea.String(code),
		GrantType:    tea.String("authorization_code"),
	}
	resp, err := authClient.GetUserToken(getUserTokenRequest)
	if err != nil {
		return "", errors.New("dingtalk sso get token error: " + err.Error())
	}

	tokenBody := resp.Body
	accessToken := tea.StringValue(tokenBody.AccessToken)
	return accessToken, nil
}

func (s *SsoClient) Reload(dingTalkConfig Config) {
	s.Lock()
	defer s.Unlock()
	s.Enable = dingTalkConfig.Enable
	s.DingTalkConfig = &dingTalkConfig
}

func (s *SsoClient) GetDisplayName() string {
	s.RLock()
	defer s.RUnlock()
	if !s.Enable {
		return ""
	}

	return s.DingTalkConfig.DisplayName
}

func (s *SsoClient) Authorize(redis storage.Redis, redirect string) (string, error) {
	state := uuid.New().String()
	ctx := context.Background()

	err := redis.Set(ctx, wrapStateKey(state), redirect, time.Duration(300*time.Second)).Err()
	if err != nil {
		return "", err
	}

	s.RLock()
	defer s.RUnlock()

	return s.AuthCodeURL()

}

func (s *SsoClient) Callback(redis storage.Redis, ctx context.Context, code, state string) (*CallbackOutput, error) {

	accessToken, err := s.GetUserToken(code)
	if err != nil {
		return nil, fmt.Errorf("CreateClient error: %s", err)
	}
	// 获取用户信息
	contactClient, err := s.DingTalkConfig.ContactClient()
	if err != nil {
		return nil, fmt.Errorf("CreateClient error: %s", err)
	}

	getUserHeaders := &contact_1_0.GetUserHeaders{}
	getUserHeaders.XAcsDingtalkAccessToken = tea.String(accessToken)

	user, err := contactClient.GetUserWithOptions(tea.String("me"), getUserHeaders, &util.RuntimeOptions{})
	if err != nil {
		return nil, fmt.Errorf("CreateClient error: %s", err)
	}

	redirect := "/"
	if redis != nil {
		redirect, err = fetchRedirect(redis, ctx, state)
		if err != nil {
			logger.Errorf("get redirect err:%v code:%s state:%s", code, state, err)
		}
	}

	err = deleteRedirect(redis, ctx, state)
	if err != nil {
		logger.Errorf("delete redirect err:%v code:%s state:%s", code, state, err)
	}

	var callbackOutput CallbackOutput

	callbackOutput.Redirect = redirect
	callbackOutput.Username = tea.ToString(*user.Body.UnionId)
	callbackOutput.Nickname = tea.ToString(*user.Body.Nick)
	callbackOutput.Email = tea.ToString(*user.Body.Email)
	if tea.ToString(*user.Body.Email) != "" {
		callbackOutput.Email = tea.ToString(*user.Body.Email)
	}
	callbackOutput.Phone = tea.ToString(*user.Body.Mobile)

	return &callbackOutput, nil

}

func fetchRedirect(redis storage.Redis, ctx context.Context, state string) (string, error) {
	return redis.Get(ctx, wrapStateKey(state)).Result()
}

func deleteRedirect(redis storage.Redis, ctx context.Context, state string) error {
	return redis.Del(ctx, wrapStateKey(state)).Err()
}
