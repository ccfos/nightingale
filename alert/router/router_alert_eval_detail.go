package router

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) alertEvalDetail(c *gin.Context) {
	id := ginx.UrlParamStr(c, "id")
	if !loggrep.IsValidRuleID(id) {
		ginx.Bomb(200, "invalid rule id format")
	}

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)

	keyword := fmt.Sprintf("alert_eval_%s", id)
	logs, err := loggrep.GrepLogDir(rt.LogDir, keyword)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:     logs,
		Instance: instance,
	}, nil)
}
