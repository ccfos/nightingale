package router

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/webapi/config"
)

func notifyChannelsGets(c *gin.Context) {
	ginx.NewRender(c).Data(config.C.NotifyChannels, nil)
}

func contactKeysGets(c *gin.Context) {
	ginx.NewRender(c).Data(config.C.ContactKeys, nil)
}
