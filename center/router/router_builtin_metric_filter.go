package router

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) metricFilterGets(c *gin.Context) {
	lst, err := models.MetricFilterGets(rt.Ctx, "")
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) metricFilterAdd(c *gin.Context) {
	var f models.MetricFilter
	ginx.BindJSON(c, &f)
	me := c.MustGet("user").(*models.User)
	f.ID = 0
	f.CreateBy = me.Username
	f.UpdateBy = me.Username
	ginx.Dangerous(f.Add(rt.Ctx))
	ginx.NewRender(c).Data(f, nil)
}

func (rt *Router) metricFilterDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()
	ginx.NewRender(c).Message(models.MetricFilterDel(rt.Ctx, f.Ids))
}

func (rt *Router) metricFilterPut(c *gin.Context) {
	var f models.MetricFilter
	ginx.BindJSON(c, &f)

	filter, err := models.MetricFilterGets(rt.Ctx, "id = ?", f.ID)
	ginx.Dangerous(err)
	if len(filter) == 0 {
		ginx.NewRender(c).Message("no such item(id: %d)", f.ID)
		return
	}

	me := c.MustGet("user").(*models.User)
	f.UpdateBy = me.Username
	ginx.NewRender(c).Message(f.Update(rt.Ctx))
}
