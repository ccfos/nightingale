package http

import (
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/models"
)

func rolesGet(c *gin.Context) {
	lst, err := models.RoleGetsAll()
	renderData(c, lst, err)
}
