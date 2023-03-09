package oidcx

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	oidc "github.com/coreos/go-oidc"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
	"golang.org/x/oauth2"
)

type SsoClient struct {
	enable          bool
	verifier        *oidc.IDTokenVerifier
	config          oauth2.Config
	ssoAddr         string
	callbackAddr    string
	coverAttributes bool
	displayName     string
	attributes      struct {
		username string
		nickname string
		phone    string
		email    string
	}

	sync.RWMutex
}

type Config struct {
	Enable          bool
	DisplayName     string
	RedirectURL     string
	SsoAddr         string
	ClientId        string
	ClientSecret    string
	CoverAttributes bool
	Attributes      struct {
		Nickname string
		Phone    string
		Email    string
	}
	DefaultRoles []string
}

func New(cf Config) (*SsoClient, error) {
	var s = &SsoClient{}
	if !cf.Enable {
		return s, nil
	}
	err := s.Reload(cf)
	return s, err
}

func (s *SsoClient) Reload(cf Config) error {
	s.Lock()
	defer s.Unlock()
	if !cf.Enable {
		s.enable = cf.Enable
		return nil
	}

	s.enable = cf.Enable
	s.ssoAddr = cf.SsoAddr
	s.callbackAddr = cf.RedirectURL
	s.coverAttributes = cf.CoverAttributes
	s.attributes.username = "sub"
	s.attributes.nickname = cf.Attributes.Nickname
	s.attributes.phone = cf.Attributes.Phone
	s.attributes.email = cf.Attributes.Email
	s.displayName = cf.DisplayName
	provider, err := oidc.NewProvider(context.Background(), cf.SsoAddr)
	if err != nil {
		return err
	}
	oidcConfig := &oidc.Config{
		ClientID: cf.ClientId,
	}

	s.verifier = provider.Verifier(oidcConfig)
	s.config = oauth2.Config{
		ClientID:     cf.ClientId,
		ClientSecret: cf.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  cf.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "phone"},
	}
	return nil
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

	return s.config.AuthCodeURL(state), nil
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
		return nil, fmt.Errorf("ilegal user:%v", err)
	}

	ret.Redirect, err = fetchRedirect(redis, ctx, state)
	if err != nil {
		logger.Debugf("get redirect err:%v code:%s state:%s", code, state, err)
	}

	err = deleteRedirect(redis, ctx, state)
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

func (s *SsoClient) exchangeUser(code string) (*CallbackOutput, error) {
	s.RLock()
	defer s.RUnlock()

	ctx := context.Background()
	oauth2Token, err := s.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %v", err)
	}

	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		return nil, fmt.Errorf("no id_token field in oauth2 token. ")
	}

	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify ID Token: %v", err)
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
		Username:    v(s.attributes.username),
		Nickname:    v(s.attributes.nickname),
		Phone:       v(s.attributes.phone),
		Email:       v(s.attributes.email),
	}, nil
}
