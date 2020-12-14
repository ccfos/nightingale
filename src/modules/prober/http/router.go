package http

import (
	"fmt"
	"os"

	"github.com/didi/nightingale/src/modules/prober/cache"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

func Config(r *gin.Engine) {

	notLogin := r.Group("/api/rdb")
	{
		notLogin.GET("/ping", ping)
		notLogin.GET("/pid", pid)
		notLogin.GET("/addr", addr)
		notLogin.GET("/collect-rule/:id", getCollectRule)
		// notLogin.POST("/data", getData)
	}

	pprof.Register(r, "/api/prober/debug/pprof")
}

func ping(c *gin.Context) {
	c.String(200, "pong")
}

func addr(c *gin.Context) {
	c.String(200, c.Request.RemoteAddr)
}

func pid(c *gin.Context) {
	c.String(200, fmt.Sprintf("%d", os.Getpid()))
}

func getCollectRule(c *gin.Context) {
	rule, _ := cache.CollectRule.Get(urlParamInt64(c, "id"))
	renderData(c, rule, nil)
}

/*
// TODO: get last collect data
func getData(c *gin.Context) {
	var input dataobj.JudgeItem
	errors.Dangerous(c.ShouldBind(&input))
	pk := input.MD5()
	linkedList, _ := cache.HistoryBigMap[pk[0:2]].Get(pk)
	data := linkedList.HistoryData()
	renderData(c, data, nil)
}
*/
