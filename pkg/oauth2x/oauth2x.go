package oauth2x

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
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

	Ctx context.Context
	sync.RWMutex
}

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
	DefaultRoles    []string
	UserinfoIsArray bool
	UserinfoPrefix  string
	Scopes          []string
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

	s.Ctx = context.Background()

	if cf.SkipTlsVerify {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		// Create an HTTP client that uses our custom transport
		client := &http.Client{Transport: transport}
		s.Ctx = context.WithValue(s.Ctx, oauth2.HTTPClient, client)
	}

	s.Config = oauth2.Config{
		ClientID:     cf.ClientId,
		ClientSecret: cf.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  cf.SsoAddr,
			TokenURL: cf.TokenAddr,
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
	return &CallbackOutput{
		AccessToken: oauth2Token.AccessToken,
		Username:    getUserinfoField(userInfo, s.UserinfoIsArray, s.UserinfoPrefix, s.Attributes.Username),
		Nickname:    getUserinfoField(userInfo, s.UserinfoIsArray, s.UserinfoPrefix, s.Attributes.Nickname),
		Phone:       getUserinfoField(userInfo, s.UserinfoIsArray, s.UserinfoPrefix, s.Attributes.Phone),
		Email:       getUserinfoField(userInfo, s.UserinfoIsArray, s.UserinfoPrefix, s.Attributes.Email),
	}, nil
}

func (s *SsoClient) getUserInfo(ClientId, UserInfoAddr, accessToken string, TranTokenMethod string) ([]byte, error) {
	var req *http.Request
	if TranTokenMethod == "formdata" {
		body := bytes.NewBuffer([]byte("access_token=" + accessToken + "&client_id=" + ClientId))
		r, err := http.NewRequest("POST", UserInfoAddr, body)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req = r
	} else if TranTokenMethod == "querystring" {
		r, err := http.NewRequest("GET", UserInfoAddr+"?access_token="+accessToken+"&client_id="+ClientId, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Authorization", "Bearer "+accessToken)
		req = r
	} else {
		r, err := http.NewRequest("GET", UserInfoAddr, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Authorization", "Bearer "+accessToken)
		r.Header.Add("client_id", ClientId)
		req = r
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
	return body, err
}

// getUserinfoField extracts a field from JSON userinfo response.
// prefix is first tried as a literal JSON key for backward compatibility (e.g. a key named "data.user").
// If no match is found and the prefix contains dots, it falls back to dot-separated multi-level path navigation.
func getUserinfoField(input []byte, isArray bool, prefix, field string) string {
	if prefix == "" {
		if isArray {
			return jsoniter.Get(input, 0).Get(field).ToString()
		}
		return jsoniter.Get(input, field).ToString()
	}

	// Try prefix as a literal key first (backward compatible).
	// Only check whether the literal prefix node itself exists — not the final field.
	// This avoids falling through to the nested path when the literal key exists
	// but the field is missing, the array is empty, or the value type doesn't match.
	prefixNode := jsoniter.Get(input, prefix)
	if prefixNode.ValueType() != jsoniter.InvalidValue {
		if isArray {
			return prefixNode.Get(0).Get(field).ToString()
		}
		return prefixNode.Get(field).ToString()
	}

	// Fall back to dot-separated path only when literal prefix key is absent and prefix contains dots.
	if !strings.Contains(prefix, ".") {
		return ""
	}

	var path []interface{}
	for _, seg := range strings.Split(prefix, ".") {
		path = append(path, seg)
	}
	if isArray {
		path = append(path, 0)
	}
	return jsoniter.Get(input, path...).Get(field).ToString()
}
