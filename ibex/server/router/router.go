package router

import (
	"fmt"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"os"
	"strings"

	"github.com/ccfos/nightingale/v6/ibex/pkg/aop"
	"github.com/ccfos/nightingale/v6/ibex/server/config"

	"github.com/ccfos/nightingale/v6/center/router"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

func New(ctx *ctx.Context, version string) *gin.Engine {
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

	rou := NewRouter(ctx)

	rou.configBaseRouter(r, version)
	rou.ConfigRouter(r)

	return r
}

type Router struct {
	ctx *ctx.Context
}

func NewRouter(ctx *ctx.Context) *Router {
	return &Router{
		ctx: ctx,
	}
}

func (rou *Router) configBaseRouter(r *gin.Engine, version string) {
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

func (rou *Router) ConfigRouter(r *gin.Engine, rts ...*router.Router) {

	if len(rts) > 0 {
		rt := rts[0]
		pagesPrefix := "/api/n9e/busi-group/:id"
		pages := r.Group(pagesPrefix)
		{
			pages.GET("/task/:id", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskGet)
			pages.PUT("/task/:id/action", rt.Auth(), rt.User(), rt.Perm("/job-tasks/put"), rt.Bgrw(), rou.taskAction)
			pages.GET("/task/:id/stdout", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskStdout)
			pages.GET("/task/:id/stderr", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskStderr)
			pages.GET("/task/:id/state", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskState)
			pages.GET("/task/:id/result", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskResult)
			pages.PUT("/task/:id/host/:host/action", rt.Auth(), rt.User(), rt.Perm("/job-tasks/put"), rt.Bgrw(), rou.taskHostAction)
			pages.GET("/task/:id/host/:host/output", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskHostOutput)
			pages.GET("/task/:id/host/:host/stdout", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskHostStdout)
			pages.GET("/task/:id/host/:host/stderr", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskHostStderr)
			pages.GET("/task/:id/stdout.txt", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskStdoutTxt)
			pages.GET("/task/:id/stderr.txt", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskStderrTxt)
			pages.GET("/task/:id/stdout.json", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskStdoutJSON)
			pages.GET("/task/:id/stderr.json", rt.Auth(), rt.User(), rt.Perm("/job-tasks"), rou.taskStderrJSON)
		}
	}

	api := r.Group("/ibex/v1")
	if len(config.C.BasicAuth) > 0 {
		api = r.Group("/ibex/v1", gin.BasicAuth(config.C.BasicAuth))
	}
	{
		api.POST("/tasks", rou.taskAdd)
		api.GET("/tasks", rou.taskGets)
		api.GET("/tasks/done-ids", rou.doneIds)
		api.GET("/task/:id", rou.taskGet)
		api.PUT("/task/:id/action", rou.taskAction)
		api.GET("/task/:id/stdout", rou.taskStdout)
		api.GET("/task/:id/stderr", rou.taskStderr)
		api.GET("/task/:id/state", rou.taskState)
		api.GET("/task/:id/result", rou.taskResult)
		api.PUT("/task/:id/host/:host/action", rou.taskHostAction)
		api.GET("/task/:id/host/:host/output", rou.taskHostOutput)
		api.GET("/task/:id/host/:host/stdout", rou.taskHostStdout)
		api.GET("/task/:id/host/:host/stderr", rou.taskHostStderr)
		api.GET("/task/:id/stdout.txt", rou.taskStdoutTxt)
		api.GET("/task/:id/stderr.txt", rou.taskStderrTxt)
		api.GET("/task/:id/stdout.json", rou.taskStdoutJSON)
		api.GET("/task/:id/stderr.json", rou.taskStderrJSON)

		// api for edge server
		api.POST("/table/record/list", rou.tableRecordListGet)
		api.POST("/table/record/count", rou.tableRecordCount)
		api.POST("/mark/done", rou.markDone)
		api.POST("/task/meta", rou.taskMetaAdd)
		api.POST("/task/host/", rou.taskHostAdd)
		api.POST("/task/hosts/upsert", rou.taskHostUpsert)
	}
}
