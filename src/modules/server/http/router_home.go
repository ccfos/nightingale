package http

import (
	"github.com/didi/nightingale/v4/src/modules/server/config"

	"github.com/gin-gonic/gin"
)

func ldapUsed(c *gin.Context) {
	renderData(c, config.Config.Rdb.LDAP.DefaultUse, nil)
}
