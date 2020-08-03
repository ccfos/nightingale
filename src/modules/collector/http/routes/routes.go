package routes

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

// Config routes
func Config(r *gin.Engine) {
	sys := r.Group("/api/collector")
	{
		sys.GET("/ping", ping)
		sys.GET("/pid", pid)
		sys.GET("/addr", addr)

		sys.GET("/stra", getStrategy)
		sys.GET("/cached", getLogCached)
		sys.POST("/push", pushData)
	}

	// compatible with open-falcon
	v1 := r.Group("/v1")
	{
		v1.GET("/ping", ping)
		v1.GET("/pid", pid)
		v1.GET("/addr", addr)

		v1.GET("/stra", getStrategy)
		v1.GET("/cached", getLogCached)
		v1.POST("/push", pushData)
	}

	pprof.Register(r, "/api/collector/debug/pprof")
}
