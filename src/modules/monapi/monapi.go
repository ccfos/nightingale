package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/acache"
	"github.com/didi/nightingale/src/modules/monapi/alarm"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/http"
	"github.com/didi/nightingale/src/modules/monapi/redisc"
	"github.com/didi/nightingale/src/modules/monapi/scache"
	"github.com/didi/nightingale/src/toolkits/i18n"

	_ "github.com/didi/nightingale/src/modules/monapi/plugins/all"
	_ "github.com/go-sql-driver/mysql"

	"github.com/toolkits/pkg/cache"
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

	runner.Init()
	fmt.Println("monapi start, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Hostname:", runner.Hostname)
}

func main() {
	aconf()
	pconf()

	cache.InitMemoryCache(time.Hour)
	config.InitLogger()
	models.InitMySQL("mon", "rdb")

	scache.Init()

	i18n.Init(config.Get().I18n)

	if err := scache.CheckJudge(); err != nil {
		logger.Errorf("check judge fail: %v", err)
	}

	if config.Get().AlarmEnabled {
		acache.Init()

		if err := alarm.SyncMaskconf(); err != nil {
			log.Fatalf("sync maskconf fail: %v", err)
		}

		if err := alarm.SyncStra(); err != nil {
			log.Fatalf("sync stra fail: %v", err)
		}

		redisc.InitRedis()

		go alarm.SyncMaskconfLoop()
		go alarm.SyncStraLoop()
		go alarm.CleanStraLoop()
		go alarm.ReadHighEvent()
		go alarm.ReadLowEvent()
		go alarm.CallbackConsumer()
		go alarm.MergeEvent()
		go alarm.CleanEventLoop()
	}

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
	fmt.Println("monapi stopped successfully")
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

	fmt.Println("no configuration file for monapi")
	os.Exit(1)
}

// parse configuration file
func pconf() {
	if err := config.Parse(*conf); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	}
}
