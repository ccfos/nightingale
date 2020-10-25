package http

import (
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/src/models"
)

func hostFieldNew(c *gin.Context) {
	loginUser(c).CheckPermGlobal("ams_host_field_mgr")

	var obj models.HostField
	bind(c, &obj)

	renderMessage(c, models.HostFieldNew(&obj))
}

func hostFieldsGets(c *gin.Context) {
	lst, err := models.HostFieldGets()
	renderData(c, lst, err)
}

func hostFieldGet(c *gin.Context) {
	obj, err := models.HostFieldGet("id = ?", urlParamInt64(c, "id"))
	renderData(c, obj, err)
}

func hostFieldPut(c *gin.Context) {
	loginUser(c).CheckPermGlobal("ams_host_field_mgr")

	var f models.HostField
	bind(c, &f)

	obj, err := models.HostFieldGet("id = ?", urlParamInt64(c, "id"))
	dangerous(err)

	if obj == nil {
		bomb("no such field")
	}

	if obj.FieldType != f.FieldType {
		bomb("field_type cannot modify")
	}

	obj.FieldName = f.FieldName
	obj.FieldExtra = f.FieldExtra
	obj.FieldRequired = f.FieldRequired
	obj.FieldCate = f.FieldCate

	renderMessage(c, obj.Update("field_name", "field_extra", "field_required", "field_cate"))
}

func hostFieldDel(c *gin.Context) {
	loginUser(c).CheckPermGlobal("ams_host_field_mgr")

	obj, err := models.HostFieldGet("id = ?", urlParamInt64(c, "id"))
	dangerous(err)

	if obj == nil {
		renderMessage(c, nil)
		return
	}

	renderMessage(c, obj.Del())
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
