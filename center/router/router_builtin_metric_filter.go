package router

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) metricFilterGets(c *gin.Context) {
	lst, err := models.MetricFilterGets(rt.Ctx, "")
	ginx.Dangerous(err)
	me := c.MustGet("user").(*models.User)

	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	arr := make([]models.MetricFilter, 0)

	for _, f := range lst {
		if me.Username == f.CreateBy {
			arr = append(arr, f)
			continue
		}

		if HasPerm(gids, f.GroupsPerm, false) {
			arr = append(arr, f)
		}
	}

	ginx.NewRender(c).Data(arr, err)
}

func (rt *Router) metricFilterAdd(c *gin.Context) {
	var f models.MetricFilter
	ginx.BindJSON(c, &f)
	me := c.MustGet("user").(*models.User)

	f.CreateBy = me.Username
	f.UpdateBy = me.Username
	ginx.Dangerous(f.Add(rt.Ctx))
	ginx.NewRender(c).Data(f, nil)
}

func (rt *Router) metricFilterDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	me := c.MustGet("user").(*models.User)

	for _, id := range f.Ids {
		old, err := models.MetricFilterGet(rt.Ctx, id)
		ginx.Dangerous(err)

		if me.Username != old.CreateBy {
			gids, err := models.MyGroupIds(rt.Ctx, me.Id)
			ginx.Dangerous(err)

			if !HasPerm(gids, old.GroupsPerm, true) {
				ginx.NewRender(c).Message("no permission")
				return
			}
		}
	}

	ginx.NewRender(c).Message(models.MetricFilterDel(rt.Ctx, f.Ids))
}

func (rt *Router) metricFilterPut(c *gin.Context) {
	var f models.MetricFilter
	ginx.BindJSON(c, &f)

	me := c.MustGet("user").(*models.User)
	old, err := models.MetricFilterGet(rt.Ctx, f.ID)
	ginx.Dangerous(err)

	if me.Username != old.CreateBy {
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)

		if !HasPerm(gids, old.GroupsPerm, true) {
			ginx.NewRender(c).Message("no permission")
			return
		}
	}

	f.UpdateBy = me.Username
	ginx.NewRender(c).Message(f.Update(rt.Ctx))
}

type metricPromqlReq struct {
	LabelFilter string `json:"label_filter"`
	Promql      string `json:"promql"`
}

func (rt *Router) getMetricPromql(c *gin.Context) {
	var req metricPromqlReq
	ginx.BindJSON(c, &req)

	promql := prom.AddLabelToPromQL(req.LabelFilter, req.Promql)
	ginx.NewRender(c).Data(promql, nil)
}

func HasPerm(gids []int64, gps []models.GroupPerm, checkWrite bool) bool {
	gmap := make(map[int64]struct{})
	for _, gp := range gps {
		if checkWrite && !gp.Write {
			continue
		}
		gmap[gp.Gid] = struct{}{}
	}

	for _, gid := range gids {
		if _, ok := gmap[gid]; ok {
			return true
		}
	}

	return false
}
