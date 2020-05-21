package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/didi/nightingale/src/modules/transfer/config"
	"github.com/didi/nightingale/src/modules/transfer/cron"
	"github.com/didi/nightingale/src/modules/transfer/http/routes"
	"github.com/didi/nightingale/src/modules/transfer/rpc"
	"github.com/didi/nightingale/src/toolkits/http"
	tlogger "github.com/didi/nightingale/src/toolkits/logger"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

var (
	vers *bool
	help *bool
	conf *string

	version   = "No Version Provided"
	gitHash   = "No GitHash Provided"
	buildTime = "No BuildTime Provided"
)

func init() {
	vers = flag.Bool("v", false, "display the version.")
	help = flag.Bool("h", false, "print this help.")
	conf = flag.String("f", "", "specify configuration file.")
	flag.Parse()

	if *vers {
		fmt.Println("Version:", version)
		fmt.Println("Git Commit Hash:", gitHash)
		fmt.Println("UTC Build Time:", buildTime)
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

	tlogger.Init(cfg.Logger)
	go stats.Init("n9e.transfer")

	backend.Init(cfg.Backend)
	cron.Init()

	go rpc.Start()

	r := gin.New()
	routes.Config(r)
	go http.Start(r, "transfer", cfg.Logger.Level)

	cleanup()
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/transfer.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/transfer.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for transfer")
	os.Exit(1)
}

// parse configuration file
func pconf() {
	if err := config.Parse(*conf); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	}
}

func cleanup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	select {
	case <-c:
		fmt.Println("stop signal caught, stopping... pid=", os.Getpid())
	}

	logger.Close()
	http.Shutdown()
	fmt.Println("sender stopped successfully")
}

func start() {
	runner.Init()
	fmt.Println("transfer started, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Hostname:", runner.Hostname)
}
