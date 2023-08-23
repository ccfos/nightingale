package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/mute"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

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
	bgid := ginx.QueryInt64(c, "bgid", -1)
	query := ginx.QueryStr(c, "query", "")
	lst, err := models.AlertMuteGets(rt.Ctx, prods, bgid, query)

	ginx.NewRender(c).Data(lst, err)
}

//When creating a mute rule, allow preview of the active alerts that will be matched,
//and provide an option to delete these active alerts with one click

type form struct {
	models.AlertMute
	DelCurAlert bool `json:"del_alert_cur"`
}

func (rt *Router) alertMuteAdd(c *gin.Context) {

	var f form
	ginx.BindJSON(c, &f)

	username := c.MustGet("username").(string)
	f.CreateBy = username
	f.GroupId = ginx.UrlParamInt64(c, "id")
	err := f.Add(rt.Ctx)
	ginx.NewRender(c).Message(err)
	if f.DelCurAlert && err == nil {
		//find match events,and delete alert_cur by id
		events := matchMuteEvents(rt.Ctx, ginx.UrlParamInt64(c, "id"), &f.AlertMute)
		ids := eventsIdFilter(events)
		rt.checkCurEventBusiGroupRWPermission(c, ids)
		ginx.NewRender(c).Message(models.AlertCurEventDel(rt.Ctx, ids))
	}
}

//preview events(alert_cur_event) match mute strategy
func (rt *Router) alertMutePreview(c *gin.Context) {
	//Generally the match of events would be less
	//and return the value of match total count(match_total_count).

	var f models.AlertMute
	ginx.BindJSON(c, &f)
	username := c.MustGet("username").(string)
	bgid := ginx.UrlParamInt64(c, "id")
	f.CreateBy = username
	f.GroupId = bgid
	ginx.Dangerous(f.Verify())
	ginx.Dangerous(f.FE2DB())

	events := matchMuteEvents(rt.Ctx, bgid, &f)

	ginx.NewRender(c).Data(gin.H{
		"list": events,
	}, nil)

}

//retrieve the current events for a specific business group ID and filter out the events that match the mute strategy.
func matchMuteEvents(ctx *ctx.Context, bgid int64, alertMute *models.AlertMute) []*models.AlertCurEvent {
	//Prevent accidental muting
	m := map[string]interface{}{"group_id": bgid}
	events, _ := searchCurEvents(ctx, m, 0)
	events = mute.CurEventMatchMuteStrategyFilter(events, alertMute)
	return events
}

// return the IDs of the events
func eventsIdFilter(events []*models.AlertCurEvent) []int64 {
	ids := make([]int64, 0, len(events))
	for i := range events {
		ids = append(ids, events[i].Id)
	}
	return ids
}

// select current events from db. if limit is set to 0, it indicates no limit.
func searchCurEvents(ctx *ctx.Context, where map[string]interface{}, limit int) ([]*models.AlertCurEvent, int64) {

	total, err := models.AlertCurEventTotalMap(ctx, where)
	ginx.Dangerous(err)
	list, err := models.AlertCurEventGetsMap(ctx, where, limit)
	ginx.Dangerous(err)

	cache := make(map[int64]*models.UserGroup)
	for i := 0; i < len(list); i++ {
		list[i].FillNotifyGroups(ctx, cache)
	}
	return list, total
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
