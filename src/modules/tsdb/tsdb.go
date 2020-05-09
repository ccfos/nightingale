package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	brpc "github.com/didi/nightingale/src/modules/tsdb/backend/rpc"
	"github.com/didi/nightingale/src/modules/tsdb/cache"
	"github.com/didi/nightingale/src/modules/tsdb/config"
	"github.com/didi/nightingale/src/modules/tsdb/http"
	"github.com/didi/nightingale/src/modules/tsdb/index"
	"github.com/didi/nightingale/src/modules/tsdb/migrate"
	"github.com/didi/nightingale/src/modules/tsdb/rpc"
	"github.com/didi/nightingale/src/modules/tsdb/rrdtool"
	tlogger "github.com/didi/nightingale/src/toolkits/logger"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/file"
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
	go stats.Init("n9e.tsdb")

	// INIT
	cache.Init(cfg.Cache)
	index.Init(cfg.Index)
	brpc.Init(cfg.RpcClient, index.IndexList.Get())

	cache.InitChunkSlot()
	rrdtool.Init(cfg.RRD)

	migrate.Init(cfg.Migrate) //读数据加队列

	go http.Start()
	go rpc.Start()

	startSignal(os.Getpid())
}

// auto detect configuration file
func aconf() {
	if *conf != "" && file.IsExist(*conf) {
		return
	}

	*conf = "etc/tsdb.local.yml"
	if file.IsExist(*conf) {
		return
	}

	*conf = "etc/tsdb.yml"
	if file.IsExist(*conf) {
		return
	}

	fmt.Println("no configuration file for tsdb")
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
	fmt.Println("tsdb start, use configuration file:", *conf)
	fmt.Println("runner.Cwd:", runner.Cwd)
	fmt.Println("runner.Hostname:", runner.Hostname)
}

func startSignal(pid int) {
	cfg := config.Config
	sigs := make(chan os.Signal, 1)
	log.Printf("%d register signal notify", pid)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	for {
		s := <-sigs
		log.Println("recv", s)

		switch s {
		case syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			log.Println("graceful shut down")

			if cfg.Http.Enabled {
				http.Close_chan <- 1
				<-http.Close_done_chan
			}
			log.Println("http stop ok")

			if cfg.Rpc.Enabled {
				rpc.Close_chan <- 1
				<-rpc.Close_done_chan
			}
			log.Println("rpc stop ok")

			cache.FlushDoneChan <- 1
			rrdtool.Persist()
			log.Println("====================== tsdb stop ok ======================")
			log.Println(pid, "exit")
			os.Exit(0)
		}
	}
}
