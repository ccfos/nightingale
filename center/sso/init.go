package sso

import (
	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"

	"github.com/pelletier/go-toml/v2"
	"github.com/toolkits/pkg/logger"
)

type SsoClient struct {
	OIDC   *oidcx.SsoClient
	LDAP   *ldapx.SsoClient
	CAS    *cas.SsoClient
	OAuth2 *oauth2x.SsoClient
}

func Init(center cconf.Center, ctx *ctx.Context) *SsoClient {
	ssoClient := new(SsoClient)
	m := make(map[string]interface{})
	m["LDAP"] = center.LDAP
	m["CAS"] = center.CAS
	m["OIDC"] = center.OIDC
	m["OAuth2"] = center.OAuth

	for name, config := range m {
		b, err := toml.Marshal(config)
		if err != nil {
			logger.Error(err)
			continue
		}

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
			Content: string(b),
		}

		err = ssoConfig.Create(ctx)
		if err != nil {
			logger.Error(err)
		}
	}

	// init ldap
	ssoClient.LDAP = ldapx.New(center.LDAP)

	// init oidc
	oidcClient, err := oidcx.New(center.OIDC)
	if err != nil {
		logger.Error("init oidc failed: %v", err)
	} else {
		ssoClient.OIDC = oidcClient
	}

	// init cas
	ssoClient.CAS = cas.New(center.CAS)

	// init oauth
	ssoClient.OAuth2 = oauth2x.New(center.OAuth)
	return ssoClient
}
