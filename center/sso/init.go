package sso

import (
	"fmt"
	"log"
	"time"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/BurntSushi/toml"
	"github.com/toolkits/pkg/logger"
)

type SsoClient struct {
	OIDC                 *oidcx.SsoClient
	LDAP                 *ldapx.SsoClient
	CAS                  *cas.SsoClient
	OAuth2               *oauth2x.SsoClient
	LastUpdateTime       int64
	configCache          *memsto.ConfigCache
	configLastUpdateTime int64
}

const LDAP = `
Enable = false
Host = 'ldap.example.org'
Port = 389
BaseDn = 'dc=example,dc=org'
BindUser = 'cn=manager,dc=example,dc=org'
BindPass = '*******'
SyncAddUsers = false
SyncDelUsers = false
# unit: s
SyncInterval = 86400
# openldap format e.g. (&(uid=%s))
# AD format e.g. (&(sAMAccountName=%s))
AuthFilter = '(&(uid=%s))'
UserFilter = '(&(uid=*))'
CoverAttributes = true
TLS = false
StartTLS = true
DefaultRoles = ['Standard']

[Attributes]
Username = 'uid'
Nickname = 'cn'
Phone = 'mobile'
Email = 'mail'
`

const OAuth2 = `
Enable = false
DisplayName = 'OAuth2登录'
RedirectURL = 'http://n9e.com/callback/oauth'
SsoAddr = 'https://sso.example.com/oauth2/authorize'
SsoLogoutAddr = 'https://sso.example.com/oauth2/authorize/session/end'
TokenAddr = 'https://sso.example.com/oauth2/token'
UserInfoAddr = 'https://api.example.com/api/v1/user/info'
TranTokenMethod = 'header'
ClientId = ''
ClientSecret = ''
CoverAttributes = true
DefaultRoles = ['Standard']
UserinfoIsArray = false
UserinfoPrefix = 'data'
Scopes = ['profile', 'email', 'phone']

[Attributes]
Username = 'sub'
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`

const CAS = `
Enable = false
DisplayName = 'CAS登录'
RedirectURL = 'http://n9e.com/callback/cas'
SsoAddr = 'https://cas.example.com/cas/'
SsoLogoutAddr = 'https://cas.example.com/cas/session/end'
# LoginPath = ''
CoverAttributes = true
DefaultRoles = ['Standard']

[Attributes]
Username = 'sub'
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`

const OIDC = `
Enable = false
DisplayName = 'OIDC登录'
RedirectURL = 'http://n9e.com/callback'
SsoAddr = 'http://sso.example.org'
SsoLogoutAddr = 'http://sso.example.org/session/end'
ClientId = ''
ClientSecret = ''
CoverAttributes = true
DefaultRoles = ['Standard']
Scopes = ['openid', 'profile', 'email', 'phone']

[Attributes]
Username = 'sub'
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`

func Init(center cconf.Center, ctx *ctx.Context, configCache *memsto.ConfigCache) *SsoClient {
	ssoClient := new(SsoClient)
	m := make(map[string]string)
	m["LDAP"] = LDAP
	m["CAS"] = CAS
	m["OIDC"] = OIDC
	m["OAuth2"] = OAuth2

	for name, config := range m {
		count, err := models.SsoConfigCountByName(ctx, name)
		if err != nil {
			logger.Error(err)
			continue
		}

		if count > 0 {
			continue
		}

		ssoConfig := models.SsoConfig{
			Name:    name,
			Content: config,
		}

		err = ssoConfig.Create(ctx)
		if err != nil {
			log.Fatalln(err)
		}
	}
	if configCache == nil {
		log.Fatalln(fmt.Errorf("configCache is nil, sso initialization failed"))
	}
	ssoClient.configCache = configCache
	userVariableMap := configCache.Get()

	configs, err := models.SsoConfigGets(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	for _, cfg := range configs {
		cfg.Content = tplx.ReplaceTemplateUseText(cfg.Name, cfg.Content, userVariableMap)
		switch cfg.Name {
		case "LDAP":
			var config ldapx.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalln("init ldap failed", err)
			}
			ssoClient.LDAP = ldapx.New(config)
		case "OIDC":
			var config oidcx.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalln("init oidc failed:", err)
			}
			logger.Info("init oidc..")
			oidcClient, err := oidcx.New(config)
			if err != nil {
				logger.Error("init oidc failed:", err)
			} else {
				ssoClient.OIDC = oidcClient
			}
		case "CAS":
			var config cas.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalln("init cas failed:", err)
			}
			ssoClient.CAS = cas.New(config)
		case "OAuth2":
			var config oauth2x.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				log.Fatalln("init oauth2 failed:", err)
			}
			ssoClient.OAuth2 = oauth2x.New(config)
		}
	}

	go ssoClient.SyncSsoUsers(ctx)
	go ssoClient.Reload(ctx)
	return ssoClient
}

// 定期更新sso配置
func (s *SsoClient) reload(ctx *ctx.Context) error {
	lastUpdateTime, err := models.SsoConfigLastUpdateTime(ctx)
	if err != nil {
		return err
	}

	lastCacheUpdateTime := s.configCache.GetLastUpdateTime()
	if lastUpdateTime == s.LastUpdateTime && lastCacheUpdateTime == s.configLastUpdateTime {
		return nil
	}

	configs, err := models.SsoConfigGets(ctx)
	if err != nil {
		return err
	}
	userVariableMap := s.configCache.Get()
	for _, cfg := range configs {
		cfg.Content = tplx.ReplaceTemplateUseText(cfg.Name, cfg.Content, userVariableMap)
		switch cfg.Name {
		case "LDAP":
			var config ldapx.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				logger.Warning("reload ldap failed", err)
				continue
			}
			s.LDAP.Reload(config)
		case "OIDC":
			var config oidcx.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				logger.Warning("reload oidc failed:", err)
				continue
			}

			logger.Info("reload oidc..")
			err = s.OIDC.Reload(config)
			if err != nil {
				logger.Error("reload oidc failed:", err)
				continue
			}
		case "CAS":
			var config cas.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				logger.Warning("reload cas failed:", err)
				continue
			}
			s.CAS.Reload(config)
		case "OAuth2":
			var config oauth2x.Config
			err := toml.Unmarshal([]byte(cfg.Content), &config)
			if err != nil {
				logger.Warning("reload oauth2 failed:", err)
				continue
			}
			s.OAuth2.Reload(config)
		}
	}

	s.LastUpdateTime = lastUpdateTime
	s.configLastUpdateTime = lastCacheUpdateTime
	return nil
}

func (s *SsoClient) Reload(ctx *ctx.Context) {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := s.reload(ctx); err != nil {
			logger.Warning("reload sso client err:", err)
		}
	}
}
