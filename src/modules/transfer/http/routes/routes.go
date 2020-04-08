package routes

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

	v2 := r.Group("/api/transfer/v2")
	{
		v2.POST("/data", QueryData)
	}

	pprof.Register(r, "/api/transfer/debug/pprof")
}
