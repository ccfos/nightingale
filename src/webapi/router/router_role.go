package router

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/models"
)

func rolesGets(c *gin.Context) {
	lst, err := models.RoleGetsAll()
	ginx.NewRender(c).Data(lst, err)
}

func permsGets(c *gin.Context) {
	user := c.MustGet("user").(*models.User)
	lst, err := models.OperationsOfRole(strings.Fields(user.Roles))
	ginx.NewRender(c).Data(lst, err)
}
