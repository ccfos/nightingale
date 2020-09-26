package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/modules/agent/cache"
	"github.com/didi/nightingale/src/modules/agent/config"
	"github.com/didi/nightingale/src/modules/agent/core"
	"github.com/didi/nightingale/src/modules/agent/http"
	"github.com/didi/nightingale/src/modules/agent/log/worker"
	"github.com/didi/nightingale/src/modules/agent/report"
	"github.com/didi/nightingale/src/modules/agent/stra"
	"github.com/didi/nightingale/src/modules/agent/sys"
	"github.com/didi/nightingale/src/modules/agent/sys/funcs"
	"github.com/didi/nightingale/src/modules/agent/sys/plugins"
	"github.com/didi/nightingale/src/modules/agent/sys/ports"
	"github.com/didi/nightingale/src/modules/agent/sys/procs"
	"github.com/didi/nightingale/src/modules/agent/timer"

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
	fmt.Println("runner.cwd:", runner.Cwd)
	fmt.Println("runner.hostname:", runner.Hostname)
}

func main() {
	parseConf()

	loggeri.Init(config.Config.Logger)

	if config.Config.Enable.Mon {
		monStart()
	}

	if config.Config.Enable.Job {
		jobStart()
	}

	if config.Config.Enable.Report {
		reportStart()
	}

	http.Start()

	endingProc()
}

func reportStart() {
	if err := report.GatherBase(); err != nil {
		fmt.Println("gatherBase fail: ", err)
		os.Exit(1)
	}

	go report.LoopReport()
}

func jobStart() {
	go timer.Heartbeat()
}

func monStart() {
	sys.Init(config.Config.Sys)
	stra.Init()

	core.InitRpcClients()
	funcs.BuildMappers()
	funcs.Collect()

	//插件采集
	plugins.Detect()

	//进程采集
	procs.Detect()

	//端口采集
	ports.Detect()

	//初始化缓存，用作保存COUNTER类型数据
	cache.Init()

	//日志采集
	go worker.UpdateConfigsLoop()
	go worker.PusherStart()
	go worker.Zeroize()
}

func parseConf() {
	if err := config.Parse(); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	}
}

func endingProc() {
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
