package webapi

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/toolkits/pkg/i18n"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/cas"
	"github.com/didi/nightingale/v5/src/pkg/httpx"
	"github.com/didi/nightingale/v5/src/pkg/ldapx"
	"github.com/didi/nightingale/v5/src/pkg/logx"
	"github.com/didi/nightingale/v5/src/pkg/oauth2x"
	"github.com/didi/nightingale/v5/src/pkg/oidcc"
	"github.com/didi/nightingale/v5/src/storage"
	"github.com/didi/nightingale/v5/src/webapi/config"
	"github.com/didi/nightingale/v5/src/webapi/prom"
	"github.com/didi/nightingale/v5/src/webapi/router"
	"github.com/didi/nightingale/v5/src/webapi/stat"
)

type Webapi struct {
	ConfigFile string
	Version    string
	Key        string
}

type WebapiOption func(*Webapi)

func SetConfigFile(f string) WebapiOption {
	return func(s *Webapi) {
		s.ConfigFile = f
	}
}

func SetVersion(v string) WebapiOption {
	return func(s *Webapi) {
		s.Version = v
	}
}

func SetKey(k string) WebapiOption {
	return func(s *Webapi) {
		s.Key = k
	}
}

// Run run webapi
func Run(opts ...WebapiOption) {
	code := 1
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	webapi := Webapi{
		ConfigFile: filepath.Join("etc", "webapi.conf"),
		Version:    "not specified",
	}

	for _, opt := range opts {
		opt(&webapi)
	}

	cleanFunc, err := webapi.initialize()
	if err != nil {
		fmt.Println("webapi init fail:", err)
		os.Exit(code)
	}

EXIT:
	for {
		sig := <-sc
		fmt.Println("received signal:", sig.String())
		switch sig {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			code = 0
			break EXIT
		case syscall.SIGHUP:
			// reload configuration?
		default:
			break EXIT
		}
	}

	cleanFunc()
	fmt.Println("webapi exited")
	os.Exit(code)
}

func (a Webapi) initialize() (func(), error) {
	// parse config file
	config.MustLoad(a.Key, a.ConfigFile)

	// init i18n
	i18n.Init(config.C.I18N)

	// init ldap
	ldapx.Init(config.C.LDAP)

	// init oidc
	oidcc.Init(config.C.OIDC)

	// init cas
	cas.Init(config.C.CAS)

	// init oauth
	oauth2x.Init(config.C.OAuth)

	// init logger
	loggerClean, err := logx.Init(config.C.Log)
	if err != nil {
		return nil, err
	}

	// init database
	if err = storage.InitDB(config.C.DB); err != nil {
		return nil, err
	}

	// init redis
	redisClean, err := storage.InitRedis(config.C.Redis)
	if err != nil {
		return nil, err
	}

	models.InitSalt()
	models.InitRoot()

	// init prometheus proxy config
	if err = prom.Init(); err != nil {
		return nil, err
	}

	stat.Init()

	// init http server
	r := router.New(a.Version)
	httpClean := httpx.Init(config.C.HTTP, r)

	// release all the resources
	return func() {
		loggerClean()
		httpClean()
		redisClean()
	}, nil
}
