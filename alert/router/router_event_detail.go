package router

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
)

func (rt *Router) eventDetail(c *gin.Context) {
	hash := ginx.UrlParamStr(c, "hash")
	if !loggrep.IsValidHash(hash) {
		ginx.Bomb(200, "invalid hash format")
	}

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)

	res := loggrep.GrepLogDir(c.Request.Context(), rt.LogDir, hash, logGrepOptions(c))

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:      res.Logs,
		Instance:  instance,
		Truncated: res.Truncated,
		Reason:    res.Reason,
	}, nil)
}
