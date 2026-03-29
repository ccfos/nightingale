package router

import (
	"fmt"
	"net/http"

	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"

	"github.com/gin-gonic/gin"
)

func (rt *Router) eventDetail(c *gin.Context) {
	hash := ginx.UrlParamStr(c, "hash")
	if !loggrep.IsValidHash(hash) {
		ginx.Bomb(http.StatusBadRequest, "invalid hash format")
	}

	instance := fmt.Sprintf("%s:%d", rt.Alert.Heartbeat.IP, rt.HTTP.Port)

	logs, err := loggrep.GrepLogDir(rt.LogDir, hash)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(loggrep.EventDetailResp{
		Logs:     logs,
		Instance: instance,
	}, nil)
}
