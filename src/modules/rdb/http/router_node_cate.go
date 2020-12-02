package http

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/models"
)

func nodeCateGets(c *gin.Context) {
	objs, err := models.NodeCateGets()
	renderData(c, objs, err)
}

type nodeCatePostForm struct {
	Ident     string `json:"ident"`
	Name      string `json:"name"`
	IconColor string `json:"icon_color"`
}

func (f nodeCatePostForm) Validate() {
	if f.Ident == "" {
		bomb("[%s] is blank", "ident")
	}

	if f.Name == "" {
		bomb("[%s] is blank", "name")
	}

	if len(f.Ident) > 32 {
		bomb("arg[%s] too long > %d", "ident", 32)
	}

	if len(f.Name) > 255 {
		bomb("arg[%s] too long > %d", "name", 255)
	}

	if !str.IsMatch(f.Ident, "[a-z]+") {
		bomb("arg[%s] invalid", "ident")
	}

	if str.Dangerous(f.Name) {
		bomb("arg[name] dangerous")
	}
}

func nodeCatePost(c *gin.Context) {
	var f nodeCatePostForm
	bind(c, &f)
	f.Validate()

	obj := models.NodeCate{
		Ident:     f.Ident,
		Name:      f.Name,
		IconColor: f.IconColor,
	}

	renderMessage(c, models.NodeCateNew(&obj))
}

type nodeCatePutForm struct {
	Name      string `json:"name"`
	IconColor string `json:"icon_color"`
}

func (f nodeCatePutForm) Validate() {
	if len(f.Name) > 255 {
		bomb("arg[%s] too long > %d", "name", 255)
	}

	if str.Dangerous(f.Name) {
		bomb("arg[name] dangerous")
	}
}

func nodeCatePut(c *gin.Context) {
	var f nodeCatePutForm
	bind(c, &f)
	f.Validate()

	id := urlParamInt64(c, "id")

	nc, err := models.NodeCateGet("id=?", id)
	dangerous(err)

	if nc == nil {
		bomb("no such NodeCate[id:%d]", id)
	}

	nc.Name = f.Name

	if nc.IconColor != f.IconColor {
		nc.IconColor = f.IconColor
		dangerous(models.UpdateIconColor(f.IconColor, nc.Ident))
	}

	renderMessage(c, nc.Update("name", "icon_color"))
}

func nodeCateDel(c *gin.Context) {
	id := urlParamInt64(c, "id")

	nc, err := models.NodeCateGet("id=?", id)
	dangerous(err)

	if nc == nil {
		renderMessage(c, nil)
		return
	}

	renderMessage(c, nc.Del())
}
