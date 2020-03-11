package routes

import (
	"fmt"
	"os"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/judge/cache"
	"github.com/didi/nightingale/src/toolkits/http/render"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func ping(c *gin.Context) {
	c.String(200, "pong")
}

func addr(c *gin.Context) {
	c.String(200, c.Request.RemoteAddr)
}

func pid(c *gin.Context) {
	c.String(200, fmt.Sprintf("%d", os.Getpid()))
}

func getStra(c *gin.Context) {
	sid := urlParamInt64(c, "id")

	stra, exists := cache.Strategy.Get(sid)
	if exists {
		render.Data(c, stra, nil)
		return
	}

	stra, _ = cache.NodataStra.Get(sid)
	render.Data(c, stra, nil)
}

func getData(c *gin.Context) {
	var input dataobj.JudgeItem
	errors.Dangerous(c.ShouldBind(&input))
	pk := input.MD5()
	linkedList, _ := cache.HistoryBigMap[pk[0:2]].Get(pk)
	data, _ := linkedList.HistoryData(10)
	render.Data(c, data, nil)
}
