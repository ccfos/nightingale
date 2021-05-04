package http

import (
	"github.com/didi/nightingale/v4/src/models"

	"github.com/gin-gonic/gin"
)

func nodeCateFieldGets(c *gin.Context) {
	lst, err := models.NodeCateFieldGets("cate = ?", queryStr(c, "cate"))
	renderData(c, lst, err)
}

func nodeCateFieldGet(c *gin.Context) {
	obj, err := models.NodeCateFieldGet("id = ?", urlParamInt64(c, "id"))
	renderData(c, obj, err)
}

func nodeCateFieldNew(c *gin.Context) {
	var obj models.NodeCateField
	bind(c, &obj)
	renderMessage(c, models.NodeCateFieldNew(&obj))
}

func nodeCateFieldPut(c *gin.Context) {
	var f models.NodeCateField
	bind(c, &f)

	obj, err := models.NodeCateFieldGet("id = ?", urlParamInt64(c, "id"))
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

	renderMessage(c, obj.Update("field_name", "field_extra", "field_required"))
}

func nodeCateFieldDel(c *gin.Context) {
	obj, err := models.NodeCateFieldGet("id = ?", urlParamInt64(c, "id"))
	dangerous(err)

	if obj == nil {
		renderMessage(c, nil)
		return
	}

	renderMessage(c, obj.Del())
}
