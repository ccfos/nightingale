package oauth2x

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/didi/nightingale/v5/src/storage"
	"github.com/toolkits/pkg/logger"

	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	"golang.org/x/oauth2"
)

type ssoClient struct {
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

var (
	cli ssoClient
)

func Init(cf Config) {
	if !cf.Enable {
		return
	}

	cli.ssoAddr = cf.SsoAddr
	cli.userInfoAddr = cf.UserInfoAddr
	cli.TranTokenMethod = cf.TranTokenMethod
	cli.callbackAddr = cf.RedirectURL
	cli.displayName = cf.DisplayName
	cli.coverAttributes = cf.CoverAttributes
	cli.attributes.username = cf.Attributes.Username
	cli.attributes.nickname = cf.Attributes.Nickname
	cli.attributes.phone = cf.Attributes.Phone
	cli.attributes.email = cf.Attributes.Email
	cli.userinfoIsArray = cf.UserinfoIsArray
	cli.userinfoPrefix = cf.UserinfoPrefix

	cli.config = oauth2.Config{
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

func GetDisplayName() string {
	return cli.displayName
}

func wrapStateKey(key string) string {
	return "n9e_oauth_" + key
}

// Authorize return the sso authorize location with state
func Authorize(redirect string) (string, error) {
	state := uuid.New().String()
	ctx := context.Background()

	err := storage.Redis.Set(ctx, wrapStateKey(state), redirect, time.Duration(300*time.Second)).Err()
	if err != nil {
		return "", err
	}

	return cli.config.AuthCodeURL(state), nil
}

func fetchRedirect(ctx context.Context, state string) (string, error) {
	return storage.Redis.Get(ctx, wrapStateKey(state)).Result()
}

func deleteRedirect(ctx context.Context, state string) error {
	return storage.Redis.Del(ctx, wrapStateKey(state)).Err()
}

// Callback 用 code 兑换 accessToken 以及 用户信息
func Callback(ctx context.Context, code, state string) (*CallbackOutput, error) {
	ret, err := exchangeUser(code)
	if err != nil {
		return nil, fmt.Errorf("ilegal user:%v", err)
	}
	ret.Redirect, err = fetchRedirect(ctx, state)
	if err != nil {
		logger.Errorf("get redirect err:%v code:%s state:%s", code, state, err)
	}

	err = deleteRedirect(ctx, state)
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

func exchangeUser(code string) (*CallbackOutput, error) {
	ctx := context.Background()
	oauth2Token, err := cli.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %s", err)
	}

	userInfo, err := getUserInfo(cli.userInfoAddr, oauth2Token.AccessToken, cli.TranTokenMethod)
	if err != nil {
		logger.Errorf("failed to get user info: %s", err)
		return nil, fmt.Errorf("failed to get user info: %s", err)
	}

	return &CallbackOutput{
		AccessToken: oauth2Token.AccessToken,
		Username:    getUserinfoField(userInfo, cli.userinfoIsArray, cli.userinfoPrefix, cli.attributes.username),
		Nickname:    getUserinfoField(userInfo, cli.userinfoIsArray, cli.userinfoPrefix, cli.attributes.nickname),
		Phone:       getUserinfoField(userInfo, cli.userinfoIsArray, cli.userinfoPrefix, cli.attributes.phone),
		Email:       getUserinfoField(userInfo, cli.userinfoIsArray, cli.userinfoPrefix, cli.attributes.email),
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
