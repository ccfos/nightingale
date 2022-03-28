package oidcc

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/didi/nightingale/v5/src/storage"

	oidc "github.com/coreos/go-oidc"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
	"golang.org/x/oauth2"
)

type ssoClient struct {
	verifier        *oidc.IDTokenVerifier
	config          oauth2.Config
	ssoAddr         string
	callbackAddr    string
	coverAttributes bool
	attributes      struct {
		username string
		nickname string
		phone    string
		email    string
	}
}

type Config struct {
	Enable          bool   `yaml:"enable"`
	RedirectURL     string `yaml:"redirectURL"`
	SsoAddr         string `yaml:"ssoAddr"`
	ClientId        string `yaml:"clientId"`
	ClientSecret    string `yaml:"clientSecret"`
	CoverAttributes bool   `yaml:"coverAttributes"`
	Attributes      struct {
		Nickname string `yaml:"nickname"`
		Phone    string `yaml:"phone"`
		Email    string `yaml:"email"`
	} `yaml:"attributes"`
	DefaultRoles []string `yaml:"defaultRoles"`
}

var (
	cli ssoClient
)

func Init(cf Config) {
	if !cf.Enable {
		return
	}

	cli.ssoAddr = cf.SsoAddr
	cli.callbackAddr = cf.RedirectURL
	cli.coverAttributes = cf.CoverAttributes
	cli.attributes.username = "sub"
	cli.attributes.nickname = cf.Attributes.Nickname
	cli.attributes.phone = cf.Attributes.Phone
	cli.attributes.email = cf.Attributes.Email
	provider, err := oidc.NewProvider(context.Background(), cf.SsoAddr)
	if err != nil {
		log.Fatal(err)
	}
	oidcConfig := &oidc.Config{
		ClientID: cf.ClientId,
	}

	cli.verifier = provider.Verifier(oidcConfig)
	cli.config = oauth2.Config{
		ClientID:     cf.ClientId,
		ClientSecret: cf.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cf.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "phone"},
	}
}

func wrapStateKey(key string) string {
	return "n9e_sso_" + key
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

// Callback 用 code 兑换 accessToken 以及 用户信息,
func Callback(ctx context.Context, code, state string) (*CallbackOutput, error) {
	ret, err := exchangeUser(code)
	if err != nil {
		return nil, fmt.Errorf("ilegal user:%v", err)
	}

	ret.Redirect, err = fetchRedirect(ctx, state)
	if err != nil {
		logger.Debugf("get redirect err:%v code:%s state:%s", code, state, err)
	}

	err = deleteRedirect(ctx, state)
	if err != nil {
		logger.Debugf("delete redirect err:%v code:%s state:%s", code, state, err)
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
		return nil, fmt.Errorf("Failed to exchange token: %s", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("No id_token field in oauth2 token.")
	}
	idToken, err := cli.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("Failed to verify ID Token: %s", err)
	}

	data := map[string]interface{}{}
	if err := idToken.Claims(&data); err != nil {
		return nil, err
	}

	v := func(k string) string {
		if in := data[k]; in == nil {
			return ""
		} else {
			return in.(string)
		}
	}

	return &CallbackOutput{
		AccessToken: oauth2Token.AccessToken,
		Username:    v(cli.attributes.username),
		Nickname:    v(cli.attributes.nickname),
		Phone:       v(cli.attributes.phone),
		Email:       v(cli.attributes.email),
	}, nil
}
