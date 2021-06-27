package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/go-sql-driver/mysql"
	prom_runtime "github.com/prometheus/prometheus/pkg/runtime"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"

	"github.com/didi/nightingale/v5/alert"
	"github.com/didi/nightingale/v5/backend"
	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/http"
	"github.com/didi/nightingale/v5/judge"
	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/pkg/i18n"
	"github.com/didi/nightingale/v5/pkg/ilog"
	"github.com/didi/nightingale/v5/rpc"
	"github.com/didi/nightingale/v5/timer"
	"github.com/didi/nightingale/v5/trans"
)

var version = "not specified"

var (
	vers *bool
	help *bool
)

func init() {
	vers = flag.Bool("v", false, "display the version.")
	help = flag.Bool("h", false, "print this help.")
	flag.Parse()

	if *vers {
		fmt.Println("version:", version)
		os.Exit(0)
	}

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	runner.Init()
	fmt.Println("runner.cwd:", runner.Cwd)
	fmt.Println("runner.hostname:", runner.Hostname)
	fmt.Println("fd_limits", prom_runtime.FdLimits())
	fmt.Println("vm_limits", prom_runtime.VMLimits())
}

func main() {
	parseConf()

	ilog.Init(config.Config.Logger)
	i18n.Init(config.Config.I18N)

	models.InitMySQL(config.Config.MySQL)
	models.InitLdap(config.Config.LDAP)
	models.InitSalt()
	models.InitRoot()
	models.InitError()

	ctx, cancelFunc := context.WithCancel(context.Background())

	timer.SyncResourceTags()
	timer.SyncUsers()
	timer.SyncUserGroupMember()
	timer.SyncClasspathReses()
	timer.SyncCollectRules()
	timer.SyncAlertMutes()
	timer.SyncAlertRules()
	timer.SyncMetricDesc()
	timer.CleanExpireMute()
	timer.CleanExpireResource()
	timer.BindOrphanRes()
	timer.UpdateAlias()

	judge.Start(ctx)
	alert.Start(ctx)
	trans.Start(ctx)

	backend.Init(config.Config.Trans.Backend)

	http.Start()
	rpc.Start()

	endingProc(cancelFunc)
}

func parseConf() {
	if err := config.Parse(); err != nil {
		fmt.Println("cannot parse configuration file:", err)
		os.Exit(1)
	}
}

func endingProc(cancelFunc context.CancelFunc) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	<-c
	fmt.Printf("stop signal caught, stopping... pid=%d\n", os.Getpid())

	// 执行清理工作
	backend.DatasourceCleanUp()
	cancelFunc()
	logger.Close()
	http.Shutdown()

	fmt.Println("process stopped successfully")
}
