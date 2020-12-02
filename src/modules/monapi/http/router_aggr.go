package http

import (
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/scache"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

func aggrCalcPost(c *gin.Context) {
	username := loginUsername(c)
	stra := new(models.AggrCalc)
	errors.Dangerous(c.ShouldBind(stra))

	can, err := models.UsernameCandoNodeOp(username, "mon_aggr_write", stra.Nid)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	stra.Creator = username
	stra.LastUpdator = username

	errors.Dangerous(stra.Encode())

	oldStra, _ := models.AggrCalcGet("nid=? and new_metric=?", stra.Nid, stra.NewMetric)
	if oldStra != nil {
		bomb("同节点下指标计算 新指标名称 %s 已存在", stra.NewMetric)
	}

	err = stra.Save()
	renderData(c, stra, err)
}

func aggrCalcPut(c *gin.Context) {
	username := loginUsername(c)

	stra := new(models.AggrCalc)
	errors.Dangerous(c.ShouldBind(stra))

	can, err := models.UsernameCandoNodeOp(username, "mon_aggr_write", stra.Nid)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	stra.LastUpdator = username
	errors.Dangerous(stra.Encode())

	oldStra, _ := models.AggrCalcGet("nid=? and new_metric=?", stra.Nid, stra.NewMetric)
	if oldStra != nil && oldStra.Id != stra.Id {
		bomb("同节点下指标计算 新指标名称 %s 已存在", stra.NewMetric)
	}

	err = stra.Update("new_metric", "new_step", "groupby", "raw_metrics", "global_operator",
		"expression", "rpn", "last_updator", "last_updated", "comment")

	renderData(c, "ok", err)
}

type CalcStrasDelRev struct {
	Ids []int64 `json:"ids"`
}

func aggrCalcsDel(c *gin.Context) {
	username := loginUsername(c)
	var rev CalcStrasDelRev
	errors.Dangerous(c.ShouldBind(&rev))

	var ids []int64
	for _, id := range rev.Ids {
		stra, err := models.AggrCalcGet("id=?", id)
		errors.Dangerous(err)
		if stra == nil {
			continue
		}
		ids = append(ids, id)

		can, err := models.UsernameCandoNodeOp(username, "mon_aggr_write", stra.Nid)
		errors.Dangerous(err)
		if !can {
			bomb("permission deny")
		}
	}

	for i := 0; i < len(ids); i++ {
		errors.Dangerous(models.AggrCalcDel(ids[i]))
	}

	renderData(c, "ok", nil)
}

func aggrCalcGet(c *gin.Context) {
	id := urlParamInt64(c, "id")

	stra, err := models.AggrCalcGet("id=?", id)
	errors.Dangerous(err)
	if stra == nil {
		bomb("stra not found")
	}

	err = stra.Decode()
	renderData(c, stra, err)
}

func aggrCalcsGet(c *gin.Context) {
	name := queryStr(c, "name", "")
	nid := mustQueryInt64(c, "nid")
	list, err := models.AggrCalcsList(name, nid)
	renderData(c, list, err)
}

func aggrCalcsWithEndpointGet(c *gin.Context) {
	renderData(c, scache.AggrCalcStraCache.Get(), nil)
}
