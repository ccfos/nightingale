package router

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/alert/dispatch"
	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/slice"

	"github.com/gin-gonic/gin"
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
			ginx.Bomb(http.StatusForbidden, "forbidden")
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
			if !slice.HaveIntersection(gids, t.UserGroupIds) {
				ginx.Bomb(http.StatusForbidden, "forbidden")
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
	if !slice.HaveIntersection(gids, nr.UserGroupIds) && !me.IsAdmin() {
		ginx.Bomb(http.StatusForbidden, "forbidden")
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

	if !slice.HaveIntersection(gids, nr.UserGroupIds) && !me.IsAdmin() {
		ginx.Bomb(http.StatusForbidden, "forbidden")
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
	models.FillUpdateByNicknames(rt.Ctx, lst)
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
		if err := dispatch.NotifyRuleMatchCheck(&f.NotifyConfig, event); err != nil {
			ginx.Bomb(http.StatusBadRequest, err.Error())
		}

		events = append(events, event)
	}

	resp, err := SendNotifyChannelMessage(rt.Ctx, rt.UserCache, rt.UserGroupCache, f.NotifyConfig, events)
	if resp == "" {
		resp = "success"
	}
	ginx.NewRender(c).Data(resp, err)
}

func SendNotifyChannelMessage(ctx *ctx.Context, userCache *memsto.UserCacheType, userGroup *memsto.UserGroupCacheType, notifyConfig models.NotifyConfig, events []*models.AlertCurEvent) (string, error) {
	notifyChannels, err := models.NotifyChannelGets(ctx, notifyConfig.ChannelID, "", "", -1)
	if err != nil {
		return "", fmt.Errorf("failed to get notify channels: %v", err)
	}

	if len(notifyChannels) == 0 {
		return "", fmt.Errorf("notify channel not found")
	}

	notifyChannel := notifyChannels[0]
	if !notifyChannel.Enable {
		return "", fmt.Errorf("notify channel not enabled, please enable it first")
	}

	// 获取站点URL用于模板渲染
	siteUrl, _ := models.ConfigsGetSiteUrl(ctx)
	if siteUrl == "" {
		siteUrl = "http://127.0.0.1:17000"
	}

	tplContent := make(map[string]interface{})
	// flashduty / pagerduty 不依赖模板，从 event 字段直接构造 payload
	if notifyChannel.RequestType != "flashduty" && notifyChannel.RequestType != "pagerduty" {
		messageTemplates, err := models.MessageTemplateGets(ctx, notifyConfig.TemplateID, "", "")
		if err != nil {
			return "", fmt.Errorf("failed to get message templates: %v", err)
		}

		if len(messageTemplates) == 0 {
			return "", fmt.Errorf("message template not found")
		}
		tplContent = messageTemplates[0].RenderEvent(events, siteUrl)
	}

	client, err := models.GetHTTPClient(notifyChannel)
	if err != nil {
		return "", fmt.Errorf("failed to get http client: %v", err)
	}
	nc, err := dispatch.BuildNotifyContext(ctx, userCache, userGroup, events, 0,
		&notifyConfig, notifyChannel, tplContent, client, siteUrl)
	if err != nil {
		return "", err
	}

	// smtp: Provider.Notify 走 SmtpChan 是异步入队，test-send 要同步拿结果
	if notifyChannel.RequestType == "smtp" {
		if len(nc.Request.Sendtos) == 0 {
			return "", fmt.Errorf("no valid email address in the user and team")
		}
		if err := provider.SendEmailNow(notifyChannel, events, tplContent, nc.Request.Sendtos); err != nil {
			return "", fmt.Errorf("failed to send email notify: %v", err)
		}
		return "", nil
	}

	// http: test-send 特有，按 sendto 扇出，让前端能看到每个收件人的结果
	if notifyChannel.RequestType == "http" {
		if notifyChannel.RequestConfig == nil || notifyChannel.RequestConfig.HTTPRequestConfig == nil {
			return "", fmt.Errorf("http request config is nil")
		}
		sendtos := nc.Request.Sendtos
		batches := [][]string{sendtos}
		if !dispatch.NeedBatchContacts(notifyChannel.RequestConfig.HTTPRequestConfig) && len(sendtos) > 0 {
			batches = make([][]string, len(sendtos))
			for i := range sendtos {
				batches[i] = []string{sendtos[i]}
			}
		}
		var lastResp string
		for _, batch := range batches {
			// 每轮拷贝一份 request，保持 nc.Request 的不可变快照语义，
			// 避免 Provider 内部异步持有引用时读到被后续迭代改写的 Sendtos
			reqCopy := *nc.Request
			reqCopy.Sendtos = batch
			r := nc.Provider.Notify(ctx.Ctx, &reqCopy)
			logger.Infof("channel_name=%s event=%s sendto=%v customParams=%v resp=%s err=%v",
				notifyChannel.Name, events[0].Hash, batch, nc.Request.CustomParams, r.Response, r.Err)
			if r.Err != nil {
				return "", fmt.Errorf("failed to send http notify: %v", r.Err)
			}
			lastResp = r.Response
		}
		return lastResp, nil
	}

	// 其余：flashduty / pagerduty / script / feishuapp / wecomapp
	// TODO(dingtalkapp): 钉钉应用本次不上线，上线时在注释中补回 dingtalkapp。
	r := nc.Provider.Notify(ctx.Ctx, nc.Request)
	logger.Infof("channel_name=%s event=%s sendtos=%v customParams=%v resp=%s err=%v",
		notifyChannel.Name, events[0].Hash, nc.Request.Sendtos, nc.Request.CustomParams, r.Response, r.Err)
	return r.Response, r.Err
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
				cname, exists := keyMap[key]
				if exists {
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
