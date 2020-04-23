package routes

import (
	"fmt"
	"os"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/didi/nightingale/src/modules/transfer/cache"
	"github.com/didi/nightingale/src/toolkits/http/render"
	"github.com/didi/nightingale/src/toolkits/str"

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

type getStraReq struct {
	Endpoint string `json:"endpoint"`
	Metric   string `json:"metric"`
}

func getStra(c *gin.Context) {
	var input getStraReq
	errors.Dangerous(c.ShouldBindJSON(&input))
	key := str.PK(input.Metric, input.Endpoint)
	stras := cache.StraMap.GetByKey(key)

	render.Data(c, stras, nil)
}

type tsdbInstanceRecv struct {
	Endpoint string            `json:"endpoint"`
	Metric   string            `json:"metric"`
	TagMap   map[string]string `json:"tags"`
}

func tsdbInstance(c *gin.Context) {
	var input tsdbInstanceRecv
	errors.Dangerous(c.ShouldBindJSON(&input))

	counter, err := backend.GetCounter(input.Metric, "", input.TagMap)
	errors.Dangerous(err)

	pk := dataobj.PKWithCounter(input.Endpoint, counter)
	pools, err := backend.SelectPoolByPK(pk)
	addrs := make([]string, len(pools))
	for i, pool := range pools {
		addrs[i] = pool.Addr
	}

	render.Data(c, addrs, nil)
}

type judgeInstanceRecv struct {
	Endpoint string            `json:"endpoint"`
	Metric   string            `json:"metric"`
	TagMap   map[string]string `json:"tags"`
	Step     int               `json:"step"`
	Sid      int64             `json:"sid"`
}

func judgeInstance(c *gin.Context) {
	var input judgeInstanceRecv
	errors.Dangerous(c.ShouldBindJSON(&input))
	var instance string
	key := str.PK(input.Metric, input.Endpoint)
	stras := cache.StraMap.GetByKey(key)
	for _, stra := range stras {
		if input.Sid != stra.Id || !backend.TagMatch(stra.Tags, input.TagMap) {
			continue
		}
		instance = stra.JudgeInstance
	}

	render.Data(c, instance, nil)
}

func judges(c *gin.Context) {
	render.Data(c, backend.GetJudges(), nil)
}
