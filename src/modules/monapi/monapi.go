package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/go-sql-driver/mysql"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/cron"
	"github.com/didi/nightingale/src/modules/monapi/http"
	"github.com/didi/nightingale/src/modules/monapi/mcache"
	"github.com/didi/nightingale/src/modules/monapi/redisc"
	"github.com/didi/nightingale/src/modules/monapi/scache"
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

	runner.Init()
	fmt.Println("runner.cwd:", runner.Cwd)
	fmt.Println("runner.hostname:", runner.Hostname)
}

func main() {
	aconf()
	pconf()

	config.InitLogger()
	go stats.Init("n9e.monapi")

	model.InitMySQL("uic", "mon", "hbs")
	model.InitRoot()
	model.InitNode()

	scache.Init()
	mcache.Init()

	if err := cron.SyncMaskconf(); err != nil {
		log.Fatalf("sync maskconf fail: %v", err)
	}

	if err := cron.SyncStra(); err != nil {
		log.Fatalf("sync stra fail: %v", err)
	}

	if err := cron.CheckJudge(); err != nil {
		log.Fatalf("check judge fail: %v", err)
	}

	redisc.InitRedis()

	go cron.SyncMaskconfLoop()
	go cron.SyncStraLoop()
	go cron.CleanStraLoop()
	go cron.CleanCollectLoop()
	go cron.EventConsumer()
	go cron.CallbackConsumer()
	go cron.CleanEventLoop()
	go cron.CheckJudgeLoop()

	http.Start()
	ending()
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
	fmt.Println("portal stopped successfully")
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/monapi.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/monapi.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for portal")
	os.Exit(1)
}

// parse configuration file
func pconf() {
	if err := config.Parse(*conf); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	} else {
		fmt.Println("portal start, use configuration file:", *conf)
	}
}
