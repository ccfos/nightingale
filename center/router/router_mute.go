package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/str"
)

// Return all, front-end search and paging
func (rt *Router) alertMuteGetsByBG(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	lst, err := models.AlertMuteGetsByBG(rt.Ctx, bgid)

	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) alertMuteGetsByGids(c *gin.Context) {
	gids := str.IdsInt64(ginx.QueryStr(c, "gids", ""), ",")
	if len(gids) > 0 {
		for _, gid := range gids {
			rt.bgroCheck(c, gid)
		}
	} else {
		me := c.MustGet("user").(*models.User)
		if !me.IsAdmin() {
			var err error
			gids, err = models.MyBusiGroupIds(rt.Ctx, me.Id)
			ginx.Dangerous(err)

			if len(gids) == 0 {
				ginx.NewRender(c).Data([]int{}, nil)
				return
			}
		}
	}

	lst, err := models.AlertMuteGetsByBGIds(rt.Ctx, gids)

	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) alertMuteGets(c *gin.Context) {
	prods := strings.Fields(ginx.QueryStr(c, "prods", ""))
	bgid := ginx.QueryInt64(c, "bgid", -1)
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

// Preview events (alert_cur_event) that match the mute strategy based on the following criteria:
// business group ID (group_id, group_id), product (prod, rule_prod),
// alert event severity (severities, severity), and event tags (tags, tags).
// For products of type not 'host', also consider the category (cate, cate) and datasource ID (datasource_ids, datasource_id).
func (rt *Router) alertMutePreview(c *gin.Context) {
	//Generally the match of events would be less.

	var f models.AlertMute
	ginx.BindJSON(c, &f)
	f.GroupId = ginx.UrlParamInt64(c, "id")
	ginx.Dangerous(f.Verify()) //verify and parse tags json to ITags
	events, err := models.AlertCurEventGetsFromAlertMute(rt.Ctx, &f)
	ginx.Dangerous(err)

	matchEvents := make([]*models.AlertCurEvent, 0, len(events))
	for i := 0; i < len(events); i++ {
		events[i].DB2Mem()
		if common.MatchTags(events[i].TagsMap, f.ITags) {
			matchEvents = append(matchEvents, events[i])
		}
	}
	ginx.NewRender(c).Data(matchEvents, err)

}

func (rt *Router) alertMuteAddByService(c *gin.Context) {
	var f models.AlertMute
	ginx.BindJSON(c, &f)

	err := f.Add(rt.Ctx)
	ginx.NewRender(c).Data(f.Id, err)
}

func (rt *Router) alertMuteDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	ginx.NewRender(c).Message(models.AlertMuteDel(rt.Ctx, f.Ids))
}

// alertMuteGet returns the alert mute by ID
func (rt *Router) alertMuteGet(c *gin.Context) {
	amid := ginx.UrlParamInt64(c, "amid")
	am, err := models.AlertMuteGetById(rt.Ctx, amid)
	am.DB2FE()
	ginx.NewRender(c).Data(am, err)
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
