package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

func alertCurEventGets(c *gin.Context) {
	stime := ginx.QueryInt64(c, "stime", 0)
	etime := ginx.QueryInt64(c, "etime", 0)
	hours := ginx.QueryInt64(c, "hours", 0)
	now := time.Now().Unix()
	if hours != 0 {
		stime = now - 3600*hours
		etime = now + 3600*24
	}

	if stime != 0 && etime == 0 {
		etime = now + 3600*24
	}

	severity := ginx.QueryInt(c, "severity", -1)
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)
	busiGroupId := ginx.UrlParamInt64(c, "id")
	clusters := queryClusters(c)

	total, err := models.AlertCurEventTotal(busiGroupId, stime, etime, severity, clusters, query)
	ginx.Dangerous(err)

	list, err := models.AlertCurEventGets(busiGroupId, stime, etime, severity, clusters, query, limit, ginx.Offset(c, limit))
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

func alertCurEventDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	ginx.NewRender(c).Message(models.AlertCurEventDel(f.Ids))
}

func alertCurEventGet(c *gin.Context) {
	eid := ginx.UrlParamInt64(c, "eid")
	event, err := models.AlertCurEventGetById(eid)
	ginx.Dangerous(err)

	if event == nil {
		ginx.Bomb(404, "No such active event")
	}

	ginx.NewRender(c).Data(event, nil)
}
