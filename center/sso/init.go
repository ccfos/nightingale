package sso

import (
	"log"

	"github.com/BurntSushi/toml"
	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"

	"github.com/toolkits/pkg/logger"
)

type SsoClient struct {
	OIDC   *oidcx.SsoClient
	LDAP   *ldapx.SsoClient
	CAS    *cas.SsoClient
	OAuth2 *oauth2x.SsoClient
}

const LDAP = `
Enable = false
Host = 'ldap.example.org'
Port = 389
BaseDn = 'dc=example,dc=org'
BindUser = 'cn=manager,dc=example,dc=org'
BindPass = '*******'
# openldap format e.g. (&(uid=%s))
# AD format e.g. (&(sAMAccountName=%s))
AuthFilter = '(&(uid=%s))'
CoverAttributes = true
TLS = false
StartTLS = true
DefaultRoles = ['Standard']

[Attributes]
Nickname = 'cn'
Phone = 'mobile'
Email = 'mail'
`

const OAuth2 = `
Enable = false
DisplayName = 'OAuth2登录'
RedirectURL = 'http://127.0.0.1:18000/callback/oauth'
SsoAddr = 'https://sso.example.com/oauth2/authorize'
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
Username = 'username'
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`

const CAS = `
Enable = false
SsoAddr = 'https://cas.example.com/cas/'
# LoginPath = ''
RedirectURL = 'http://127.0.0.1:18000/callback/cas'
DisplayName = 'CAS登录'
CoverAttributes = false
DefaultRoles = ['Standard']

[Attributes]
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`
const OIDC = `
Enable = false
DisplayName = 'OIDC登录'
RedirectURL = 'http://n9e.com/callback'
SsoAddr = 'http://sso.example.org'
ClientId = ''
ClientSecret = ''
CoverAttributes = true
DefaultRoles = ['Standard']

[Attributes]
Nickname = 'nickname'
Phone = 'phone_number'
Email = 'email'
`

func Init(center cconf.Center, ctx *ctx.Context) *SsoClient {
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

	configs, err := models.SsoConfigGets(ctx)
	if err != nil {
		log.Fatalln(err)
	}

	for _, cfg := range configs {
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
	return ssoClient
}
