package ibex

import (
	"fmt"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"os"
	"strings"

	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/ibex/server/router"
	"github.com/ccfos/nightingale/v6/ibex/server/rpc"
	"github.com/ccfos/nightingale/v6/ibex/server/timer"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	n9eRouter "github.com/ccfos/nightingale/v6/center/router"
	"github.com/ccfos/nightingale/v6/conf"
	n9eConf "github.com/ccfos/nightingale/v6/conf"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	HttpPort int
)

func ServerStart(ctx *ctx.Context, isCenter bool, db *gorm.DB, rc redis.Cmdable, basicAuth gin.Accounts, heartbeat aconf.HeartbeatConfig,
	api *n9eConf.CenterApi, r *gin.Engine, centerRouter *n9eRouter.Router, ibex conf.Ibex, httpPort int) {
	config.C.IsCenter = isCenter
	config.C.BasicAuth = make(gin.Accounts)
	if len(basicAuth) > 0 {
		config.C.BasicAuth = basicAuth
	}

	config.C.Heartbeat.IP = heartbeat.IP
	config.C.Heartbeat.Interval = heartbeat.Interval
	config.C.Heartbeat.LocalAddr = schedulerAddrGet(ibex.RPCListen)
	HttpPort = httpPort

	config.C.Output.ComeFrom = ibex.Output.ComeFrom
	config.C.Output.AgtdPort = ibex.Output.AgtdPort

	rou := router.NewRouter(ctx)

	if centerRouter != nil {
		rou.ConfigRouter(r, centerRouter)
	} else {
		rou.ConfigRouter(r)
	}

	storage.IbexCache = rc
	if err := storage.IdInit(); err != nil {
		fmt.Println("cannot init id generator: ", err)
		os.Exit(1)
	}

	rpc.Start(ibex.RPCListen, ctx)

	if isCenter {
		storage.IbexDB = db

		go timer.Heartbeat(ctx)
		go timer.Schedule(ctx)
		go timer.CleanLong(ctx)
	} else {
		config.C.CenterApi = *api
	}

	timer.CacheHostDoing(ctx)
	timer.ReportResult(ctx)
}

func schedulerAddrGet(rpcListen string) string {
	ip := fmt.Sprint(config.GetOutboundIP())
	if ip == "" {
		fmt.Println("heartbeat ip auto got is blank")
		os.Exit(1)
	}

	port := strings.Split(rpcListen, ":")[1]
	localAddr := ip + ":" + port
	return localAddr
}
