package http

import (
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/src/models"
)

func hostFieldsGets(c *gin.Context) {
	lst, err := models.HostFieldGets()
	renderData(c, lst, err)
}

func hostFieldGet(c *gin.Context) {
	obj, err := models.HostFieldGet("id = ?", urlParamInt64(c, "id"))
	renderData(c, obj, err)
}

func hostFieldGets(c *gin.Context) {
	lst, err := models.HostFieldValueGets(urlParamInt64(c, "id"))
	renderData(c, lst, err)
}

func hostFieldPuts(c *gin.Context) {
	var objs []models.HostFieldValue
	bind(c, &objs)

	loginUser(c).CheckPermGlobal("ams_host_modify")

	renderMessage(c, models.HostFieldValuePuts(urlParamInt64(c, "id"), objs))
}
