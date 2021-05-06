package http

import (
	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func getStraInJudge(c *gin.Context) {
	sid := urlParamInt64(c, "id")

	stra, exists := cache.Strategy.Get(sid)
	if exists {
		renderData(c, stra, nil)
		return
	}

	stra, _ = cache.NodataStra.Get(sid)
	renderData(c, stra, nil)
}

func getData(c *gin.Context) {
	var input dataobj.JudgeItem
	errors.Dangerous(c.ShouldBind(&input))
	pk := input.MD5()
	linkedList, _ := cache.HistoryBigMap[pk[0:2]].Get(pk)
	data := linkedList.HistoryData()
	renderData(c, data, nil)
}
