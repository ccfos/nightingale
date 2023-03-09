package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

// Return all, front-end search and paging
func (rt *Router) alertMuteGetsByBG(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	lst, err := models.AlertMuteGetsByBG(rt.Ctx, bgid)

	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) alertMuteGets(c *gin.Context) {
	prods := strings.Fields(ginx.QueryStr(c, "prods", ""))
	bgid := ginx.QueryInt64(c, "bgid", 0)
	query := ginx.QueryStr(c, "query", "")
	lst, err := models.AlertMuteGets(rt.Ctx, prods, bgid, query)

	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) alertMuteAdd(c *gin.Context) {
	var f models.AlertMute
	ginx.BindJSON(c, &f)

	username := c.MustGet("username").(string)
	f.CreateBy = username
	f.GroupId = ginx.UrlParamInt64(c, "id")

	ginx.NewRender(c).Message(f.Add(rt.Ctx))
}

func (rt *Router) alertMuteAddByService(c *gin.Context) {
	var f models.AlertMute
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(f.Add(rt.Ctx))
}

func (rt *Router) alertMuteDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	ginx.NewRender(c).Message(models.AlertMuteDel(rt.Ctx, f.Ids))
}

func (rt *Router) alertMutePutByFE(c *gin.Context) {
	var f models.AlertMute
	ginx.BindJSON(c, &f)

	amid := ginx.UrlParamInt64(c, "amid")
	am, err := models.AlertMuteGetById(rt.Ctx, amid)
	ginx.Dangerous(err)

	if am == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such AlertMute")
		return
	}

	rt.bgrwCheck(c, am.GroupId)

	f.UpdateBy = c.MustGet("username").(string)
	ginx.NewRender(c).Message(am.Update(rt.Ctx, f))
}

type alertMuteFieldForm struct {
	Ids    []int64                `json:"ids"`
	Fields map[string]interface{} `json:"fields"`
}

func (rt *Router) alertMutePutFields(c *gin.Context) {
	var f alertMuteFieldForm
	ginx.BindJSON(c, &f)

	if len(f.Fields) == 0 {
		ginx.Bomb(http.StatusBadRequest, "fields empty")
	}

	f.Fields["update_by"] = c.MustGet("username").(string)
	f.Fields["update_at"] = time.Now().Unix()

	for i := 0; i < len(f.Ids); i++ {
		am, err := models.AlertMuteGetById(rt.Ctx, f.Ids[i])
		ginx.Dangerous(err)

		if am == nil {
			continue
		}

		am.FE2DB()
		ginx.Dangerous(am.UpdateFieldsMap(rt.Ctx, f.Fields))
	}

	ginx.NewRender(c).Message(nil)
}
