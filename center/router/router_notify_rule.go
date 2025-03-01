package router

import (
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/alert/dispatch"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/slice"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

func (rt *Router) notifyRulesAdd(c *gin.Context) {
	var lst []*models.NotifyRule
	ginx.BindJSON(c, &lst)
	if len(lst) == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	me := c.MustGet("user").(*models.User)
	isAdmin := me.IsAdmin()
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	for _, nr := range lst {
		ginx.Dangerous(nr.Verify())
		if !isAdmin && !slice.HaveIntersection(gids, nr.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "no permission")
		}

		nr.CreateBy = me.Username
		nr.CreateAt = time.Now().Unix()
		nr.UpdateBy = me.Username
		nr.UpdateAt = time.Now().Unix()
	}

	ginx.Dangerous(models.DB(rt.Ctx).CreateInBatches(lst, 100).Error)
	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) notifyRulesDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	if me := c.MustGet("user").(*models.User); !me.IsAdmin() {
		lst, err := models.NotifyRulesGet(rt.Ctx, "id in (?)", f.Ids)
		ginx.Dangerous(err)
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		for _, t := range lst {
			if !slice.HaveIntersection[int64](gids, t.UserGroupIds) {
				ginx.Bomb(http.StatusForbidden, "no permission")
			}
		}
	}

	ginx.NewRender(c).Message(models.DB(rt.Ctx).
		Delete(&models.NotifyRule{}, "id in (?)", f.Ids).Error)
}

func (rt *Router) notifyRulePut(c *gin.Context) {
	var f models.NotifyRule
	ginx.BindJSON(c, &f)

	nr, err := models.NotifyRuleGet(rt.Ctx, "id = ?", ginx.UrlParamInt64(c, "id"))
	ginx.Dangerous(err)
	if nr == nil {
		ginx.Bomb(http.StatusNotFound, "notify rule not found")
	}

	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	if !slice.HaveIntersection[int64](gids, nr.UserGroupIds) {
		ginx.Bomb(http.StatusForbidden, "no permission")
	}

	f.UpdateBy = me.Username
	ginx.NewRender(c).Message(nr.Update(rt.Ctx, f))
}

func (rt *Router) notifyRuleGet(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	tid := ginx.UrlParamInt64(c, "id")
	nr, err := models.NotifyRuleGet(rt.Ctx, "id = ?", tid)
	ginx.Dangerous(err)
	if nr == nil {
		ginx.Bomb(http.StatusNotFound, "notify rule not found")
	}
	if !slice.HaveIntersection[int64](gids, nr.UserGroupIds) {
		ginx.Bomb(http.StatusForbidden, "no permission")
	}

	ginx.NewRender(c).Data(nr, nil)
}

func (rt *Router) notifyRulesGet(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	lst, err := models.NotifyRulesGet(rt.Ctx, "", nil)
	ginx.Dangerous(err)

	res := make([]*models.NotifyRule, 0)
	for _, nr := range lst {
		if slice.HaveIntersection[int64](gids, nr.UserGroupIds) {
			res = append(res, nr)
		}
	}
	ginx.NewRender(c).Data(res, nil)
}

type NotifyTestForm struct {
	EventIDs     []int64             `json:"event_ids" binding:"required"`
	NotifyConfig models.NotifyConfig `json:"notify_config" binding:"required"`
}

func (rt *Router) notifyTest(c *gin.Context) {
	var f NotifyTestForm
	ginx.BindJSON(c, &f)

	hisEvents, err := models.AlertHisEventGetByIds(rt.Ctx, f.EventIDs)
	ginx.Dangerous(err)

	if len(hisEvents) == 0 {
		ginx.Bomb(http.StatusBadRequest, "event not found")
	}

	ginx.Dangerous(err)
	events := make([]*models.AlertCurEvent, len(hisEvents))
	for i, he := range hisEvents {
		events[i] = he.ToCur()
	}

	if !dispatch.NotifyRuleApplicable(&f.NotifyConfig, events[0]) {
		ginx.Bomb(http.StatusBadRequest, "event not applicable")
	}

	messageTemplates, err := models.MessageTemplateGets(rt.Ctx, f.NotifyConfig.TemplateID, "", "")
	ginx.Dangerous(err)
	if len(messageTemplates) == 0 {
		ginx.Bomb(http.StatusBadRequest, "message template not found")
	}
	tplContent := messageTemplates[0].RenderEvent(events)

	notifyChannels, err := models.NotifyChannelGets(rt.Ctx, f.NotifyConfig.ChannelID, "", "", -1)
	ginx.Dangerous(err)
	if len(notifyChannels) == 0 {
		ginx.Bomb(http.StatusBadRequest, "notify channel not found")
	}

	notifyChannel := notifyChannels[0]
	userInfos, flashDutyChannelIDs, customParams := dispatch.GetNotifyConfigParams(&f.NotifyConfig, rt.UserCache, rt.UserGroupCache)

	switch notifyChannel.RequestType {
	case "flashduty":
		client, err := models.GetHTTPClient(notifyChannel)
		ginx.Dangerous(err)
		for i := range flashDutyChannelIDs {
			_, err = notifyChannel.SendFlashDuty(events, flashDutyChannelIDs[i], client)
			if err != nil {
				break
			}
		}
		ginx.NewRender(c).Message(err)
	case "http":
		client, err := models.GetHTTPClient(notifyChannel)
		ginx.Dangerous(err)
		if notifyChannel.ParamConfig.UserInfo != nil && len(userInfos) > 0 {
			for i := range userInfos {
				_, err = notifyChannel.SendHTTP(events, tplContent, customParams, userInfos[i], client)
				if err != nil {
					logger.Errorf("failed to send http notify: %v", err)
					ginx.NewRender(c).Message(err)
					break
				}
				ginx.NewRender(c).Message(err)
			}
		} else {
			_, err = notifyChannel.SendHTTP(events, tplContent, customParams, nil, client)
			if err != nil {
				logger.Errorf("failed to send http notify: %v", err)
			}
			ginx.NewRender(c).Message(err)
		}
	case "email":
		err := notifyChannel.SendEmail2(events, tplContent, userInfos)
		ginx.NewRender(c).Message(err)
	case "script":
		_, _, err := notifyChannel.SendScript(events, tplContent, customParams, userInfos)
		ginx.NewRender(c).Message(err)
	default:
		ginx.NewRender(c).Message(errors.New("unsupported request type"))
	}
}
