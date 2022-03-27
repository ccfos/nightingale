package router

import (
	"net/http"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// no param
func metricViewGets(c *gin.Context) {
	lst, err := models.MetricViewGets(c.MustGet("userid"))
	ginx.NewRender(c).Data(lst, err)
}

// body: name, configs
func metricViewAdd(c *gin.Context) {
	var f models.MetricView
	ginx.BindJSON(c, &f)

	f.Id = 0
	f.CreateBy = c.MustGet("userid").(int64)

	ginx.NewRender(c).Message(f.Add())
}

// body: ids
func metricViewDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(models.MetricViewDel(f.Ids, c.MustGet("userid")))
}

// body: id, name, configs
func metricViewPut(c *gin.Context) {
	var f models.MetricView
	ginx.BindJSON(c, &f)

	view, err := models.MetricViewGet("id = ?", f.Id)
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

	ginx.NewRender(c).Message(view.Update(f.Name, f.Configs))
}
