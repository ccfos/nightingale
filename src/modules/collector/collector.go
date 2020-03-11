package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/didi/nightingale/src/modules/collector/config"
	"github.com/didi/nightingale/src/modules/collector/http/routes"
	"github.com/didi/nightingale/src/modules/collector/log/worker"
	"github.com/didi/nightingale/src/modules/collector/stra"
	"github.com/didi/nightingale/src/modules/collector/sys"
	"github.com/didi/nightingale/src/modules/collector/sys/funcs"
	"github.com/didi/nightingale/src/modules/collector/sys/plugins"
	"github.com/didi/nightingale/src/modules/collector/sys/ports"
	"github.com/didi/nightingale/src/modules/collector/sys/procs"
	"github.com/didi/nightingale/src/toolkits/http"
	"github.com/didi/nightingale/src/toolkits/identity"
	tlogger "github.com/didi/nightingale/src/toolkits/logger"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"
)

const version = 1

var (
	vers *bool
	help *bool
	conf *string
)

func init() {
	vers = flag.Bool("v", false, "display the version.")
	help = flag.Bool("h", false, "print this help.")
	conf = flag.String("f", "", "specify configuration file.")
	flag.Parse()

	if *vers {
		fmt.Println("version:", version)
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
	cfg := config.Get()

	tlogger.Init(cfg.Logger)

	identity.Init(cfg.Identity)
	if identity.Identity == "127.0.0.1" {
		log.Fatalln("endpoint: 127.0.0.1, cannot work")
	}

	sys.Init(cfg.Sys)
	stra.Init(cfg.Stra)

	funcs.BuildMappers()
	funcs.Collect()
	stra.GetCollects()

	//插件采集
	plugins.Detect()

	//进程采集
	procs.Detect()

	//端口采集
	ports.Detect()

	//日志采集
	worker.Init(config.Config.Worker)
	go worker.UpdateConfigsLoop()
	go worker.PusherStart()
	go worker.Zeroize()

	r := gin.New()
	routes.Config(r)
	http.Start(r, "collector", cfg.Logger.Level)
	ending()
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/collector.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/collector.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for collector")
	os.Exit(1)
}

// parse configuration file
func pconf() {
	if err := config.Parse(*conf); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	}
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
	fmt.Println("sender stopped successfully")
}

func start() {
	runner.Init()
	fmt.Println("collector start, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Endpoint:", runner.Hostname)
}
