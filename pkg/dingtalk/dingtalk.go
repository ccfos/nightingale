package dingtalk

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	dingtalkUserClient "github.com/ccfos/nightingale/v6/pkg/dingtalk/user"
	"github.com/ccfos/nightingale/v6/storage"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	"github.com/alibabacloud-go/dingtalk/contact_1_0"
	dingtalkoauth2 "github.com/alibabacloud-go/dingtalk/oauth2_1_0"
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
	Enable      bool   `json:"enable"`
	AuthURL     string `json:"auth_url"`
	DisplayName string `json:"display_name"`
	// CorpId 用于指定用户需要选择的组织, scope包含corpid时该参数存在意义
	CorpId          string   `json:"corpId"`
	ClientID        string   `json:"client_id"`
	ClientSecret    string   `json:"client_secret"`
	RedirectURL     string   `json:"redirect_url"`
	UsernameField   string   `json:"username_field"`
	Endpoint        string   `json:"endpoint"`
	DingTalkAPI     string   `json:"dingtalk_api"`
	UseMemberInfo   bool     `json:"use_member_info"` // 是否开启查询用户详情，需要qyapi_get_member权限
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
func (c *Config) CreateClient() (*dingtalkoauth2.Client, error) {

	config := &openapi.Config{}
	config.Protocol = tea.String("https")
	config.RegionId = tea.String("central")
	err := c.setProxy(config)
	if err != nil {
		return nil, err
	}
	err = c.setEndpoint(config, c.Endpoint)
	if err != nil {
		return nil, err
	}
	dingTalkOAuthClient, err := dingtalkoauth2.NewClient(config)

	return dingTalkOAuthClient, err

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
	err = c.setEndpoint(config, c.Endpoint)
	if err != nil {
		return nil, err
	}
	dingTalkContactClient, err := contact_1_0.NewClient(config)
	return dingTalkContactClient, err
}

// UserClient 用户详情
func (c *Config) UserClient() (*dingtalkUserClient.Client, error) {

	config := &openapi.Config{}
	// 请求协议
	config.Protocol = tea.String("https")
	config.RegionId = tea.String("central")
	err := c.setProxy(config)
	if err != nil {
		return nil, err
	}
	err = c.setEndpoint(config, c.DingTalkAPI)
	if err != nil {
		return nil, err
	}
	dingTalkUserClient, err := dingtalkUserClient.NewClient(config)
	return dingTalkUserClient, err
}

func (c *Config) setEndpoint(config *openapi.Config, endpoint string) error {

	if endpoint == "" {
		return nil
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	switch endpointURL.Scheme {
	case "http":
		config.SetProtocol("http")
		config.Endpoint = tea.String(strings.Replace(endpoint, "http://", "", 1))
	case "https":
		config.SetProtocol("https")
		config.Endpoint = tea.String(strings.Replace(endpoint, "https://", "", 1))
	default:
		config.SetProtocol("https")
		config.Endpoint = tea.String(endpoint)
	}
	return nil
}

func (c *Config) setProxy(config *openapi.Config) error {
	// 解析 代理URL协议:http\https
	proxyURL, err := url.Parse(c.Proxy)
	if err != nil {
		return err
	}
	switch proxyURL.Scheme {
	case "https":
		config.SetHttpsProxy(c.Proxy)
	default:
		config.SetHttpProxy(c.Proxy)
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

func (s *SsoClient) AuthCodeURL(state string) (string, error) {
	var buf bytes.Buffer
	dingTalkOauthAuthURl := defaultAuthURL
	if s.DingTalkConfig.AuthURL != "" {
		dingTalkOauthAuthURl = s.DingTalkConfig.AuthURL
	}
	buf.WriteString(dingTalkOauthAuthURl)
	v := url.Values{
		"response_type": {"code"},
		"client_id":     {s.DingTalkConfig.ClientID},
	}
	v.Set("redirect_uri", s.DingTalkConfig.RedirectURL)

	if s.DingTalkConfig.RedirectURL == "" {
		return "", errors.New("DingTalk OAuth RedirectURL is empty")
	}

	if s.DingTalkConfig.CorpId != "" {
		// Scope 授权范围, 当前只支持两种输入,
		// openid：授权后可获得用户userid, openid
		// corpid：授权后可获得用户id和登录过程中用户选择的组织id，空格分隔。注意url编码
		v.Set("scope", "openid corpid")
		// corpId: 必须设置scope值为openid corpid
		v.Set("corpId", s.DingTalkConfig.CorpId)
	} else {
		v.Set("scope", "openid")
	}
	v.Set("prompt", "consent")
	v.Set("state", state)

	if strings.Contains(dingTalkOauthAuthURl, "?") {
		buf.WriteByte('&')
	} else {
		buf.WriteByte('?')
	}
	buf.WriteString(v.Encode())

	return buf.String(), nil

}

func (s *SsoClient) GetUserToken(code string) (string, error) {
	authClient, err := s.DingTalkConfig.CreateClient()
	getUserTokenRequest := &dingtalkoauth2.GetUserTokenRequest{
		ClientId:     tea.String(s.DingTalkConfig.ClientID),
		ClientSecret: tea.String(s.DingTalkConfig.ClientSecret),
		Code:         tea.String(code),
		RefreshToken: tea.String(code),
		GrantType:    tea.String("authorization_code"),
	}
	resp, err := authClient.GetUserToken(getUserTokenRequest)
	if err != nil {
		return "", errors.New("dingTalk sso get token error: " + err.Error())
	}

	tokenBody := resp.Body
	accessToken := tea.StringValue(tokenBody.AccessToken)
	return accessToken, nil
}

func (s *SsoClient) GetAccessToken() (string, error) {
	authClient, err := s.DingTalkConfig.CreateClient()
	getUserTokenRequest := &dingtalkoauth2.GetAccessTokenRequest{
		AppKey:    tea.String(s.DingTalkConfig.ClientID),
		AppSecret: tea.String(s.DingTalkConfig.ClientSecret),
	}
	resp, err := authClient.GetAccessToken(getUserTokenRequest)
	if err != nil {
		return "", errors.New("dingTalk sso get token error: " + err.Error())
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

	return s.AuthCodeURL(state)

}

func (s *SsoClient) GetUserInfo(accessToken string, unionid string) (*dingtalkUserClient.GetUserResult, error) {
	userClient, err := s.DingTalkConfig.UserClient()
	if err != nil {
		return nil, fmt.Errorf("CreateClient error: %s", err)
	}
	query := &dingtalkUserClient.GetUserQuery{AccessToken: accessToken}
	unionReq := &dingtalkUserClient.GetUnionIdRequest{
		UnionID: unionid,
	}
	uid, err := userClient.GetByUnionId(unionReq, query)
	if err != nil {
		return nil, err
	}
	if uid.Body == nil {
		return nil, errors.Errorf("dingTalk get userid fail status code : %d", tea.Int32Value(uid.StatusCode))
	}
	if uid.Body.Result == nil {
		return nil, errors.Errorf("dingTalk get userid body: %s", uid.Body.String())
	}
	req := &dingtalkUserClient.GetUserRequest{
		UserID: tea.StringValue(uid.Body.Result.UserId),
	}

	userInfo, err := userClient.GetUser(req, query)

	if userInfo.Body == nil {
		return nil, errors.Errorf("dingTalk get userinfo status code: %d", tea.Int32Value(userInfo.StatusCode))
	}

	logger.Debugf("dingTalk get userinfo RequestID %s UserID %s ", userInfo.Body.RequestID, req.UserID)

	return userInfo.Body.Result, nil
}

func (s *SsoClient) Callback(redis storage.Redis, ctx context.Context, code, state string) (*CallbackOutput, error) {

	userAccessToken, err := s.GetUserToken(code)
	if err != nil {
		return nil, fmt.Errorf("dingTalk GetUserToken error: %s", err)
	}
	// 获取用户信息
	contactClient, err := s.DingTalkConfig.ContactClient()
	if err != nil {
		return nil, fmt.Errorf("dingTalk New ContactClient error: %s", err)
	}

	getUserHeaders := &contact_1_0.GetUserHeaders{}
	getUserHeaders.XAcsDingtalkAccessToken = tea.String(userAccessToken)

	me, err := contactClient.GetUserWithOptions(tea.String("me"), getUserHeaders, &util.RuntimeOptions{})
	if err != nil {
		return nil, fmt.Errorf("dingTalk GetUser me error: %s", err)
	}

	redirect := ""
	if redis != nil {
		redirect, err = fetchRedirect(redis, ctx, state)
		if err != nil {
			logger.Errorf("get redirect err:%v code:%s state:%s", err, code, state)
		}
	}
	if redirect == "" {
		redirect = "/"
	}

	err = deleteRedirect(redis, ctx, state)
	if err != nil {
		logger.Errorf("delete redirect err:%v code:%s state:%s", err, code, state)
	}

	var callbackOutput CallbackOutput
	if me.Body == nil {
		return nil, fmt.Errorf("dingTalk GetUser failed, status code:%d", me.StatusCode)
	}
	logger.Debugf("dingTalk get contact %+v", me)
	username := tea.StringValue(me.Body.Nick)
	nickname := tea.StringValue(me.Body.Nick)
	phone := tea.StringValue(me.Body.Mobile)
	email := tea.StringValue(me.Body.Email)

	if s.DingTalkConfig.UseMemberInfo {
		unionID := tea.StringValue(me.Body.UnionId)
		accessToken, err := dingTalkAccessTokenCacheGet(redis, ctx)
		if err != nil {
			logger.Warningf("dingTalk get accessToken cache fail %s", err.Error())
		}
		if accessToken == "" {
			accessToken, err = s.GetAccessToken()
			if err != nil {
				return nil, err
			}
			err = dingTalkAccessTokenCacheSet(redis, ctx, accessToken)
			if err != nil {
				logger.Warningf("dingTalk set accessToken cache fail %s", err.Error())
			}
		}

		user, err := s.GetUserInfo(accessToken, unionID)
		if err != nil {
			return nil, err
		}
		if user == nil {
			return nil, fmt.Errorf("dingTalk GetUserInfo unionid %s username %s is nil", unionID, username)
		}
		logger.Debugf("dingTalk get user info unionID %s accessToken %s result %+v", unionID, accessToken, user)
		username = tea.StringValue(user.Name)
		nickname = tea.StringValue(user.Name)
		phone = tea.StringValue(user.Mobile)
		email = tea.StringValue(user.Email)
	}

	callbackOutput.Redirect = redirect

	switch s.DingTalkConfig.UsernameField {
	case "name":
		if username == "" {
			return nil, errors.New("dingTalk user name is empty")
		}
		callbackOutput.Username = username
	case "email":
		if email == "" {
			return nil, errors.New("dingTalk user email is empty")
		}
		callbackOutput.Username = email
	default:
		if phone == "" {
			return nil, errors.New("dingTalk user mobile is empty")
		}
		callbackOutput.Username = phone
	}
	callbackOutput.Nickname = nickname
	callbackOutput.Email = email
	callbackOutput.Phone = phone

	return &callbackOutput, nil

}

func dingTalkAccessTokenCacheSet(redis storage.Redis, ctx context.Context, accessToken string) error {
	// accessToken的有效期为7200秒（2小时），有效期内重复获取会返回相同结果并自动续期，过期后获取会返回新的accessToken
	// 不能频繁调用gettoken接口，否则会受到频率拦截。
	// 设置accessToken缓存90分钟，比官方少半小时
	return redis.Set(ctx, wrapStateKey("dingtalk_access_token"), accessToken, time.Duration(5400*time.Second)).Err()
}

func dingTalkAccessTokenCacheGet(redis storage.Redis, ctx context.Context) (string, error) {
	return redis.Get(ctx, wrapStateKey("dingtalk_access_token")).Result()
}

func fetchRedirect(redis storage.Redis, ctx context.Context, state string) (string, error) {
	return redis.Get(ctx, wrapStateKey(state)).Result()
}

func deleteRedirect(redis storage.Redis, ctx context.Context, state string) error {
	return redis.Del(ctx, wrapStateKey(state)).Err()
}
