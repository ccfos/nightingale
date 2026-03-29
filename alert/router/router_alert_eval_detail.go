package router

import (
	"fmt"
	"net/http"

	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
)

func (rt *Router) alertEvalDetail(c *gin.Context) {
	id := ginx.UrlParamStr(c, "id")
	if !loggrep.IsValidRuleID(id) {
		ginx.Bomb(http.StatusBadRequest, "invalid rule id format")
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
