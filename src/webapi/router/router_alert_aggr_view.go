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

// body: name, rule
func alertAggrViewAdd(c *gin.Context) {
	var f models.AlertAggrView
	ginx.BindJSON(c, &f)

	f.Id = 0
	f.CreateBy = c.MustGet("userid").(int64)
	ginx.Dangerous(f.Add())

	ginx.NewRender(c).Data(f, nil)
}

// body: ids
func alertAggrViewDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(models.AlertAggrViewDel(f.Ids, c.MustGet("userid")))
}

// body: id, name, rule
func alertAggrViewPut(c *gin.Context) {
	var f models.AlertAggrView
	ginx.BindJSON(c, &f)

	view, err := models.AlertAggrViewGet("id = ?", f.Id)
	ginx.Dangerous(err)

	if view == nil {
		ginx.NewRender(c).Message("no such item(id: %d)", f.Id)
		return
	}

	userid := c.MustGet("userid").(int64)
	if view.CreateBy != userid {
		ginx.NewRender(c, http.StatusForbidden).Message("forbidden")
		return
	}

	ginx.NewRender(c).Message(view.Update(f.Name, f.Rule))
}
