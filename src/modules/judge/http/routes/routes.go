package routes

import (
	"strconv"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

// Config routes
func Config(r *gin.Engine) {
	sys := r.Group("/api/judge")
	{
		sys.GET("/ping", ping)
		sys.GET("/pid", pid)
		sys.GET("/addr", addr)
		sys.GET("/stra/:id", getStra)
		sys.POST("/data", getData)
	}

	pprof.Register(r, "/api/judge/debug/pprof")
}

func urlParamStr(c *gin.Context, field string) string {
	val := c.Param(field)

	if val == "" {
		errors.Bomb("[%s] is blank", field)
	}

	return val
}

func urlParamInt64(c *gin.Context, field string) int64 {
	strval := urlParamStr(c, field)
	intval, err := strconv.ParseInt(strval, 10, 64)
	if err != nil {
		errors.Bomb("cannot convert %s to int64", strval)
	}

	return intval
}
