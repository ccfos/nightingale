package router

import (
	"fmt"

	"os"
	"strings"

	"github.com/ccfos/nightingale/v6/ibex/pkg/aop"
	"github.com/ccfos/nightingale/v6/ibex/server/config"

	"github.com/ccfos/nightingale/v6/center/router"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
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

	configBaseRouter(r, version)
	ConfigRouter(r)

	return r
}

func configBaseRouter(r *gin.Engine, version string) {
	if config.C.HTTP.PProf {
		pprof.Register(r, "/debug/pprof")
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
}

func ConfigRouter(r *gin.Engine, rts ...*router.Router) {

	if len(rts) > 0 {
		rt := rts[0]
		pagesPrefix := "/api/n9e/busi-group/:id"
		pages := r.Group(pagesPrefix)
		{
			pages.GET("/task/:id", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskGet)
			pages.PUT("/task/:id/action", rt.Auth(), rt.User(), rt.Perm("/job-tasks/put"), rt.Bgrw(), taskAction)
			pages.GET("/task/:id/stdout", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskStdout)
			pages.GET("/task/:id/stderr", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskStderr)
			pages.GET("/task/:id/state", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskState)
			pages.GET("/task/:id/result", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskResult)
			pages.PUT("/task/:id/host/:host/action", rt.Auth(), rt.User(), rt.Perm("/job-tasks/put"), rt.Bgrw(), taskHostAction)
			pages.GET("/task/:id/host/:host/output", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskHostOutput)
			pages.GET("/task/:id/host/:host/stdout", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskHostStdout)
			pages.GET("/task/:id/host/:host/stderr", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskHostStderr)
			pages.GET("/task/:id/stdout.txt", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskStdoutTxt)
			pages.GET("/task/:id/stderr.txt", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskStderrTxt)
			pages.GET("/task/:id/stdout.json", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskStdoutJSON)
			pages.GET("/task/:id/stderr.json", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), taskStderrJSON)
		}
	}

	api := r.Group("/ibex/v1")
	if len(config.C.BasicAuth) > 0 {
		api = r.Group("/ibex/v1", gin.BasicAuth(config.C.BasicAuth))
	}
	{
		api.POST("/tasks", taskAdd)
		api.GET("/tasks", taskGets)
		api.GET("/tasks/done-ids", doneIds)
		api.GET("/task/:id", taskGet)
		api.PUT("/task/:id/action", taskAction)
		api.GET("/task/:id/stdout", taskStdout)
		api.GET("/task/:id/stderr", taskStderr)
		api.GET("/task/:id/state", taskState)
		api.GET("/task/:id/result", taskResult)
		api.PUT("/task/:id/host/:host/action", taskHostAction)
		api.GET("/task/:id/host/:host/output", taskHostOutput)
		api.GET("/task/:id/host/:host/stdout", taskHostStdout)
		api.GET("/task/:id/host/:host/stderr", taskHostStderr)
		api.GET("/task/:id/stdout.txt", taskStdoutTxt)
		api.GET("/task/:id/stderr.txt", taskStderrTxt)
		api.GET("/task/:id/stdout.json", taskStdoutJSON)
		api.GET("/task/:id/stderr.json", taskStderrJSON)

		// api for edge server
		api.POST("/table/record/list", tableRecordListGet)
		api.POST("/table/record/count", tableRecordCount)
		api.POST("/mark/done", markDone)
		api.POST("/task/meta", taskMetaAdd)
		api.POST("/task/host/", taskHostAdd)
		api.POST("/task/hosts/upsert", taskHostUpsert)
	}
}
