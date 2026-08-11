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
	res := loggrep.GrepLatestLogFiles(c.Request.Context(), rt.LogDir, keyword, logGrepOptions(c))

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:      res.Logs,
		Instance:  instance,
		Truncated: res.Truncated,
		Reason:    res.Reason,
	}, nil)
}
