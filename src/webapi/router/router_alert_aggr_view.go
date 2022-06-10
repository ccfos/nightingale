package router

import (
	"net/http"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// no param
func alertAggrViewGets(c *gin.Context) {
	lst, err := models.AlertAggrViewGets(c.MustGet("userid"))
	ginx.NewRender(c).Data(lst, err)
}

// body: name, rule, cate
func alertAggrViewAdd(c *gin.Context) {
	var f models.AlertAggrView
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	if !me.IsAdmin() {
		// 管理员可以选择当前这个视图是公开呢，还是私有，普通用户的话就只能是私有的
		f.Cate = 1
	}

	f.Id = 0
	f.CreateBy = me.Id
	ginx.Dangerous(f.Add())

	ginx.NewRender(c).Data(f, nil)
}

// body: ids
func alertAggrViewDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	me := c.MustGet("user").(*models.User)
	if me.IsAdmin() {
		ginx.NewRender(c).Message(models.AlertAggrViewDel(f.Ids))
	} else {
		ginx.NewRender(c).Message(models.AlertAggrViewDel(f.Ids, me.Id))
	}
}

// body: id, name, rule, cate
func alertAggrViewPut(c *gin.Context) {
	var f models.AlertAggrView
	ginx.BindJSON(c, &f)

	view, err := models.AlertAggrViewGet("id = ?", f.Id)
	ginx.Dangerous(err)

	if view == nil {
		ginx.NewRender(c).Message("no such item(id: %d)", f.Id)
		return
	}

	me := c.MustGet("user").(*models.User)
	if !me.IsAdmin() {
		f.Cate = 1

		if view.CreateBy != me.Id {
			ginx.NewRender(c, http.StatusForbidden).Message("forbidden")
			return
		}
	}

	ginx.NewRender(c).Message(view.Update(f.Name, f.Rule, f.Cate))
}
