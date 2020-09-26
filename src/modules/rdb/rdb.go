package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/go-sql-driver/mysql"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"

	"github.com/didi/nightingale/src/common/loggeri"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/didi/nightingale/src/modules/rdb/cron"
	"github.com/didi/nightingale/src/modules/rdb/http"
	"github.com/didi/nightingale/src/modules/rdb/redisc"
	"github.com/didi/nightingale/src/modules/rdb/ssoc"
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

	// 初始化数据库和相关数据
	models.InitMySQL("rdb")
	models.InitSalt()
	models.InitRooter()

	ssoc.InitSSO()

	// 初始化redis用来发送邮件短信等
	redisc.InitRedis()
	cron.InitWorker()

	go cron.ConsumeMail()
	go cron.ConsumeSms()
	go cron.ConsumeVoice()
	go cron.ConsumeIm()

	http.Start()

	endingProc()
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
	redisc.CloseRedis()
	fmt.Println("portal stopped successfully")
}
