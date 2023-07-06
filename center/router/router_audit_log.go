package router

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) auditLogGets(c *gin.Context) {
	limit := ginx.QueryInt(c, "limit", 20)
	username := ginx.QueryStr(c, "username", "")
	event := ginx.QueryStr(c, "event", "")
	startTime := ginx.QueryInt64(c, "start_time")
	endTime := ginx.QueryInt64(c, "end_time")

	total, err := models.AuditLogTotal(rt.Ctx, username, event, startTime, endTime)
	ginx.Dangerous(err)

	list, err := models.AuditLogGets(rt.Ctx, username, event, startTime, endTime, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func (rt *Router) auditEventGets(c *gin.Context) {
	ginx.NewRender(c).Data(models.AuditEventDesc, nil)
}
