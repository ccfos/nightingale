package oidcx

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	oidc "github.com/coreos/go-oidc"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
	"golang.org/x/oauth2"
)

type SsoClient struct {
	Enable          bool
	Verifier        *oidc.IDTokenVerifier
	Config          oauth2.Config
	SsoAddr         string
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

	Ctx context.Context
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
	SkipTlsVerify   bool
	Attributes      struct {
		Nickname string
		Username string
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
		s.Enable = cf.Enable
		return nil
	}

	if cf.Attributes.Username == "" {
		cf.Attributes.Username = "sub"
	}

	s.Enable = cf.Enable
	s.SsoAddr = cf.SsoAddr
	s.CallbackAddr = cf.RedirectURL
	s.CoverAttributes = cf.CoverAttributes
	s.Attributes.Username = cf.Attributes.Username
	s.Attributes.Nickname = cf.Attributes.Nickname
	s.Attributes.Phone = cf.Attributes.Phone
	s.Attributes.Email = cf.Attributes.Email
	s.DisplayName = cf.DisplayName
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

	provider, err := oidc.NewProvider(s.Ctx, cf.SsoAddr)
	if err != nil {
		return err
	}
	oidcConfig := &oidc.Config{
		ClientID: cf.ClientId,
	}

	s.Verifier = provider.Verifier(oidcConfig)
	s.Config = oauth2.Config{
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
	if !s.Enable {
		return ""
	}

	return s.DisplayName
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

	v := func(k string) string {
		if in := data[k]; in == nil {
			return ""
		} else {
			return in.(string)
		}
	}

	return &CallbackOutput{
		AccessToken: oauth2Token.AccessToken,
		Username:    v(s.Attributes.Username),
		Nickname:    v(s.Attributes.Nickname),
		Phone:       v(s.Attributes.Phone),
		Email:       v(s.Attributes.Email),
	}, nil
}
