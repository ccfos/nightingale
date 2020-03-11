package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/model"
)

func teamListGet(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	total, err := model.TeamTotal(query)
	errors.Dangerous(err)

	list, err := model.TeamGets(query, limit, offset(c, limit, total))
	errors.Dangerous(err)

	for i := 0; i < len(list); i++ {
		errors.Dangerous(list[i].FillObjs())
	}

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type teamForm struct {
	Ident   string  `json:"ident"`
	Name    string  `json:"name"`
	Mgmt    int     `json:"mgmt"`
	Admins  []int64 `json:"admins"`
	Members []int64 `json:"members"`
}

func teamAddPost(c *gin.Context) {
	var f teamForm
	errors.Dangerous(c.ShouldBind(&f))

	renderMessage(c, model.TeamAdd(f.Ident, f.Name, f.Mgmt, f.Admins, f.Members))
}

func teamPut(c *gin.Context) {
	me := loginUser(c)

	var f teamForm
	errors.Dangerous(c.ShouldBind(&f))

	t, err := model.TeamGet("id", urlParamInt64(c, "id"))
	errors.Dangerous(err)

	if t == nil {
		errors.Bomb("no such team")
	}

	can, err := me.CanModifyTeam(t)
	errors.Dangerous(err)
	if !can {
		errors.Bomb("no privilege")
	}

	renderMessage(c, t.Modify(f.Ident, f.Name, f.Mgmt, f.Admins, f.Members))
}

func teamDel(c *gin.Context) {
	me := loginUser(c)

	t, err := model.TeamGet("id", urlParamInt64(c, "id"))
	errors.Dangerous(err)

	if t == nil {
		renderMessage(c, nil)
		return
	}

	can, err := me.CanModifyTeam(t)
	errors.Dangerous(err)
	if !can {
		errors.Bomb("no privilege")
	}

	renderMessage(c, t.Del())
}
