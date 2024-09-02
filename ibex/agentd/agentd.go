package agentd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/toolkits/pkg/i18n"

	"github.com/ccfos/nightingale/v6/ibex/agentd/config"
	"github.com/ccfos/nightingale/v6/ibex/agentd/router"
	"github.com/ccfos/nightingale/v6/ibex/agentd/timer"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
)

type Agentd struct {
	ConfigFile string
	Version    string
}

type AgentdOption func(*Agentd)

func SetConfigFile(f string) AgentdOption {
	return func(s *Agentd) {
		s.ConfigFile = f
	}
}

func SetVersion(v string) AgentdOption {
	return func(s *Agentd) {
		s.Version = v
	}
}

// Run run agentd
func Run(opts ...AgentdOption) {
	code := 1
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	agentd := Agentd{
		ConfigFile: filepath.Join("etc", "ibex", "agentd.toml"),
		Version:    "not specified",
	}

	for _, opt := range opts {
		opt(&agentd)
	}

	cleanFunc, err := agentd.initialize()
	if err != nil {
		fmt.Println("agentd init fail:", err)
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
	fmt.Println("agentd exited")
	os.Exit(code)
}

func (s Agentd) initialize() (func(), error) {
	fns := Functions{}
	ctx, cancel := context.WithCancel(context.Background())
	fns.Add(cancel)

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// parse config file
	config.MustLoad(s.ConfigFile)

	// init i18n
	i18n.Init()

	// init http server
	r := router.New(s.Version)
	httpClean := httpx.Init(config.C.HTTP, ctx, r)
	fns.Add(httpClean)

	go timer.Heartbeat(ctx)

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
