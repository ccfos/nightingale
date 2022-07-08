package config

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/koding/multiconfig"

	"github.com/didi/nightingale/v5/src/pkg/httpx"
	"github.com/didi/nightingale/v5/src/pkg/ldapx"
	"github.com/didi/nightingale/v5/src/pkg/logx"
	"github.com/didi/nightingale/v5/src/pkg/oidcc"
	"github.com/didi/nightingale/v5/src/pkg/ormx"
	"github.com/didi/nightingale/v5/src/storage"
)

var (
	C    = new(Config)
	once sync.Once
)

func MustLoad(fpaths ...string) {
	once.Do(func() {
		loaders := []multiconfig.Loader{
			&multiconfig.TagLoader{},
			&multiconfig.EnvironmentLoader{},
		}

		for _, fpath := range fpaths {
			handled := false

			if strings.HasSuffix(fpath, "toml") {
				loaders = append(loaders, &multiconfig.TOMLLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "conf") {
				loaders = append(loaders, &multiconfig.TOMLLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "json") {
				loaders = append(loaders, &multiconfig.JSONLoader{Path: fpath})
				handled = true
			}
			if strings.HasSuffix(fpath, "yaml") {
				loaders = append(loaders, &multiconfig.YAMLLoader{Path: fpath})
				handled = true
			}

			if !handled {
				fmt.Println("config file invalid, valid file exts: .conf,.yaml,.toml,.json")
				os.Exit(1)
			}
		}

		m := multiconfig.DefaultLoader{
			Loader:    multiconfig.MultiLoader(loaders...),
			Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
		}

		m.MustLoad(C)

		if !strings.HasPrefix(C.Ibex.Address, "http") {
			C.Ibex.Address = "http://" + C.Ibex.Address
		}

		err := loadMetricsYaml()
		if err != nil {
			fmt.Println("failed to load metrics.yaml:", err)
			os.Exit(1)
		}
	})
}

type Config struct {
	RunMode              string
	I18N                 string
	AdminRole            string
	MetricsYamlFile      string
	BuiltinAlertsDir     string
	BuiltinDashboardsDir string
	ClustersFrom         string
	ClustersFromAPIs     []string
	ContactKeys          []LabelAndKey
	NotifyChannels       []LabelAndKey
	Log                  logx.Config
	HTTP                 httpx.Config
	JWTAuth              JWTAuth
	ProxyAuth            ProxyAuth
	BasicAuth            gin.Accounts
	AnonymousAccess      AnonymousAccess
	LDAP                 ldapx.LdapSection
	Redis                storage.RedisConfig
	DB                   ormx.DBConfig
	Clusters             []ClusterOptions
	Ibex                 Ibex
	OIDC                 oidcc.Config
}

type ClusterOptions struct {
	Name string
	Prom string

	BasicAuthUser string
	BasicAuthPass string

	Headers []string

	Timeout     int64
	DialTimeout int64
	KeepAlive   int64

	MaxIdleConnsPerHost int
}

type LabelAndKey struct {
	Label string `json:"label"`
	Key   string `json:"key"`
}

func LabelAndKeyHasKey(keys []LabelAndKey, key string) bool {
	for i := 0; i < len(keys); i++ {
		if keys[i].Key == key {
			return true
		}
	}
	return false
}

type JWTAuth struct {
	SigningKey     string
	AccessExpired  int64
	RefreshExpired int64
	RedisKeyPrefix string
}

type ProxyAuth struct {
	Enable            bool
	HeaderUserNameKey string
	DefaultRoles      []string
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

func (c *Config) IsDebugMode() bool {
	return c.RunMode == "debug"
}
