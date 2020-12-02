package http

import (
	"strings"

	"github.com/didi/nightingale/src/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
)

type MaskconfForm struct {
	Nid         int64             `json:"nid"`
	Category    int               `json:"category"` //1 设备相关 2 设备无关
	Endpoints   []string          `json:"endpoints"`
	CurNidPaths map[string]string `json:"cur_nid_paths"`
	Metric      string            `json:"metric"`
	Tags        string            `json:"tags"`
	Cause       string            `json:"cause"`
	Btime       int64             `json:"btime"`
	Etime       int64             `json:"etime"`
}

func (f MaskconfForm) Validate() {
	mustNode(f.Nid)

	if f.Category == 1 && (f.Endpoints == nil || len(f.Endpoints) == 0) {
		bomb("arg[endpoints] empty")
	}

	if f.Category == 2 && len(f.CurNidPaths) == 0 {
		bomb("arg[cur_nid_paths] empty")
	}

	if f.Btime >= f.Etime {
		bomb("args[btime,etime] invalid")
	}

	if f.Tags == "" {
		return
	}

	tagsList := strings.Split(f.Tags, ",")
	for i := 0; i < len(tagsList); i++ {
		kv := strings.Split(tagsList[i], "=")
		if len(kv) != 2 {
			bomb("arg[tags] invalid")
		}
	}
}

func maskconfPost(c *gin.Context) {
	var f MaskconfForm
	errors.Dangerous(c.ShouldBind(&f))
	can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_maskconf_create", f.Nid)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	f.Validate()

	obj := &models.Maskconf{
		Nid:      f.Nid,
		Metric:   f.Metric,
		Category: f.Category,
		Tags:     f.Tags,
		Cause:    f.Cause,
		Btime:    f.Btime,
		Etime:    f.Etime,
		User:     loginUsername(c),
	}

	if f.Category == 1 {
		errors.Dangerous(obj.AddEndpoints(f.Endpoints))
	} else {
		errors.Dangerous(obj.AddNids(f.CurNidPaths))
	}

	renderMessage(c, nil)
}

func maskconfGets(c *gin.Context) {
	nid := urlParamInt64(c, "id")

	objs, err := models.MaskconfGets(nid)
	errors.Dangerous(err)

	for i := 0; i < len(objs); i++ {
		if objs[i].Category == 1 {
			errors.Dangerous(objs[i].FillEndpoints())
		} else {
			errors.Dangerous(objs[i].FillNids())
		}
	}

	renderData(c, objs, nil)
}

func maskconfDel(c *gin.Context) {
	id := urlParamInt64(c, "id")

	mask, err := models.MaskconfGet("id", id)
	errors.Dangerous(err)

	can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_maskconf_delete", mask.Nid)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	renderMessage(c, models.MaskconfDel(id))
}

func maskconfPut(c *gin.Context) {
	mc, err := models.MaskconfGet("id", urlParamInt64(c, "id"))
	errors.Dangerous(err)

	if mc == nil {
		bomb("maskconf is nil")
	}

	can, err := models.UsernameCandoNodeOp(loginUsername(c), "mon_maskconf_modify", mc.Nid)
	errors.Dangerous(err)
	if !can {
		bomb("permission deny")
	}

	var f MaskconfForm
	errors.Dangerous(c.ShouldBind(&f))
	f.Validate()

	mc.Metric = f.Metric
	mc.Tags = f.Tags
	mc.Etime = f.Etime
	mc.Btime = f.Btime
	mc.Cause = f.Cause
	mc.Category = f.Category
	mc.Category = f.Category

	if f.Category == 1 {
		renderMessage(c, mc.UpdateEndpoints(f.Endpoints, "metric", "tags", "etime", "btime", "cause", "category"))
	} else {
		renderMessage(c, mc.UpdateNids(f.CurNidPaths, "metric", "tags", "etime", "btime", "cause", "category"))
	}
}
