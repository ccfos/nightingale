package http

import (
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

// Config routes
func Config(r *gin.Engine) {
	sys := r.Group("/api/transfer")
	{
		sys.GET("/ping", ping)
		sys.GET("/pid", pid)
		sys.GET("/addr", addr)
		sys.POST("/stra", getStra)
		sys.POST("/which-tsdb", tsdbInstance)
		sys.POST("/which-judge", judgeInstance)
		sys.GET("/alive-judges", judges)

		sys.POST("/push", PushData)
		sys.POST("/data", QueryData)
		sys.POST("/data/ui", QueryDataForUI)
	}

	index := r.Group("/api/index")
	{
		index.POST("/metrics", GetMetrics)
		index.POST("/tagkv", GetTagPairs)
		index.POST("/counter/clude", GetIndexByClude)
		index.POST("/counter/fullmatch", GetIndexByFullTags)
	}

	pprof.Register(r, "/api/transfer/debug/pprof")
}
