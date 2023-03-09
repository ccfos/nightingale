package cas

import (
	"bytes"
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	"github.com/google/uuid"
	"github.com/toolkits/pkg/cas"
	"github.com/toolkits/pkg/logger"
)

type Config struct {
	Enable          bool
	SsoAddr         string
	RedirectURL     string
	DisplayName     string
	CoverAttributes bool
	Attributes      struct {
		Nickname string
		Phone    string
		Email    string
	}
	DefaultRoles []string
}

type SsoClient struct {
	enable       bool
	config       Config
	ssoAddr      string
	callbackAddr string
	displayName  string
	attributes   struct {
		nickname string
		phone    string
		email    string
	}

	sync.RWMutex
}

func New(cf Config) *SsoClient {
	var cli SsoClient
	if !cf.Enable {
		return &cli
	}

	cli.config = cf
	cli.ssoAddr = cf.SsoAddr
	cli.callbackAddr = cf.RedirectURL
	cli.displayName = cf.DisplayName
	cli.attributes.nickname = cf.Attributes.Nickname
	cli.attributes.phone = cf.Attributes.Phone
	cli.attributes.email = cf.Attributes.Email

	return &cli
}

func (s *SsoClient) Reload(cf Config) {
	s.Lock()
	defer s.Unlock()
	if !cf.Enable {
		s.enable = cf.Enable
		return
	}

	s.enable = cf.Enable
	s.config = cf
	s.ssoAddr = cf.SsoAddr
	s.callbackAddr = cf.RedirectURL
	s.displayName = cf.DisplayName
	s.attributes.nickname = cf.Attributes.Nickname
	s.attributes.phone = cf.Attributes.Phone
	s.attributes.email = cf.Attributes.Email
}

func (s *SsoClient) GetDisplayName() string {
	s.RLock()
	defer s.RUnlock()
	if !s.enable {
		return ""
	}

	return s.displayName
}

// Authorize return the cas authorize location and state
func (s *SsoClient) Authorize(redis storage.Redis, redirect string) (string, string, error) {
	state := uuid.New().String()
	ctx := context.Background()
	err := redis.Set(ctx, wrapStateKey(state), redirect, time.Duration(300*time.Second)).Err()
	if err != nil {
		return "", "", err
	}
	return s.genRedirectURL(state), state, nil
}

func fetchRedirect(ctx context.Context, state string, redis storage.Redis) (string, error) {
	return redis.Get(ctx, wrapStateKey(state)).Result()
}

func deleteRedirect(ctx context.Context, state string, redis storage.Redis) error {
	return redis.Del(ctx, wrapStateKey(state)).Err()
}

func wrapStateKey(key string) string {
	return "n9e_cas_" + key
}

func (s *SsoClient) genRedirectURL(state string) string {
	var buf bytes.Buffer
	s.RLock()
	defer s.RUnlock()

	ssoAddr, err := url.Parse(s.config.SsoAddr)
	ssoAddr.Path = "login"
	if err != nil {
		logger.Error(err)
		return buf.String()
	}

	buf.WriteString(ssoAddr.String())
	v := url.Values{
		"service": {s.callbackAddr},
	}
	if strings.Contains(s.ssoAddr, "?") {
		buf.WriteByte('&')
	} else {
		buf.WriteByte('?')
	}
	buf.WriteString(v.Encode())
	return buf.String()
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

func (s *SsoClient) ValidateServiceTicket(ctx context.Context, ticket, state string, redis storage.Redis) (ret *CallbackOutput, err error) {
	s.RLock()
	defer s.RUnlock()

	casUrl, err := url.Parse(s.config.SsoAddr)
	if err != nil {
		logger.Error(err)
		return
	}
	serviceUrl, err := url.Parse(s.callbackAddr)
	if err != nil {
		logger.Error(err)
		return
	}
	resOptions := &cas.RestOptions{
		CasURL:     casUrl,
		ServiceURL: serviceUrl,
	}
	resCli := cas.NewRestClient(resOptions)
	authRet, err := resCli.ValidateServiceTicket(cas.ServiceTicket(ticket))
	if err != nil {
		logger.Errorf("Ticket Validating Failed: %s", err)
		return
	}
	ret = &CallbackOutput{}
	ret.Username = authRet.User
	ret.Nickname = authRet.Attributes.Get(s.attributes.nickname)
	logger.Debugf("CAS Authentication Response's Attributes--[Nickname]: %s", ret.Nickname)
	ret.Email = authRet.Attributes.Get(s.attributes.email)
	logger.Debugf("CAS Authentication Response's Attributes--[Email]: %s", ret.Email)
	ret.Phone = authRet.Attributes.Get(s.attributes.phone)
	logger.Debugf("CAS Authentication Response's Attributes--[Phone]: %s", ret.Phone)
	ret.Redirect, err = fetchRedirect(ctx, state, redis)
	if err != nil {
		logger.Debugf("get redirect err:%s state:%s", state, err)
	}
	err = deleteRedirect(ctx, state, redis)
	if err != nil {
		logger.Debugf("delete redirect err:%s state:%s", state, err)
	}
	return
}
