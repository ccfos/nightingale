package routes

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

// Config routes
func Config(r *gin.Engine) {
	sys := r.Group("/api/index")
	{
		sys.GET("/ping", ping)
		sys.GET("/pid", pid)
		sys.GET("/addr", addr)
		sys.GET("/index-total", indexTotal)

		sys.POST("/metrics", GetMetrics)
		sys.DELETE("/metrics", DelMetrics)
		sys.DELETE("/endpoints", DelIdxByEndpoint)
		sys.DELETE("/counter", DelCounter)
		sys.POST("/tagkv", GetTagPairs)
		sys.POST("/counter/fullmatch", GetIndexByFullTags)
		sys.POST("/counter/clude", GetIndexByClude)
		sys.POST("/dump", DumpIndex)
		sys.GET("/idxfile", GetIdxFile)
	}

	pprof.Register(r, "/api/index/debug/pprof")
}
