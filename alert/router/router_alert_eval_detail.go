package router

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
)

func (rt *Router) alertEvalDetail(c *gin.Context) {
	id := ginx.UrlParamStr(c, "id")
	if !loggrep.IsValidRuleID(id) {
		ginx.Bomb(200, "invalid rule id format")
	}

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)

	keyword := fmt.Sprintf("alert_eval_%s", id)
	res := loggrep.GrepLogDir(c.Request.Context(), rt.LogDir, keyword, logGrepOptions(c))

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:      res.Logs,
		Instance:  instance,
		Truncated: res.Truncated,
		Reason:    res.Reason,
	}, nil)
}
