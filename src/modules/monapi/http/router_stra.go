package http

import (
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/scache"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func straPost(c *gin.Context) {
	username := loginUsername(c)
	stra := new(models.Stra)
	errors.Dangerous(c.ShouldBindJSON(stra))

	can, err := models.UsernameCandoNodeOp(username, "mon_stra_create", stra.Nid)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	stra.Creator = username
	stra.LastUpdator = username

	errors.Dangerous(stra.Encode())

	old, err := models.StraFindOne("nid=? and name=?", stra.Nid, stra.Name)
	dangerous(err)

	if old != nil {
		bomb("同节点下策略名称 %s 已存在", stra.Name)
	}

	errors.Dangerous(stra.Save())

	type Id struct {
		Id int64 `json:"id"`
	}
	id := Id{Id: stra.Id}

	renderData(c, id, nil)
}

func straPut(c *gin.Context) {
	username := loginUsername(c)

	stra := new(models.Stra)
	errors.Dangerous(c.ShouldBind(stra))

	can, err := models.UsernameCandoNodeOp(username, "mon_stra_modify", stra.Nid)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	stra.LastUpdator = username
	errors.Dangerous(stra.Encode())

	old, err := models.StraFindOne("nid=? and name=? and id <> ?", stra.Nid, stra.Name, stra.Id)
	dangerous(err)

	if old != nil {
		bomb("同节点下策略名称 %s 已存在", stra.Name)
	}

	s, err := models.StraGet("id", stra.Id)
	errors.Dangerous(err)
	stra.Creator = s.Creator

	errors.Dangerous(stra.Update())

	renderData(c, "ok", nil)
}

type StrasDelRev struct {
	Ids []int64 `json:"ids"`
}

func strasDel(c *gin.Context) {
	username := loginUsername(c)
	var rev StrasDelRev
	errors.Dangerous(c.ShouldBind(&rev))

	for _, id := range rev.Ids {
		stra, err := models.StraGet("id", id)
		errors.Dangerous(err)
		can, err := models.UsernameCandoNodeOp(username, "mon_stra_delete", stra.Nid)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}
	}

	for i := 0; i < len(rev.Ids); i++ {
		errors.Dangerous(models.StraDel(rev.Ids[i]))
	}

	renderData(c, "ok", nil)
}

func straGet(c *gin.Context) {
	sid := urlParamInt64(c, "sid")

	stra, err := models.StraGet("id", sid)
	errors.Dangerous(err)
	if stra == nil {
		bomb("stra not found")
	}

	err = stra.Decode()
	errors.Dangerous(err)

	renderData(c, stra, nil)
}

func strasGet(c *gin.Context) {
	name := queryStr(c, "name", "")
	priority := queryInt(c, "priority", 4)
	nid := queryInt64(c, "nid", 0)
	list, err := models.StrasList(name, priority, nid)
	renderData(c, list, err)
}

func strasAll(c *gin.Context) {
	list, err := models.StrasAll()
	renderData(c, list, err)
}

func effectiveStrasGet(c *gin.Context) {
	stras := []*models.Stra{}
	instance := queryStr(c, "instance", "")

	if queryInt(c, "all", 0) == 1 {
		stras = scache.StraCache.GetAll()
	} else if instance != "" {
		node, err := scache.ActiveJudgeNode.GetNodeBy(instance)
		errors.Dangerous(err)

		stras = scache.StraCache.GetByNode(node)
	}
	renderData(c, stras, nil)
}
