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

func allPerms(c *gin.Context) {
	roles, err := models.RoleGetsAll()
	ginx.Dangerous(err)
	m := make(map[string][]string)
	for _, r := range roles {
		lst, err := models.OperationsOfRole(strings.Fields(r.Name))
		if err != nil {
			continue
		}
		m[r.Name] = lst
	}

	ginx.NewRender(c).Data(m, err)
}
