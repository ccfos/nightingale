package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/common/report"
	"github.com/didi/nightingale/src/modules/index/cache"
	"github.com/didi/nightingale/src/modules/index/config"
	"github.com/didi/nightingale/src/modules/index/http/routes"
	"github.com/didi/nightingale/src/modules/index/rpc"
	"github.com/didi/nightingale/src/toolkits/http"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

var (
	vers *bool
	help *bool
	conf *string

	version = "No Version Provided"
)

func init() {
	vers = flag.Bool("v", false, "display the version.")
	help = flag.Bool("h", false, "print this help.")
	conf = flag.String("f", "", "specify configuration file.")
	flag.Parse()

	if *vers {
		fmt.Println("Version:", version)
		os.Exit(0)
	}

	if *help {
		flag.Usage()
		os.Exit(0)
	}
}

func main() {
	aconf()
	pconf()
	start()

	cfg := config.Config

	loggeri.Init(cfg.Logger)
	go stats.Init("n9e.index")

	identity.Parse()
	cache.InitDB(cfg.Cache)

	go report.Init(cfg.Report, "rdb")
	go rpc.Start()

	http.Start(routes.Config, "index", cfg.Logger.Level)

	http.GracefulShutdown()
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/index.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/index.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for index")
	os.Exit(1)
}

// parse configuration file
func pconf() {
	if err := config.Parse(*conf); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	}
}

func start() {
	runner.Init()
	fmt.Println("index start, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Hostname:", runner.Hostname)
}
