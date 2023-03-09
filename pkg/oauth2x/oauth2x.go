package oauth2x

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/logger"

	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/oauth2"
)

type SsoClient struct {
	enable          bool
	config          oauth2.Config
	ssoAddr         string
	userInfoAddr    string
	TranTokenMethod string
	callbackAddr    string
	displayName     string
	coverAttributes bool
	attributes      struct {
		username string
		nickname string
		phone    string
		email    string
	}
	userinfoIsArray bool
	userinfoPrefix  string

	sync.RWMutex
}

type Config struct {
	Enable          bool
	DisplayName     string
	RedirectURL     string
	SsoAddr         string
	TokenAddr       string
	UserInfoAddr    string
	TranTokenMethod string
	ClientId        string
	ClientSecret    string
	CoverAttributes bool
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
		s.enable = cf.Enable
		return
	}

	s.enable = cf.Enable
	s.ssoAddr = cf.SsoAddr
	s.userInfoAddr = cf.UserInfoAddr
	s.TranTokenMethod = cf.TranTokenMethod
	s.callbackAddr = cf.RedirectURL
	s.displayName = cf.DisplayName
	s.coverAttributes = cf.CoverAttributes
	s.attributes.username = cf.Attributes.Username
	s.attributes.nickname = cf.Attributes.Nickname
	s.attributes.phone = cf.Attributes.Phone
	s.attributes.email = cf.Attributes.Email
	s.userinfoIsArray = cf.UserinfoIsArray
	s.userinfoPrefix = cf.UserinfoPrefix

	s.config = oauth2.Config{
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
	if !s.enable {
		return ""
	}

	return s.displayName
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
	return s.config.AuthCodeURL(state), nil
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
		return nil, fmt.Errorf("ilegal user:%v", err)
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
	Username    string `json:"username"`
	Nickname    string `json:"nickname"`
	Phone       string `yaml:"phone"`
	Email       string `yaml:"email"`
}

func (s *SsoClient) exchangeUser(code string) (*CallbackOutput, error) {
	s.RLock()
	defer s.RUnlock()

	ctx := context.Background()
	oauth2Token, err := s.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %s", err)
	}

	userInfo, err := getUserInfo(s.userInfoAddr, oauth2Token.AccessToken, s.TranTokenMethod)
	if err != nil {
		logger.Errorf("failed to get user info: %s", err)
		return nil, fmt.Errorf("failed to get user info: %s", err)
	}

	return &CallbackOutput{
		AccessToken: oauth2Token.AccessToken,
		Username:    getUserinfoField(userInfo, s.userinfoIsArray, s.userinfoPrefix, s.attributes.username),
		Nickname:    getUserinfoField(userInfo, s.userinfoIsArray, s.userinfoPrefix, s.attributes.nickname),
		Phone:       getUserinfoField(userInfo, s.userinfoIsArray, s.userinfoPrefix, s.attributes.phone),
		Email:       getUserinfoField(userInfo, s.userinfoIsArray, s.userinfoPrefix, s.attributes.email),
	}, nil
}

func getUserInfo(userInfoAddr, accessToken string, TranTokenMethod string) ([]byte, error) {
	var req *http.Request
	if TranTokenMethod == "formdata" {
		body := bytes.NewBuffer([]byte("access_token=" + accessToken))
		r, err := http.NewRequest("POST", userInfoAddr, body)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req = r
	} else if TranTokenMethod == "querystring" {
		r, err := http.NewRequest("GET", userInfoAddr+"?access_token="+accessToken, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Authorization", "Bearer "+accessToken)
		req = r
	} else {
		r, err := http.NewRequest("GET", userInfoAddr, nil)
		if err != nil {
			return nil, err
		}
		r.Header.Add("Authorization", "Bearer "+accessToken)
		req = r
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, nil
	}
	return body, err
}

func getUserinfoField(input []byte, isArray bool, prefix, field string) string {
	if prefix == "" {
		if isArray {
			return jsoniter.Get(input, 0).Get(field).ToString()
		} else {
			return jsoniter.Get(input, field).ToString()
		}
	} else {
		if isArray {
			return jsoniter.Get(input, prefix, 0).Get(field).ToString()
		} else {
			return jsoniter.Get(input, prefix).Get(field).ToString()
		}
	}
}
