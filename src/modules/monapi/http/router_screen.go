package http

import (
	"strings"

	"github.com/didi/nightingale/src/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type ScreenForm struct {
	Name string `json:"name"`
}

func screenNameValidate(name string) {
	if str.Dangerous(name) {
		bomb("arg[name] is dangerous")
	}

	if len(name) > 250 {
		bomb("arg[name] too long")
	}

	if len(name) == 0 {
		bomb("arg[name] is blank")
	}

	if strings.ContainsAny(name, "/%") {
		bomb("arg[name] invalid")
	}
}

func screenPost(c *gin.Context) {
	username := loginUsername(c)
	node := mustNode(urlParamInt64(c, "id"))

	var f ScreenForm
	errors.Dangerous(c.ShouldBind(&f))
	can, err := models.UsernameCandoNodeOp(username, "mon_screen_create", node.Id)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	screenNameValidate(f.Name)

	screen := models.Screen{
		NodeId:      node.Id,
		Name:        f.Name,
		LastUpdator: loginUsername(c),
	}

	err = screen.Add()
	renderData(c, screen.Id, err)
}

func screenGets(c *gin.Context) {
	username := loginUsername(c)

	nid := urlParamInt64(c, "id")
	can, err := models.UsernameCandoNodeOp(username, "mon_screen_view", nid)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	objs, err := models.ScreenGets(nid)
	renderData(c, objs, err)
}

func screenGet(c *gin.Context) {
	username := loginUsername(c)
	obj, err := models.ScreenGet("id", urlParamInt64(c, "id"))
	node := mustNode(obj.NodeId)

	can, err := models.UsernameCandoNodeOp(username, "mon_screen_view", obj.NodeId)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	obj.NodePath = node.Path
	renderData(c, obj, err)
}

type ScreenPutForm struct {
	Name   string `json:"name"`
	NodeId int64  `json:"node_id"`
}

func screenPut(c *gin.Context) {
	username := loginUsername(c)
	screen := mustScreen(urlParamInt64(c, "id"))

	var f ScreenPutForm
	errors.Dangerous(c.ShouldBind(&f))
	screenNameValidate(f.Name)

	can, err := models.UsernameCandoNodeOp(username, "mon_screen_modify", screen.NodeId)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	node := mustNode(f.NodeId)

	screen.Name = f.Name
	screen.NodeId = node.Id
	screen.LastUpdator = username

	renderMessage(c, screen.Update("name", "node_id", "last_updator"))
}

func screenDel(c *gin.Context) {
	username := loginUsername(c)
	screen := mustScreen(urlParamInt64(c, "id"))

	can, err := models.UsernameCandoNodeOp(username, "mon_screen_delete", screen.NodeId)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	errors.Dangerous(screen.Del())
	logger.Infof("[screen] %s delete %+v", username, screen)

	renderMessage(c, nil)
}

func screenSubclassGets(c *gin.Context) {
	objs, err := models.ScreenSubclassGets(urlParamInt64(c, "id"))
	renderData(c, objs, err)
}

type ScreenSubclassForm struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

func (f ScreenSubclassForm) Validate() {
	if str.Dangerous(f.Name) {
		bomb("arg[name] invalid")
	}

	if strings.ContainsAny(f.Name, "/%") {
		bomb("arg[name] invalid")
	}
}

func screenSubclassPost(c *gin.Context) {
	screen := mustScreen(urlParamInt64(c, "id"))

	var f ScreenSubclassForm
	errors.Dangerous(c.ShouldBind(&f))
	f.Validate()

	can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_screen_create", screen.NodeId)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	screenNameValidate(f.Name)

	subclass := models.ScreenSubclass{
		ScreenId: screen.Id,
		Name:     f.Name,
		Weight:   f.Weight,
	}

	err = subclass.Add()
	renderData(c, subclass.Id, err)
}

func screenSubclassPut(c *gin.Context) {
	username := loginUsername(c)

	var arr []models.ScreenSubclass
	errors.Dangerous(c.ShouldBind(&arr))

	cnt := len(arr)
	//校验权限
	for i := 0; i < cnt; i++ {
		screen := mustScreen(arr[i].ScreenId)
		can, err := models.UsernameCandoNodeOp(username, "mon_screen_modify", screen.NodeId)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}
	}

	for i := 0; i < cnt; i++ {
		errors.Dangerous(arr[i].Update("weight", "name"))
	}

	renderMessage(c, nil)
}

func screenSubclassLocPut(c *gin.Context) {
	username := loginUsername(c)

	var arr []models.ScreenSubclass
	errors.Dangerous(c.ShouldBind(&arr))

	cnt := len(arr)
	//校验权限
	for i := 0; i < cnt; i++ {
		screen := mustScreen(arr[i].ScreenId)

		can, err := models.UsernameCandoNodeOp(username, "mon_screen_modify", screen.NodeId)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}
	}

	for i := 0; i < cnt; i++ {
		errors.Dangerous(arr[i].Update("screen_id"))
	}

	renderMessage(c, nil)
}

func screenSubclassDel(c *gin.Context) {
	subclass, err := models.ScreenSubclassGet("id", urlParamInt64(c, "id"))
	errors.Dangerous(err)

	screen := mustScreen(subclass.ScreenId)
	can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_screen_delete", screen.NodeId)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	if subclass == nil {
		renderMessage(c, nil)
		return
	}

	errors.Dangerous(subclass.Del())
	logger.Infof("[screen_sublcass] %s delete %+v", loginUsername(c), subclass)

	renderMessage(c, nil)
}
