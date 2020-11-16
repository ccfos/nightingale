package http

import (
	"fmt"
	"os"

	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/didi/nightingale/src/modules/transfer/cache"
	"github.com/didi/nightingale/src/modules/transfer/config"
	"github.com/didi/nightingale/src/toolkits/http/render"
	"github.com/didi/nightingale/src/toolkits/str"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
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
	key := str.MD5(input.Endpoint, input.Metric, "")
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

	dataSource, err := backend.GetDataSourceFor(config.Config.Backend.DataSource)
	if err != nil {
		logger.Warningf("could not find datasource")
		render.Message(c, err)
		return
	}

	addrs := dataSource.GetInstance(input.Metric, input.Endpoint, input.TagMap)
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
	key := str.MD5(input.Endpoint, input.Metric, "")
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
