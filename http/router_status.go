package http

import (
	"time"

	"github.com/didi/nightingale/v5/models"
	"github.com/gin-gonic/gin"
)

func Status(c *gin.Context) {
	var err error
	data := make(map[string]int64)
	data["user_total"], err = models.UserTotal("")
	dangerous(err)

	data["user_group_total"], err = models.UserGroupTotal("")
	dangerous(err)

	data["resource_total"], err = models.ResourceTotal("")
	dangerous(err)

	data["alert_rule_total"], err = models.AlertRuleTotal("")
	dangerous(err)

	data["dashboard_total"], err = models.DashboardCount("")
	dangerous(err)

	now := time.Now().Unix()
	stime := now - 24*3600
	data["event_total_day"], err = models.AlertEventTotal(stime, now, "", -1, -1)
	dangerous(err)

	stime = now - 7*24*3600
	data["event_total_week"], err = models.AlertEventTotal(stime, now, "", -1, -1)
	dangerous(err)

	stime = now - 30*24*3600
	data["event_total_month"], err = models.AlertEventTotal(stime, now, "", -1, -1)
	dangerous(err)

	renderData(c, data, nil)
}
