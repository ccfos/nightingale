package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"

	"github.com/didi/nightingale/src/modules/judge/backend/query"
	"github.com/didi/nightingale/src/modules/judge/backend/redi"
	"github.com/didi/nightingale/src/modules/judge/cache"
	"github.com/didi/nightingale/src/modules/judge/config"
	"github.com/didi/nightingale/src/modules/judge/http/routes"
	"github.com/didi/nightingale/src/modules/judge/judge"
	"github.com/didi/nightingale/src/modules/judge/rpc"
	"github.com/didi/nightingale/src/modules/judge/stra"
	"github.com/didi/nightingale/src/toolkits/http"
	"github.com/didi/nightingale/src/toolkits/identity"
	tlogger "github.com/didi/nightingale/src/toolkits/logger"
	"github.com/didi/nightingale/src/toolkits/report"
	"github.com/didi/nightingale/src/toolkits/stats"
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
	identity.Init(cfg.Identity)
	tlogger.Init(cfg.Logger)
	go stats.Init("n9e.judge")

	query.Init(cfg.Query)
	redi.Init(cfg.Redis)

	cache.InitHistoryBigMap()
	cache.Strategy = cache.NewStrategyMap()
	cache.NodataStra = cache.NewStrategyMap()
	cache.SeriesMap = cache.NewIndexMap()

	go rpc.Start()

	go stra.GetStrategy(cfg.Strategy)
	go judge.NodataJudge(cfg.NodataConcurrency)
	go report.Init(cfg.Report, "monapi")

	r := gin.New()
	routes.Config(r)
	go http.Start(r, "judge", cfg.Logger.Level)

	ending()
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/judge.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/judge.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for judge")
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
	fmt.Println("transfer start, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Hostname:", runner.Hostname)
}

func ending() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	select {
	case <-c:
		fmt.Printf("stop signal caught, stopping... pid=%d\n", os.Getpid())
	}

	logger.Close()
	http.Shutdown()
	redi.CloseRedis()
	fmt.Println("alarm stopped successfully")
}
