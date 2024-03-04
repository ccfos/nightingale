package ibex

import (
	"fmt"
	"os"
	"strings"

	n9eConf "github.com/ccfos/nightingale/v6/conf"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/ulricqin/ibex/src/server/config"
	"github.com/ulricqin/ibex/src/server/router"
	"github.com/ulricqin/ibex/src/server/rpc"
	"github.com/ulricqin/ibex/src/server/timer"
	"github.com/ulricqin/ibex/src/storage"
	"gorm.io/gorm"
)

var (
	Conf     *n9eConf.Ibex
	HttpPort int
)

func EdgeServerStart(rc redis.Cmdable, n9eIbex n9eConf.Ibex, api config.CenterApi, r *gin.Engine, httpPort int) {
	Conf = &n9eIbex

	config.C.IsCenter = false
	config.C.CenterApi = api
	config.C.BasicAuth = make(gin.Accounts)
	config.C.BasicAuth[n9eIbex.BasicAuthUser] = n9eIbex.BasicAuthPass
	HttpPort = httpPort

	router.ConfigRouter(r)

	storage.Cache = rc
	if err := storage.IdInit(); err != nil {
		fmt.Println("cannot init id generator: ", err)
		os.Exit(1)
	}

	rpc.Start(n9eIbex.RPCListen)

	timer.CacheHostDoing()
	timer.ReportResult()
}

func CenterServerStart(db *gorm.DB, rc redis.Cmdable, n9eIbex n9eConf.Ibex, r *gin.Engine, httpPort int) {
	Conf = &n9eIbex

	config.C.IsCenter = true
	config.C.BasicAuth = make(gin.Accounts)
	config.C.BasicAuth[n9eIbex.BasicAuthUser] = n9eIbex.BasicAuthPass
	config.C.Heartbeat.LocalAddr = schedulerAddrGet(n9eIbex.RPCListen)
	HttpPort = httpPort

	router.ConfigRouter(r)

	storage.DB = db
	storage.Cache = rc
	if err := storage.IdInit(); err != nil {
		fmt.Println("cannot init id generator: ", err)
		os.Exit(1)
	}

	rpc.Start(n9eIbex.RPCListen)

	timer.CacheHostDoing()
	timer.ReportResult()
	go timer.Heartbeat()
	go timer.Schedule()
	go timer.CleanLong()
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
