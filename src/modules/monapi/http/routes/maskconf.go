package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/model"
)

type MaskconfForm struct {
	Nid       int64    `json:"nid"`
	Endpoints []string `json:"endpoints"`
	Metric    string   `json:"metric"`
	Tags      string   `json:"tags"`
	Cause     string   `json:"cause"`
	Btime     int64    `json:"btime"`
	Etime     int64    `json:"etime"`
}

func (f MaskconfForm) Validate() {
	mustNode(f.Nid)

	if f.Endpoints == nil || len(f.Endpoints) == 0 {
		errors.Bomb("arg[endpoints] empty")
	}

	if f.Btime >= f.Etime {
		errors.Bomb("args[btime,etime] invalid")
	}
}

func maskconfPost(c *gin.Context) {
	var f MaskconfForm
	errors.Dangerous(c.ShouldBind(&f))
	f.Validate()

	obj := &model.Maskconf{
		Nid:    f.Nid,
		Metric: f.Metric,
		Tags:   f.Tags,
		Cause:  f.Cause,
		Btime:  f.Btime,
		Etime:  f.Etime,
		User:   loginUsername(c),
	}

	renderMessage(c, obj.Add(f.Endpoints))
}

func maskconfGets(c *gin.Context) {
	nid := urlParamInt64(c, "id")

	objs, err := model.MaskconfGets(nid)
	errors.Dangerous(err)

	for i := 0; i < len(objs); i++ {
		errors.Dangerous(objs[i].FillEndpoints())
	}

	renderData(c, objs, nil)
}

func maskconfDel(c *gin.Context) {
	id := urlParamInt64(c, "id")
	renderMessage(c, model.MaskconfDel(id))
}

func maskconfPut(c *gin.Context) {
	mc, err := model.MaskconfGet("id", urlParamInt64(c, "id"))
	errors.Dangerous(err)

	if mc == nil {
		errors.Bomb("maskconf is nil")
	}

	var f MaskconfForm
	errors.Dangerous(c.ShouldBind(&f))
	f.Validate()

	mc.Metric = f.Metric
	mc.Tags = f.Tags
	mc.Etime = f.Etime
	mc.Btime = f.Btime
	mc.Cause = f.Cause
	renderMessage(c, mc.Update(f.Endpoints, "metric", "tags", "etime", "btime", "cause"))
}
