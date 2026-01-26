package feishu

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	larkauthen "github.com/larksuite/oapi-sdk-go/v3/service/authen/v1"
	larkcontact "github.com/larksuite/oapi-sdk-go/v3/service/contact/v3"
)

const defaultAuthURL = "https://accounts.feishu.cn/open-apis/authen/v1/authorize"
const SsoTypeName = "feishu"

type SsoClient struct {
	Enable       bool
	FeiShuConfig *Config `json:"-"`
	Ctx          context.Context
	client       *lark.Client
	sync.RWMutex
}

type Config struct {
	Enable          bool     `json:"enable"`
	AuthURL         string   `json:"auth_url"`
	DisplayName     string   `json:"display_name"`
	AppID           string   `json:"app_id"`
	AppSecret       string   `json:"app_secret"`
	RedirectURL     string   `json:"redirect_url"`
	UsernameField   string   `json:"username_field"`  // name, email, phone
	FeiShuEndpoint  string   `json:"feishu_endpoint"` // 飞书API端点，默认为 open.feishu.cn
	Proxy           string   `json:"proxy"`
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
	return "n9e_feishu_oauth_" + key
}

// createClient 创建飞书SDK客户端（v3版本）
func (c *Config) createClient() (*lark.Client, error) {
	opts := []lark.ClientOptionFunc{
		lark.WithLogLevel(larkcore.LogLevelInfo),
		lark.WithEnableTokenCache(true), // 启用token缓存
	}

	if c.FeiShuEndpoint != "" {
		lark.FeishuBaseUrl = c.FeiShuEndpoint
	}

	// 创建客户端（v3版本）
	client := lark.NewClient(
		c.AppID,
		c.AppSecret,
		opts...,
	)

	return client, nil
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
	feishuAuthURL := defaultAuthURL
	if s.FeiShuConfig.AuthURL != "" {
		feishuAuthURL = s.FeiShuConfig.AuthURL
	}
	buf.WriteString(feishuAuthURL)
	v := url.Values{
		"app_id": {s.FeiShuConfig.AppID},
		"state":  {state},
	}
	v.Set("redirect_uri", s.FeiShuConfig.RedirectURL)

	if s.FeiShuConfig.RedirectURL == "" {
		return "", errors.New("FeiShu OAuth RedirectURL is empty")
	}

	if strings.Contains(feishuAuthURL, "?") {
		buf.WriteByte('&')
	} else {
		buf.WriteByte('?')
	}
	buf.WriteString(v.Encode())

	return buf.String(), nil
}

// GetUserToken 通过授权码获取用户access token和user_id（使用SDK v3）
func (s *SsoClient) GetUserToken(code string) (string, string, error) {
	if s.client == nil {
		return "", "", errors.New("feishu client is not initialized")
	}

	ctx := context.Background()

	// 使用SDK v3的authen服务获取access token
	req := larkauthen.NewCreateAccessTokenReqBuilder().
		Body(larkauthen.NewCreateAccessTokenReqBodyBuilder().
			GrantType("authorization_code").
			Code(code).
			Build()).
		Build()

	resp, err := s.client.Authen.AccessToken.Create(ctx, req)
	if err != nil {
		return "", "", fmt.Errorf("feishu get access token error: %w", err)
	}

	// 检查响应
	if !resp.Success() {
		return "", "", fmt.Errorf("feishu api error: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil {
		return "", "", errors.New("feishu api returned empty data")
	}

	userID := ""
	if resp.Data.UserId != nil {
		userID = *resp.Data.UserId
	}
	if userID == "" {
		return "", "", errors.New("feishu api returned empty user_id")
	}

	accessToken := ""
	if resp.Data.AccessToken != nil {
		accessToken = *resp.Data.AccessToken
	}
	if accessToken == "" {
		return "", "", errors.New("feishu api returned empty access_token")
	}

	return accessToken, userID, nil
}

// GetUserInfo 通过user_id获取用户详细信息（使用SDK v3）
// 注意：SDK内部会自动管理token，所以不需要传入accessToken
func (s *SsoClient) GetUserInfo(userID string) (*larkcontact.GetUserRespData, error) {
	if s.client == nil {
		return nil, errors.New("feishu client is not initialized")
	}

	ctx := context.Background()

	// 使用SDK v3的contact服务获取用户详情
	req := larkcontact.NewGetUserReqBuilder().
		UserId(userID).
		UserIdType(larkcontact.UserIdTypeUserId).
		Build()

	resp, err := s.client.Contact.User.Get(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("feishu get user detail error: %w", err)
	}

	// 检查响应
	if !resp.Success() {
		return nil, fmt.Errorf("feishu api error: code=%d, msg=%s", resp.Code, resp.Msg)
	}

	if resp.Data == nil || resp.Data.User == nil {
		return nil, errors.New("feishu api returned empty user data")
	}

	return resp.Data, nil
}

func (s *SsoClient) Reload(feishuConfig Config) {
	s.Lock()
	defer s.Unlock()
	s.Enable = feishuConfig.Enable
	s.FeiShuConfig = &feishuConfig

	// 重新创建客户端
	if feishuConfig.Enable && feishuConfig.AppID != "" && feishuConfig.AppSecret != "" {
		client, err := feishuConfig.createClient()
		if err != nil {
			logger.Errorf("create feishu client error: %v", err)
		} else {
			s.client = client
		}
	}
}

func (s *SsoClient) GetDisplayName() string {
	s.RLock()
	defer s.RUnlock()
	if !s.Enable {
		return ""
	}

	return s.FeiShuConfig.DisplayName
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

func (s *SsoClient) Callback(redis storage.Redis, ctx context.Context, code, state string) (*CallbackOutput, error) {
	// 通过code获取access token和user_id
	accessToken, userID, err := s.GetUserToken(code)
	if err != nil {
		return nil, fmt.Errorf("feishu GetUserToken error: %s", err)
	}

	// 获取用户详细信息
	userData, err := s.GetUserInfo(userID)
	if err != nil {
		return nil, fmt.Errorf("feishu GetUserInfo error: %s", err)
	}

	// 获取redirect URL
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
	if userData == nil || userData.User == nil {
		return nil, fmt.Errorf("feishu GetUserInfo failed, user data is nil")
	}

	user := userData.User
	logger.Debugf("feishu get user info userID %s result %+v", userID, user)

	// 提取用户信息
	username := ""
	if user.UserId != nil {
		username = *user.UserId
	}
	if username == "" {
		return nil, errors.New("feishu user_id is empty")
	}

	nickname := ""
	if user.Name != nil {
		nickname = *user.Name
	}

	phone := ""
	if user.Mobile != nil {
		phone = *user.Mobile
	}

	email := ""
	if user.Email != nil {
		email = *user.Email
	}

	if email == "" {
		if user.EnterpriseEmail != nil {
			email = *user.EnterpriseEmail
		}
	}

	callbackOutput.Redirect = redirect
	callbackOutput.AccessToken = accessToken

	// 根据UsernameField配置确定username
	switch s.FeiShuConfig.UsernameField {
	case "userid":
		callbackOutput.Username = username
	case "name":
		if nickname == "" {
			return nil, errors.New("feishu user name is empty")
		}
		callbackOutput.Username = nickname
	case "phone":
		if phone == "" {
			return nil, errors.New("feishu user phone is empty")
		}
		callbackOutput.Username = phone
	default:
		if email == "" {
			return nil, errors.New("feishu user email is empty")
		}
		callbackOutput.Username = email
	}

	callbackOutput.Nickname = nickname
	callbackOutput.Email = email
	callbackOutput.Phone = phone

	return &callbackOutput, nil
}

func fetchRedirect(redis storage.Redis, ctx context.Context, state string) (string, error) {
	return redis.Get(ctx, wrapStateKey(state)).Result()
}

func deleteRedirect(redis storage.Redis, ctx context.Context, state string) error {
	return redis.Del(ctx, wrapStateKey(state)).Err()
}
