package router

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/slice"
	"github.com/ccfos/nightingale/v6/pkg/strx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

func (rt *Router) messageTemplatesAdd(c *gin.Context) {
	var lst []*models.MessageTemplate
	ginx.BindJSON(c, &lst)
	if len(lst) == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	me := c.MustGet("user").(*models.User)
	isAdmin := me.IsAdmin()
	idents := make([]string, 0, len(lst))
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	now := time.Now().Unix()
	for _, tpl := range lst {
		ginx.Dangerous(tpl.Verify())
		if !isAdmin && !slice.HaveIntersection(gids, tpl.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "no permission")
		}
		idents = append(idents, tpl.Ident)

		tpl.CreateBy = me.Username
		tpl.CreateAt = now
		tpl.UpdateBy = me.Username
		tpl.UpdateAt = now
	}

	lstWithSameId, err := models.MessageTemplatesGet(rt.Ctx, "ident IN ?", idents)
	ginx.Dangerous(err)
	if len(lstWithSameId) > 0 {
		ginx.Bomb(http.StatusBadRequest, "ident already exists")
	}

	ids := make([]int64, 0, len(lst))
	for _, tpl := range lst {
		err := models.Insert(rt.Ctx, tpl)
		ginx.Dangerous(err)

		ids = append(ids, tpl.ID)
	}
	ginx.NewRender(c).Data(ids, nil)
}

func (rt *Router) messageTemplatesDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	lst, err := models.MessageTemplatesGet(rt.Ctx, "id in (?)", f.Ids)
	ginx.Dangerous(err)
	notifyRuleIds, err := models.UsedByNotifyRule(rt.Ctx, models.MsgTplList(lst))
	ginx.Dangerous(err)
	if len(notifyRuleIds) > 0 {
		ginx.NewRender(c).Message(fmt.Errorf("used by notify rule: %v", notifyRuleIds))
		return
	}
	if me := c.MustGet("user").(*models.User); !me.IsAdmin() {
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		for _, t := range lst {
			if !slice.HaveIntersection[int64](gids, t.UserGroupIds) {
				ginx.Bomb(http.StatusForbidden, "no permission")
			}
		}
	}

	ginx.NewRender(c).Message(models.DB(rt.Ctx).Delete(
		&models.MessageTemplate{}, "id in (?)", f.Ids).Error)
}

func (rt *Router) messageTemplatePut(c *gin.Context) {
	var f models.MessageTemplate
	ginx.BindJSON(c, &f)

	mt, err := models.MessageTemplateGet(rt.Ctx, "id <> ? and ident = ?", ginx.UrlParamInt64(c, "id"), f.Ident)
	ginx.Dangerous(err)
	if mt != nil {
		ginx.Bomb(http.StatusBadRequest, "message template ident already exists")
	}

	mt, err = models.MessageTemplateGet(rt.Ctx, "id = ?", ginx.UrlParamInt64(c, "id"))
	ginx.Dangerous(err)
	if mt == nil {
		ginx.Bomb(http.StatusNotFound, "message template not found")
	}

	me := c.MustGet("user").(*models.User)
	if !me.IsAdmin() {
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		if !slice.HaveIntersection[int64](gids, mt.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "no permission")
		}
	}

	f.UpdateBy = me.Username
	ginx.NewRender(c).Message(mt.Update(rt.Ctx, f))
}

func (rt *Router) messageTemplateGet(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	tid := ginx.UrlParamInt64(c, "id")
	mt, err := models.MessageTemplateGet(rt.Ctx, "id = ?", tid)
	ginx.Dangerous(err)
	if mt == nil {
		ginx.Bomb(http.StatusNotFound, "message template not found")
	}
	if mt.Private == 1 && !slice.HaveIntersection[int64](gids, mt.UserGroupIds) {
		ginx.Bomb(http.StatusForbidden, "no permission")
	}

	ginx.NewRender(c).Data(mt, nil)
}

func (rt *Router) messageTemplatesGet(c *gin.Context) {
	var notifyChannelIdents []string
	if tmp := ginx.QueryStr(c, "notify_channel_idents", ""); tmp != "" {
		notifyChannelIdents = strings.Split(tmp, ",")
	}
	notifyChannelIds := strx.IdsInt64ForAPI(ginx.QueryStr(c, "notify_channel_ids", ""))
	if len(notifyChannelIds) > 0 {
		ginx.Dangerous(models.DB(rt.Ctx).Model(models.NotifyChannelConfig{}).
			Where("id in (?)", notifyChannelIds).Pluck("ident", &notifyChannelIdents).Error)
	}

	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	lst, err := models.MessageTemplatesGetBy(rt.Ctx, notifyChannelIdents)
	ginx.Dangerous(err)

	if me.IsAdmin() {
		ginx.NewRender(c).Data(lst, nil)
		return
	}

	res := make([]*models.MessageTemplate, 0)
	for _, t := range lst {
		if slice.HaveIntersection[int64](gids, t.UserGroupIds) || t.Private == 0 {
			res = append(res, t)
		}
	}
	ginx.NewRender(c).Data(res, nil)
}

type evtMsgReq struct {
	EventIds []int64 `json:"event_ids"`
	Tpl      struct {
		Content map[string]string `json:"content"`
	} `json:"tpl"`
}

func (rt *Router) eventsMessage(c *gin.Context) {
	var req evtMsgReq
	ginx.BindJSON(c, &req)

	hisEvents, err := models.AlertHisEventGetByIds(rt.Ctx, req.EventIds)
	ginx.Dangerous(err)

	if len(hisEvents) == 0 {
		ginx.Bomb(http.StatusBadRequest, "event not found")
	}

	ginx.Dangerous(err)
	events := make([]*models.AlertCurEvent, len(hisEvents))
	for i, he := range hisEvents {
		events[i] = he.ToCur()
	}

	var defs = []string{
		"{{$events := .}}",
		"{{$event := index . 0}}",
	}
	ret := make(map[string]string, len(req.Tpl.Content))
	for k, v := range req.Tpl.Content {
		text := strings.Join(append(defs, v), "")
		tpl, err := template.New(k).Funcs(tplx.TemplateFuncMap).Parse(text)
		if err != nil {
			ret[k] = err.Error()
			continue
		}

		var buf bytes.Buffer
		err = tpl.Execute(&buf, events)
		if err != nil {
			ret[k] = err.Error()
			continue
		}

		ret[k] = buf.String()
	}
	ginx.NewRender(c).Data(ret, nil)
}
