package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ccfos/nightingale/v6/ibex/pkg/httpx"
	"github.com/ccfos/nightingale/v6/ibex/pkg/logx"
	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/ibex/server/router"
	"github.com/ccfos/nightingale/v6/ibex/server/rpc"
	"github.com/ccfos/nightingale/v6/ibex/server/timer"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/i18n"
)

type Server struct {
	ConfigFile string
	Version    string
}

type ServerOption func(*Server)

func SetConfigFile(f string) ServerOption {
	return func(s *Server) {
		s.ConfigFile = f
	}
}

func SetVersion(v string) ServerOption {
	return func(s *Server) {
		s.Version = v
	}
}

// Run run server
func Run(isCenter bool, opts ...ServerOption) {
	code := 1
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	server := Server{
		ConfigFile: filepath.Join("etc", "server.conf"),
		Version:    "not specified",
	}

	for _, opt := range opts {
		opt(&server)
	}

	// parse config file
	config.MustLoad(server.ConfigFile)
	config.C.IsCenter = isCenter

	cleanFunc, err := server.initialize()
	if err != nil {
		fmt.Println("server init fail:", err)
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
	fmt.Println("server exited")
	os.Exit(code)
}

func (s Server) initialize() (func(), error) {
	fns := Functions{}
	ctx, cancel := context.WithCancel(context.Background())
	fns.Add(cancel)

	// init i18n
	i18n.Init()

	// init logger
	loggerClean, err := logx.Init(config.C.Log)
	if err != nil {
		return fns.Ret(), err
	} else {
		fns.Add(loggerClean)
	}

	// init database
	if config.C.IsCenter {
		if err = storage.InitIbexDB(config.C.DB); err != nil {
			return fns.Ret(), err
		}
	}
	if err = storage.InitRedis(config.C.Redis); err != nil {
		return fns.Ret(), err
	}

	timer.CacheHostDoing()
	timer.ReportResult()
	if config.C.IsCenter {
		go timer.Heartbeat()
		go timer.Schedule()
		go timer.CleanLong()
	}
	// init http server
	r := router.New(s.Version)
	httpClean := httpx.Init(config.C.HTTP, ctx, r)
	fns.Add(httpClean)

	// start rpc server
	rpc.Start(config.C.RPC.Listen)

	// release all the resources
	return fns.Ret(), nil
}

type Functions struct {
	List []func()
}

func (fs *Functions) Add(f func()) {
	fs.List = append(fs.List, f)
}

func (fs *Functions) Ret() func() {
	return func() {
		for i := 0; i < len(fs.List); i++ {
			fs.List[i]()
		}
	}
}
