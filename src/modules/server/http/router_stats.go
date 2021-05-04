package http

import (
	"github.com/didi/nightingale/v4/src/common/str"
	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/backend"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
	"github.com/didi/nightingale/v4/src/modules/server/config"
	"github.com/didi/nightingale/v4/src/modules/server/judge"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type rdbStats struct {
	Login *models.Stats
}

var (
	stats *rdbStats
)

func initStats() {
	stats = &rdbStats{
		Login: models.MustNewStats("login"),
	}
}

func counterGet(c *gin.Context) {
	renderData(c, map[string]int64{
		"login": stats.Login.Get(),
	}, nil)
}

type getStraReq struct {
	Endpoint string `json:"endpoint"`
	Metric   string `json:"metric"`
}

func getStra(c *gin.Context) {
	var input getStraReq
	errors.Dangerous(c.ShouldBindJSON(&input))
	key := str.ToMD5(input.Endpoint, input.Metric, "")
	stras := cache.StraMap.GetByKey(key)

	renderData(c, stras, nil)
}

type tsdbInstanceRecv struct {
	Endpoint string            `json:"endpoint"`
	Metric   string            `json:"metric"`
	TagMap   map[string]string `json:"tags"`
}

func tsdbInstance(c *gin.Context) {
	var input tsdbInstanceRecv
	errors.Dangerous(c.ShouldBindJSON(&input))

	dataSource, err := backend.GetDataSourceFor(config.Config.Transfer.Backend.DataSource)
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	addrs := dataSource.GetInstance(input.Metric, input.Endpoint, input.TagMap)
	renderData(c, addrs, nil)
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
	key := str.ToMD5(input.Endpoint, input.Metric, "")
	stras := cache.StraMap.GetByKey(key)
	for _, stra := range stras {
		if input.Sid != stra.Id || !judge.TagMatch(stra.Tags, input.TagMap) {
			continue
		}
		instance = stra.JudgeInstance
	}

	renderData(c, instance, nil)
}

func judges(c *gin.Context) {
	renderData(c, judge.GetJudges(), nil)
}
