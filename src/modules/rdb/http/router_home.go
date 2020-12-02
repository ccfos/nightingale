package http

import (
	"github.com/didi/nightingale/src/modules/rdb/config"
	"github.com/gin-gonic/gin"
)

func ldapUsed(c *gin.Context) {
	renderData(c, config.Config.LDAP.DefaultUse, nil)
}
