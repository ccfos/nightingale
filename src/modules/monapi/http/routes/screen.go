package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/model"
)

type ScreenForm struct {
	Name string `json:"name"`
}

func screenNameValidate(name string) {
	if str.Dangerous(name) {
		errors.Bomb("arg[name] is dangerous")
	}

	if len(name) > 250 {
		errors.Bomb("arg[name] too long")
	}

	if len(name) == 0 {
		errors.Bomb("arg[name] is blank")
	}
}

func screenPost(c *gin.Context) {
	node := mustNode(urlParamInt64(c, "id"))

	var f ScreenForm
	errors.Dangerous(c.ShouldBind(&f))
	screenNameValidate(f.Name)

	screen := model.Screen{
		NodeId:      node.Id,
		Name:        f.Name,
		LastUpdator: loginUsername(c),
	}

	renderMessage(c, screen.Add())
}

func screenGets(c *gin.Context) {
	objs, err := model.ScreenGets(urlParamInt64(c, "id"))
	renderData(c, objs, err)
}

type ScreenPutForm struct {
	Name   string `json:"name"`
	NodeId int64  `json:"node_id"`
}

func screenPut(c *gin.Context) {
	screen := mustScreen(urlParamInt64(c, "id"))

	var f ScreenPutForm
	errors.Dangerous(c.ShouldBind(&f))
	screenNameValidate(f.Name)
	node := mustNode(f.NodeId)

	screen.Name = f.Name
	screen.NodeId = node.Id
	screen.LastUpdator = loginUsername(c)

	renderMessage(c, screen.Update("name", "node_id", "last_updator"))
}

func screenDel(c *gin.Context) {
	screen := mustScreen(urlParamInt64(c, "id"))

	errors.Dangerous(screen.Del())
	logger.Infof("[screen] %s delete %+v", loginUsername(c), screen)

	renderMessage(c, nil)
}

func screenSubclassGets(c *gin.Context) {
	objs, err := model.ScreenSubclassGets(urlParamInt64(c, "id"))
	renderData(c, objs, err)
}

type ScreenSubclassForm struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

func screenSubclassPost(c *gin.Context) {
	screen := mustScreen(urlParamInt64(c, "id"))

	var f ScreenSubclassForm
	errors.Dangerous(c.ShouldBind(&f))
	screenNameValidate(f.Name)

	subclass := model.ScreenSubclass{
		ScreenId: screen.Id,
		Name:     f.Name,
		Weight:   f.Weight,
	}

	renderMessage(c, subclass.Add())
}

func screenSubclassPut(c *gin.Context) {
	var arr []model.ScreenSubclass
	errors.Dangerous(c.ShouldBind(&arr))

	cnt := len(arr)
	for i := 0; i < cnt; i++ {
		errors.Dangerous(arr[i].Update("weight", "name"))
	}

	renderMessage(c, nil)
}

func screenSubclassLocPut(c *gin.Context) {
	var arr []model.ScreenSubclass
	errors.Dangerous(c.ShouldBind(&arr))

	cnt := len(arr)
	for i := 0; i < cnt; i++ {
		errors.Dangerous(arr[i].Update("screen_id"))
	}

	renderMessage(c, nil)
}

func screenSubclassDel(c *gin.Context) {
	subclass, err := model.ScreenSubclassGet("id", urlParamInt64(c, "id"))
	errors.Dangerous(err)

	if subclass == nil {
		renderMessage(c, nil)
		return
	}

	errors.Dangerous(subclass.Del())
	logger.Infof("[screen_sublcass] %s delete %+v", loginUsername(c), subclass)

	renderMessage(c, nil)
}
