package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/didi/nightingale/src/common/identity"
	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/common/report"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/didi/nightingale/src/modules/prober/cache"
	"github.com/didi/nightingale/src/modules/prober/config"
	"github.com/didi/nightingale/src/modules/prober/core"
	"github.com/didi/nightingale/src/modules/prober/http"
	"github.com/didi/nightingale/src/modules/prober/manager"

	_ "github.com/didi/nightingale/src/modules/monapi/plugins/all"
	_ "github.com/go-sql-driver/mysql"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
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

	ctx, cancel := context.WithCancel(context.Background())

	cfg := config.Config
	identity.Parse()
	loggeri.Init(cfg.Logger)

	go stats.Init("n9e.prober")
	go report.Init(cfg.Report, "rdb")

	cache.Init(ctx)

	if cfg.Logger.Level != "DEBUG" {
		gin.SetMode(gin.ReleaseMode)
	}

	// for manager -> core.Push()
	core.InitRpcClients()

	manager.NewManager(cfg, cache.CollectRule).Start(ctx)

	http.Start()

	ending(cancel)
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/prober.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/prober.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for prober")
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
	fmt.Println("prober start, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Hostname:", runner.Hostname)
}

func ending(cancel context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	select {
	case <-c:
		fmt.Printf("stop signal caught, stopping... pid=%d\n", os.Getpid())
	}

	cancel()
	logger.Close()
	http.Shutdown()
	fmt.Printf("%s stopped successfully\n", os.Args[0])
}
