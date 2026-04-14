package router

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
)

func (rt *Router) traceLogs(c *gin.Context) {
	traceId := ginx.UrlParamStr(c, "traceid")
	if !loggrep.IsValidTraceID(traceId) {
		ginx.Bomb(200, "invalid trace id format")
	}

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)

	keyword := "trace_id=" + traceId
	logs, err := loggrep.GrepLatestLogFiles(rt.LogDir, keyword)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:     logs,
		Instance: instance,
	}, nil)
}
