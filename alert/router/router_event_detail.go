package router

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) eventDetail(c *gin.Context) {
	hash := ginx.UrlParamStr(c, "hash")
	if !loggrep.IsValidHash(hash) {
		ginx.Bomb(200, "invalid hash format")
	}

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)

	logs, err := loggrep.GrepLogDir(rt.LogDir, hash)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:     logs,
		Instance: instance,
	}, nil)
}
