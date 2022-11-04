package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/toolkits/pkg/i18n"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/pkg/httpx"
	"github.com/didi/nightingale/v5/src/pkg/logx"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/engine"
	"github.com/didi/nightingale/v5/src/server/idents"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/didi/nightingale/v5/src/server/naming"
	"github.com/didi/nightingale/v5/src/server/router"
	"github.com/didi/nightingale/v5/src/server/stat"
	"github.com/didi/nightingale/v5/src/server/usage"
	"github.com/didi/nightingale/v5/src/server/writer"
	"github.com/didi/nightingale/v5/src/storage"
)

type Server struct {
	ConfigFile string
	Version    string
	Key        string
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

func SetKey(k string) ServerOption {
	return func(s *Server) {
		s.Key = k
	}
}

// Run run server
func Run(opts ...ServerOption) {
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
			reload()
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

	// parse config file
	config.MustLoad(s.Key, s.ConfigFile)

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
	if err = storage.InitDB(config.C.DB); err != nil {
		return fns.Ret(), err
	}

	// init redis
	redisClean, err := storage.InitRedis(config.C.Redis)
	if err != nil {
		return fns.Ret(), err
	} else {
		fns.Add(redisClean)
	}

	// init prometheus remote writers
	if err = writer.Init(config.C.Writers, config.C.WriterOpt); err != nil {
		return fns.Ret(), err
	}

	// init prometheus remote reader
	if err = config.InitReader(); err != nil {
		return fns.Ret(), err
	}

	// sync rules/users/mutes/targets to memory cache
	memsto.Sync()

	// start heartbeat
	if err = naming.Heartbeat(ctx); err != nil {
		return fns.Ret(), err
	}

	// start judge engine
	if err = engine.Start(ctx); err != nil {
		return fns.Ret(), err
	}

	stat.Init()

	// init http server
	r := router.New(s.Version, reload)
	httpClean := httpx.Init(config.C.HTTP, r)
	fns.Add(httpClean)

	// register ident and nodata logic
	idents.Handle(ctx)

	if !config.C.DisableUsageReport {
		go usage.Report()
	}

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

func reload() {
	logger.Info("start reload configs")
	engine.Reload()
	logger.Info("reload configs finished")
}
