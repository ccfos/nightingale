package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

func getTimeRange(c *gin.Context) (stime, etime int64) {
	stime = ginx.QueryInt64(c, "stime", 0)
	etime = ginx.QueryInt64(c, "etime", 0)
	hours := ginx.QueryInt64(c, "hours", 0)
	now := time.Now().Unix()
	if hours != 0 {
		stime = now - 3600*hours
		etime = now + 3600*24
	}

	if stime != 0 && etime == 0 {
		etime = now + 3600*24
	}
	return
}

func alertHisEventsList(c *gin.Context) {
	stime, etime := getTimeRange(c)

	severity := ginx.QueryInt(c, "severity", -1)
	recovered := ginx.QueryInt(c, "is_recovered", -1)
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)
	busiGroupId := ginx.QueryInt64(c, "bgid", 0)
	clusters := queryClusters(c)

	total, err := models.AlertHisEventTotal(busiGroupId, stime, etime, severity, recovered, clusters, query)
	ginx.Dangerous(err)

	list, err := models.AlertHisEventGets(busiGroupId, stime, etime, severity, recovered, clusters, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	cache := make(map[int64]*models.UserGroup)
	for i := 0; i < len(list); i++ {
		list[i].FillNotifyGroups(cache)
	}

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func alertHisEventGets(c *gin.Context) {
	stime, etime := getTimeRange(c)

	severity := ginx.QueryInt(c, "severity", -1)
	recovered := ginx.QueryInt(c, "is_recovered", -1)
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)
	busiGroupId := ginx.UrlParamInt64(c, "id")
	clusters := queryClusters(c)

	total, err := models.AlertHisEventTotal(busiGroupId, stime, etime, severity, recovered, clusters, query)
	ginx.Dangerous(err)

	list, err := models.AlertHisEventGets(busiGroupId, stime, etime, severity, recovered, clusters, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	cache := make(map[int64]*models.UserGroup)
	for i := 0; i < len(list); i++ {
		list[i].FillNotifyGroups(cache)
	}

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func alertHisEventGet(c *gin.Context) {
	eid := ginx.UrlParamInt64(c, "eid")
	event, err := models.AlertHisEventGetById(eid)
	ginx.Dangerous(err)

	if event == nil {
		ginx.Bomb(404, "No such alert event")
	}

	ginx.NewRender(c).Data(event, err)
}
