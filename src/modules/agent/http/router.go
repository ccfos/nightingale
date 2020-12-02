package http

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

func Config(r *gin.Engine) {
	// executor apis
	r.GET("/output/:id/stdout.json", stdoutJSON)
	r.GET("/output/:id/stdout.txt", stdoutTxt)
	r.GET("/output/:id/stderr.json", stderrJSON)
	r.GET("/output/:id/stderr.txt", stderrTxt)

	// collector apis, compatible with open-falcon
	v1 := r.Group("/v1")
	{
		v1.GET("/ping", ping)
		v1.GET("/pid", pid)
		v1.GET("/addr", addr)

		v1.GET("/stra", getStrategy)
		v1.GET("/cached", getLogCached)
		v1.POST("/push", pushData)
	}

	col := r.Group("/api/collector")
	{
		col.GET("/stra", getStrategy)
		col.GET("/cached", getLogCached)
		col.POST("/push", pushData)
	}

	pprof.Register(r, "/debug/pprof")
}
