package cconf

import (
	"github.com/ccfos/nightingale/v6/pkg/cas"
	"github.com/ccfos/nightingale/v6/pkg/ldapx"
	"github.com/ccfos/nightingale/v6/pkg/oauth2x"
	"github.com/ccfos/nightingale/v6/pkg/oidcx"

	"github.com/gin-gonic/gin"
)

type Center struct {
	Plugins                []Plugin
	BasicAuth              gin.Accounts
	MetricsYamlFile        string
	OpsYamlFile            string
	BuiltinIntegrationsDir string
	I18NHeaderKey          string
	MetricDesc             MetricDescType
	TargetMetrics          map[string]string
	AnonymousAccess        AnonymousAccess
	JWTAuth                JWTAuth
	ProxyAuth              ProxyAuth
	LDAP                   ldapx.Config
	OIDC                   oidcx.Config
	CAS                    cas.Config
	OAuth                  oauth2x.Config
	Ibex                   Ibex
}

type Plugin struct {
	Id       int64  `json:"id"`
	Category string `json:"category"`
	Type     string `json:"plugin_type"`
	TypeName string `json:"plugin_type_name"`
}

type ProxyAuth struct {
	Enable            bool
	HeaderUserNameKey string
	DefaultRoles      []string
}

type JWTAuth struct {
	SigningKey     string
	AccessExpired  int64
	RefreshExpired int64
	RedisKeyPrefix string
}

type AnonymousAccess struct {
	PromQuerier bool
	AlertDetail bool
}

type Ibex struct {
	Address       string
	BasicAuthUser string
	BasicAuthPass string
	Timeout       int64
}

func (c *Center) PreCheck() {
	if len(c.Plugins) == 0 {
		c.Plugins = Plugins
	}
}
