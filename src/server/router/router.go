package router

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/pkg/aop"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/naming"

	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

func New(version string) *gin.Engine {
	gin.SetMode(config.C.RunMode)

	loggerMid := aop.Logger()
	recoveryMid := aop.Recovery()

	if strings.ToLower(config.C.RunMode) == "release" {
		aop.DisableConsoleColor()
	}

	r := gin.New()

	r.Use(recoveryMid)

	// whether print access log
	if config.C.HTTP.PrintAccessLog {
		r.Use(loggerMid)
	}

	configRoute(r, version)

	return r
}

func configRoute(r *gin.Engine, version string) {
	if config.C.HTTP.PProf {
		pprof.Register(r, "/api/debug/pprof")
	}

	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	r.GET("/pid", func(c *gin.Context) {
		c.String(200, fmt.Sprintf("%d", os.Getpid()))
	})

	r.GET("/addr", func(c *gin.Context) {
		c.String(200, c.Request.RemoteAddr)
	})

	r.GET("/version", func(c *gin.Context) {
		c.String(200, version)
	})

	r.GET("/servers/active", func(c *gin.Context) {
		lst, err := naming.ActiveServers(c.Request.Context(), config.C.ClusterName)
		ginx.NewRender(c).Data(lst, err)
	})

	// use apiKey not basic auth
	r.POST("/datadog/api/v1/series", stat(), datadogSeries)
	r.POST("/datadog/api/v1/check_run", datadogCheckRun)
	r.GET("/datadog/api/v1/validate", datadogValidate)
	r.POST("/datadog/api/v1/metadata", datadogMetadata)
	r.POST("/datadog/intake/", datadogIntake)

	if len(config.C.BasicAuth) > 0 {
		auth := gin.BasicAuth(config.C.BasicAuth)
		r.Use(auth)
	}

	r.POST("/opentsdb/put", stat(), handleOpenTSDB)
	r.POST("/openfalcon/push", stat(), falconPush)
	r.POST("/prometheus/v1/write", stat(), remoteWrite)
	r.POST("/prometheus/v1/query", stat(), queryPromql)

	r.GET("/memory/alert-rule", alertRuleGet)
	r.GET("/memory/alert-rule-location", alertRuleLocationGet)
	r.GET("/memory/idents", identsGets)
	r.GET("/memory/alert-mutes", mutesGets)
	r.GET("/memory/alert-subscribes", subscribesGets)
	r.GET("/memory/target", targetGet)
	r.GET("/memory/user", userGet)
	r.GET("/memory/user-group", userGroupGet)

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	service := r.Group("/v1/n9e")
	service.POST("/event", pushEventToQueue)
}

func stat() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		code := fmt.Sprintf("%d", c.Writer.Status())
		method := c.Request.Method
		labels := []string{code, c.FullPath(), method}

		promstat.RequestDuration.WithLabelValues(labels...).Observe(time.Since(start).Seconds())
	}
}
