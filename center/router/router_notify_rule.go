package router

import (
	"fmt"
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

	now := time.Now().Unix()
	for _, nr := range lst {
		ginx.Dangerous(nr.Verify())
		if !isAdmin && !slice.HaveIntersection(gids, nr.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "no permission")
		}

		nr.CreateBy = me.Username
		nr.CreateAt = now
		nr.UpdateBy = me.Username
		nr.UpdateAt = now

		err := models.Insert(rt.Ctx, nr)
		ginx.Dangerous(err)
	}
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
	if !slice.HaveIntersection[int64](gids, nr.UserGroupIds) && !me.IsAdmin() {
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

	if !slice.HaveIntersection[int64](gids, nr.UserGroupIds) && !me.IsAdmin() {
		ginx.Bomb(http.StatusForbidden, "no permission")
	}

	ginx.NewRender(c).Data(nr, nil)
}

func (rt *Router) notifyRulesGetByService(c *gin.Context) {
	ginx.NewRender(c).Data(models.NotifyRulesGet(rt.Ctx, "enable = ?", true))
}

func (rt *Router) notifyRulesGet(c *gin.Context) {
	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	lst, err := models.NotifyRulesGet(rt.Ctx, "", nil)
	ginx.Dangerous(err)
	if me.IsAdmin() {
		ginx.NewRender(c).Data(lst, nil)
		return
	}

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
	events := []*models.AlertCurEvent{}
	for _, he := range hisEvents {
		event := he.ToCur()
		event.SetTagsMap()
		if dispatch.NotifyRuleApplicable(&f.NotifyConfig, event) {
			events = append(events, event)
		}
	}

	if len(events) == 0 {
		ginx.Bomb(http.StatusBadRequest, "not events applicable")
	}

	notifyChannels, err := models.NotifyChannelGets(rt.Ctx, f.NotifyConfig.ChannelID, "", "", -1)
	ginx.Dangerous(err)
	if len(notifyChannels) == 0 {
		ginx.Bomb(http.StatusBadRequest, "notify channel not found")
	}

	notifyChannel := notifyChannels[0]

	if !notifyChannel.Enable {
		ginx.Bomb(http.StatusBadRequest, "notify channel not enabled, please enable it first")
	}

	tplContent := make(map[string]interface{})
	if notifyChannel.RequestType != "flashtudy" {
		messageTemplates, err := models.MessageTemplateGets(rt.Ctx, f.NotifyConfig.TemplateID, "", "")
		ginx.Dangerous(err)
		if len(messageTemplates) == 0 {
			ginx.Bomb(http.StatusBadRequest, "message template not found")
		}
		tplContent = messageTemplates[0].RenderEvent(events)
	}

	var contactKey string
	if notifyChannel.ParamConfig != nil && notifyChannel.ParamConfig.UserInfo != nil {
		contactKey = notifyChannel.ParamConfig.UserInfo.ContactKey
	}

	sendtos, flashDutyChannelIDs, customParams := dispatch.GetNotifyConfigParams(&f.NotifyConfig, contactKey, rt.UserCache, rt.UserGroupCache)

	var resp string
	switch notifyChannel.RequestType {
	case "flashduty":
		client, err := models.GetHTTPClient(notifyChannel)
		ginx.Dangerous(err)

		for i := range flashDutyChannelIDs {
			resp, err = notifyChannel.SendFlashDuty(events, flashDutyChannelIDs[i], client)
			if err != nil {
				break
			}
		}
		logger.Infof("channel_name: %v, event:%+v, tplContent:%s, customParams:%v, respBody: %v, err: %v", notifyChannel.Name, events[0], tplContent, customParams, resp, err)
		ginx.NewRender(c).Data(resp, err)
	case "http":
		client, err := models.GetHTTPClient(notifyChannel)
		ginx.Dangerous(err)

		if notifyChannel.RequestConfig == nil {
			ginx.Bomb(http.StatusBadRequest, "request config not found")
		}

		if notifyChannel.RequestConfig.HTTPRequestConfig == nil {
			ginx.Bomb(http.StatusBadRequest, "http request config not found")
		}

		if dispatch.NeedBatchContacts(notifyChannel.RequestConfig.HTTPRequestConfig) || len(sendtos) == 0 {
			resp, err = notifyChannel.SendHTTP(events, tplContent, customParams, sendtos, client)
			logger.Infof("channel_name: %v, event:%+v, sendtos:%+v, tplContent:%s, customParams:%v, respBody: %v, err: %v", notifyChannel.Name, events[0], sendtos, tplContent, customParams, resp, err)
			if err != nil {
				logger.Errorf("failed to send http notify: %v", err)
			}
			ginx.NewRender(c).Data(resp, err)
		} else {
			for i := range sendtos {
				resp, err = notifyChannel.SendHTTP(events, tplContent, customParams, []string{sendtos[i]}, client)
				logger.Infof("channel_name: %v, event:%+v,  tplContent:%s, customParams:%v, sendto:%+v, respBody: %v, err: %v", notifyChannel.Name, events[0], tplContent, customParams, sendtos[i], resp, err)
				if err != nil {
					logger.Errorf("failed to send http notify: %v", err)
					ginx.NewRender(c).Message(err)
					return
				}
			}
			ginx.NewRender(c).Message(err)
		}

	case "smtp":
		err := notifyChannel.SendEmailNow(events, tplContent, sendtos)
		ginx.NewRender(c).Message(err)
	case "script":
		resp, _, err := notifyChannel.SendScript(events, tplContent, customParams, sendtos)
		logger.Infof("channel_name: %v, event:%+v, tplContent:%s, customParams:%v, respBody: %v, err: %v", notifyChannel.Name, events[0], tplContent, customParams, resp, err)
		ginx.NewRender(c).Data(resp, err)
	default:
		logger.Errorf("unsupported request type: %v", notifyChannel.RequestType)
		ginx.NewRender(c).Message(errors.New("unsupported request type"))
	}
}

type paramList struct {
	Name  string      `json:"name"`
	CName string      `json:"cname"`
	Value interface{} `json:"value"`
}

func (rt *Router) notifyRuleCustomParamsGet(c *gin.Context) {
	notifyChannelID := ginx.QueryInt64(c, "notify_channel_id")

	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)

	notifyChannel, err := models.NotifyChannelGet(rt.Ctx, "id=?", notifyChannelID)
	ginx.Dangerous(err)

	keyMap := make(map[string]string)
	if notifyChannel == nil {
		ginx.NewRender(c).Data([][]paramList{}, nil)
		return
	}

	if notifyChannel.ParamConfig == nil {
		ginx.NewRender(c).Data([][]paramList{}, nil)
		return
	}

	for _, param := range notifyChannel.ParamConfig.Custom.Params {
		keyMap[param.Key] = param.CName
	}

	lst, err := models.NotifyRulesGet(rt.Ctx, "", nil)
	ginx.Dangerous(err)

	res := make([][]paramList, 0)
	filter := make(map[string]struct{})
	for _, nr := range lst {
		if !slice.HaveIntersection[int64](gids, nr.UserGroupIds) {
			continue
		}

		for _, nc := range nr.NotifyConfigs {
			if nc.ChannelID != notifyChannelID {
				continue
			}

			list := make([]paramList, 0)
			filterKey := ""
			for key, value := range nc.Params {
				// 找到在通知媒介中的自定义变量配置项，进行 cname 转换
				cname, exsits := keyMap[key]
				if exsits {
					list = append(list, paramList{
						Name:  key,
						CName: cname,
						Value: value,
					})
				}
				filterKey += fmt.Sprintf("%s:%s,", key, value)
			}
			if _, ok := filter[filterKey]; ok {
				continue
			}
			filter[filterKey] = struct{}{}
			res = append(res, list)
		}
	}

	ginx.NewRender(c).Data(res, nil)
}
