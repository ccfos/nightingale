package router

import (
	"net/http"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func alertAggrViewGets(c *gin.Context) {
	lst, err := models.AlertAggrViewGets(c.MustGet("userid"))
	ginx.NewRender(c).Data(lst, err)
}

// name and rule is necessary
func alertAggrViewAdd(c *gin.Context) {
	var f models.AlertAggrView
	ginx.BindJSON(c, &f)

	f.Id = 0
	f.CreateBy = c.MustGet("username").(string)
	f.UserId = c.MustGet("userid").(int64)

	ginx.NewRender(c).Message(f.Add())
}

func alertAggrViewDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(models.AlertAggrViewDel(f.Ids, c.MustGet("userid")))
}

// id / name / rule is necessary
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
	if view.UserId != userid {
		ginx.NewRender(c, http.StatusForbidden).Message("forbidden")
		return
	}

	ginx.NewRender(c).Message(view.Update(f.Name, f.Rule))
}
