package dispatch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

type Dispatch struct {
	alertRuleCache      *memsto.AlertRuleCacheType
	userCache           *memsto.UserCacheType
	userGroupCache      *memsto.UserGroupCacheType
	alertSubscribeCache *memsto.AlertSubscribeCacheType
	targetCache         *memsto.TargetCacheType
	notifyConfigCache   *memsto.NotifyConfigCacheType
	taskTplsCache       *memsto.TaskTplCache

	notifyRuleCache      *memsto.NotifyRuleCacheType
	notifyChannelCache   *memsto.NotifyChannelCacheType
	messageTemplateCache *memsto.MessageTemplateCacheType

	alerting aconf.Alerting

	Senders          map[string]sender.Sender
	CallBacks        map[string]sender.CallBacker
	tpls             map[string]*template.Template
	ExtraSenders     map[string]sender.Sender
	BeforeSenderHook func(*models.AlertCurEvent) bool

	ctx    *ctx.Context
	Astats *astats.Stats

	RwLock sync.RWMutex
}

// 创建一个 Notify 实例
func NewDispatch(alertRuleCache *memsto.AlertRuleCacheType, userCache *memsto.UserCacheType, userGroupCache *memsto.UserGroupCacheType,
	alertSubscribeCache *memsto.AlertSubscribeCacheType, targetCache *memsto.TargetCacheType, notifyConfigCache *memsto.NotifyConfigCacheType,
	taskTplsCache *memsto.TaskTplCache, notifyRuleCache *memsto.NotifyRuleCacheType, notifyChannelCache *memsto.NotifyChannelCacheType,
	messageTemplateCache *memsto.MessageTemplateCacheType, alerting aconf.Alerting, ctx *ctx.Context, astats *astats.Stats) *Dispatch {
	notify := &Dispatch{
		alertRuleCache:       alertRuleCache,
		userCache:            userCache,
		userGroupCache:       userGroupCache,
		alertSubscribeCache:  alertSubscribeCache,
		targetCache:          targetCache,
		notifyConfigCache:    notifyConfigCache,
		taskTplsCache:        taskTplsCache,
		notifyRuleCache:      notifyRuleCache,
		notifyChannelCache:   notifyChannelCache,
		messageTemplateCache: messageTemplateCache,

		alerting: alerting,

		Senders:          make(map[string]sender.Sender),
		tpls:             make(map[string]*template.Template),
		ExtraSenders:     make(map[string]sender.Sender),
		BeforeSenderHook: func(*models.AlertCurEvent) bool { return true },

		ctx:    ctx,
		Astats: astats,
	}
	return notify
}

func (e *Dispatch) ReloadTpls() error {
	err := e.reloadTpls()
	if err != nil {
		logger.Errorf("failed to reload tpls: %v", err)
	}

	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := e.reloadTpls(); err != nil {
			logger.Warning("failed to reload tpls:", err)
		}
	}
}

func (e *Dispatch) reloadTpls() error {
	tmpTpls, err := models.ListTpls(e.ctx)
	if err != nil {
		return err
	}
	smtp := e.notifyConfigCache.GetSMTP()

	senders := map[string]sender.Sender{
		models.Email:      sender.NewSender(models.Email, tmpTpls, smtp),
		models.Dingtalk:   sender.NewSender(models.Dingtalk, tmpTpls),
		models.Wecom:      sender.NewSender(models.Wecom, tmpTpls),
		models.Feishu:     sender.NewSender(models.Feishu, tmpTpls),
		models.Mm:         sender.NewSender(models.Mm, tmpTpls),
		models.Telegram:   sender.NewSender(models.Telegram, tmpTpls),
		models.FeishuCard: sender.NewSender(models.FeishuCard, tmpTpls),
		models.Lark:       sender.NewSender(models.Lark, tmpTpls),
		models.LarkCard:   sender.NewSender(models.LarkCard, tmpTpls),
	}

	// domain -> Callback()
	callbacks := map[string]sender.CallBacker{
		models.DingtalkDomain:   sender.NewCallBacker(models.DingtalkDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
		models.WecomDomain:      sender.NewCallBacker(models.WecomDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
		models.FeishuDomain:     sender.NewCallBacker(models.FeishuDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
		models.TelegramDomain:   sender.NewCallBacker(models.TelegramDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
		models.FeishuCardDomain: sender.NewCallBacker(models.FeishuCardDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
		models.IbexDomain:       sender.NewCallBacker(models.IbexDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
		models.LarkDomain:       sender.NewCallBacker(models.LarkDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
		models.DefaultDomain:    sender.NewCallBacker(models.DefaultDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
		models.LarkCardDomain:   sender.NewCallBacker(models.LarkCardDomain, e.targetCache, e.userCache, e.taskTplsCache, tmpTpls),
	}

	e.RwLock.RLock()
	for channelName, extraSender := range e.ExtraSenders {
		senders[channelName] = extraSender
	}
	e.RwLock.RUnlock()

	e.RwLock.Lock()
	e.tpls = tmpTpls
	e.Senders = senders
	e.CallBacks = callbacks
	e.RwLock.Unlock()
	return nil
}

func (e *Dispatch) HandleEventWithNotifyRule(event *models.AlertCurEvent, isSubscribe bool) {

	if len(event.NotifyRuleIDs) > 0 {
		for _, notifyRuleId := range event.NotifyRuleIDs {
			logger.Infof("notify rule ids: %v, event: %+v", notifyRuleId, event)
			notifyRule := e.notifyRuleCache.Get(notifyRuleId)
			if notifyRule == nil {
				continue
			}

			if !notifyRule.Enable {
				continue
			}

			for i := range notifyRule.NotifyConfigs {
				if !NotifyRuleApplicable(&notifyRule.NotifyConfigs[i], event) {
					continue
				}
				notifyChannel := e.notifyChannelCache.Get(notifyRule.NotifyConfigs[i].ChannelID)
				messageTemplate := e.messageTemplateCache.Get(notifyRule.NotifyConfigs[i].TemplateID)
				if notifyChannel == nil {
					sender.NotifyRecord(e.ctx, []*models.AlertCurEvent{event}, notifyRuleId, fmt.Sprintf("notify_channel_id:%d", notifyRule.NotifyConfigs[i].ChannelID), "", "", errors.New("notify_channel not found"))
					logger.Warningf("notify_id: %d, event:%+v, channel_id:%d, template_id: %d, notify_channel not found", notifyRuleId, event, notifyRule.NotifyConfigs[i].ChannelID, notifyRule.NotifyConfigs[i].TemplateID)
					continue
				}

				if notifyChannel.RequestType != "flashduty" && messageTemplate == nil {
					logger.Warningf("notify_id: %d, channel_name: %v, event:%+v, template_id: %d, message_template not found", notifyRuleId, notifyChannel.Ident, event, notifyRule.NotifyConfigs[i].TemplateID)
					sender.NotifyRecord(e.ctx, []*models.AlertCurEvent{event}, notifyRuleId, notifyChannel.Name, "", "", errors.New("message_template not found"))

					continue
				}

				// todo go send
				// todo 聚合 event
				go e.sendV2([]*models.AlertCurEvent{event}, notifyRuleId, &notifyRule.NotifyConfigs[i], notifyChannel, messageTemplate)
			}
		}
	}
}

func NotifyRuleApplicable(notifyConfig *models.NotifyConfig, event *models.AlertCurEvent) bool {
	tm := time.Unix(event.TriggerTime, 0)
	triggerTime := tm.Format("15:04")
	triggerWeek := int(tm.Weekday())

	timeMatch := false

	if len(notifyConfig.TimeRanges) == 0 {
		timeMatch = true
	}
	for j := range notifyConfig.TimeRanges {
		if timeMatch {
			break
		}
		enableStime := notifyConfig.TimeRanges[j].Start
		enableEtime := notifyConfig.TimeRanges[j].End
		enableDaysOfWeek := notifyConfig.TimeRanges[j].Week
		length := len(enableDaysOfWeek)
		// enableStime,enableEtime,enableDaysOfWeek三者长度肯定相同，这里循环一个即可
		for i := 0; i < length; i++ {
			if enableDaysOfWeek[i] != triggerWeek {
				continue
			}

			if enableStime < enableEtime {
				if enableEtime == "23:59" {
					// 02:00-23:59，这种情况做个特殊处理，相当于左闭右闭区间了
					if triggerTime < enableStime {
						// mute, 即没生效
						continue
					}
				} else {
					// 02:00-04:00 或者 02:00-24:00
					if triggerTime < enableStime || triggerTime >= enableEtime {
						// mute, 即没生效
						continue
					}
				}
			} else if enableStime > enableEtime {
				// 21:00-09:00
				if triggerTime < enableStime && triggerTime >= enableEtime {
					// mute, 即没生效
					continue
				}
			}

			// 到这里说明当前时刻在告警规则的某组生效时间范围内，即没有 mute，直接返回 false
			timeMatch = true
			break
		}
	}

	severityMatch := false
	for i := range notifyConfig.Severities {
		if notifyConfig.Severities[i] == event.Severity {
			severityMatch = true
		}
	}

	tagMatch := true
	if len(notifyConfig.LabelKeys) > 0 {
		for i := range notifyConfig.LabelKeys {
			if notifyConfig.LabelKeys[i].Func == "" {
				notifyConfig.LabelKeys[i].Func = notifyConfig.LabelKeys[i].Op
			}
		}

		tagFilters, err := models.ParseTagFilter(notifyConfig.LabelKeys)
		if err != nil {
			logger.Errorf("notify send failed to parse tag filter: %v event:%+v notify_config:%+v", err, event, notifyConfig)
			return false
		}
		tagMatch = common.MatchTags(event.TagsMap, tagFilters)
	}

	attributesMatch := true
	if len(notifyConfig.Attributes) > 0 {
		tagFilters, err := models.ParseTagFilter(notifyConfig.Attributes)
		if err != nil {
			logger.Errorf("notify send failed to parse tag filter: %v event:%+v notify_config:%+v err:%v", tagFilters, event, notifyConfig, err)
			return false
		}

		attributesMatch = common.MatchTags(event.JsonTagsAndValue(), tagFilters)
	}
	logger.Infof("notify send timeMatch:%v severityMatch:%v tagMatch:%v attributesMatch:%v event:%+v notify_config:%+v", timeMatch, severityMatch, tagMatch, attributesMatch, event, notifyConfig)
	return timeMatch && severityMatch && tagMatch && attributesMatch
}

func GetNotifyConfigParams(notifyConfig *models.NotifyConfig, contactKey string, userCache *memsto.UserCacheType, userGroupCache *memsto.UserGroupCacheType) ([]string, []int64, map[string]string) {
	customParams := make(map[string]string)
	var flashDutyChannelIDs []int64
	var userInfoParams models.CustomParams

	for key, value := range notifyConfig.Params {
		switch key {
		case "user_ids", "user_group_ids", "ids":
			if data, err := json.Marshal(value); err == nil {
				var ids []int64
				if json.Unmarshal(data, &ids) == nil {
					if key == "user_ids" {
						userInfoParams.UserIDs = ids
					} else if key == "user_group_ids" {
						userInfoParams.UserGroupIDs = ids
					} else if key == "ids" {
						flashDutyChannelIDs = ids
					}
				}
			}
		default:
			customParams[key] = value.(string)
		}
	}

	if len(userInfoParams.UserIDs) == 0 && len(userInfoParams.UserGroupIDs) == 0 {
		return []string{}, flashDutyChannelIDs, customParams
	}

	userIds := make([]int64, 0)
	userIds = append(userIds, userInfoParams.UserIDs...)

	if len(userInfoParams.UserGroupIDs) > 0 {
		userGroups := userGroupCache.GetByUserGroupIds(userInfoParams.UserGroupIDs)
		for _, userGroup := range userGroups {
			userIds = append(userIds, userGroup.UserIds...)
		}
	}

	users := userCache.GetByUserIds(userIds)
	visited := make(map[int64]bool)
	sendtos := make([]string, 0)
	for _, user := range users {
		if visited[user.Id] {
			continue
		}
		var sendto string
		if contactKey == "phone" {
			sendto = user.Phone
		} else if contactKey == "email" {
			sendto = user.Email
		} else {
			sendto, _ = user.ExtractToken(contactKey)
		}

		if sendto == "" {
			continue
		}
		sendtos = append(sendtos, sendto)
		visited[user.Id] = true
	}

	return sendtos, flashDutyChannelIDs, customParams
}

func (e *Dispatch) sendV2(events []*models.AlertCurEvent, notifyRuleId int64, notifyConfig *models.NotifyConfig, notifyChannel *models.NotifyChannelConfig, messageTemplate *models.MessageTemplate) {
	if len(events) == 0 {
		logger.Errorf("notify_id: %d events is empty", notifyRuleId)
		return
	}

	tplContent := make(map[string]interface{})
	if notifyChannel.RequestType != "flashduty" {
		tplContent = messageTemplate.RenderEvent(events)
	}

	var contactKey string
	if notifyChannel.ParamConfig != nil && notifyChannel.ParamConfig.UserInfo != nil {
		contactKey = notifyChannel.ParamConfig.UserInfo.ContactKey
	}

	sendtos, flashDutyChannelIDs, customParams := GetNotifyConfigParams(notifyConfig, contactKey, e.userCache, e.userGroupCache)

	e.Astats.GaugeNotifyRecordQueueSize.Inc()
	defer e.Astats.GaugeNotifyRecordQueueSize.Dec()

	switch notifyChannel.RequestType {
	case "flashduty":
		for i := range flashDutyChannelIDs {
			respBody, err := notifyChannel.SendFlashDuty(events, flashDutyChannelIDs[i], e.notifyChannelCache.GetHttpClient(notifyChannel.ID))
			logger.Infof("notify_id: %d, channel_name: %v, event:%+v, IntegrationUrl: %v dutychannel_id: %v, respBody: %v, err: %v", notifyRuleId, notifyChannel.Name, events[0], notifyChannel.RequestConfig.FlashDutyRequestConfig.IntegrationUrl, flashDutyChannelIDs[i], respBody, err)
			sender.NotifyRecord(e.ctx, events, notifyRuleId, notifyChannel.Name, strconv.FormatInt(flashDutyChannelIDs[i], 10), respBody, err)
		}
		return
	case "http":
		if e.notifyChannelCache.HttpConcurrencyAdd(notifyChannel.ID) {
			defer e.notifyChannelCache.HttpConcurrencyDone(notifyChannel.ID)
		}
		if notifyChannel.RequestConfig == nil {
			logger.Warningf("notify_id: %d, channel_name: %v, event:%+v, request config not found", notifyRuleId, notifyChannel.Name, events[0])
		}

		if notifyChannel.RequestConfig.HTTPRequestConfig == nil {
			logger.Warningf("notify_id: %d, channel_name: %v, event:%+v, http request config not found", notifyRuleId, notifyChannel.Name, events[0])
		}

		if NeedBatchContacts(notifyChannel.RequestConfig.HTTPRequestConfig) || len(sendtos) == 0 {
			resp, err := notifyChannel.SendHTTP(events, tplContent, customParams, sendtos, e.notifyChannelCache.GetHttpClient(notifyChannel.ID))
			logger.Infof("notify_id: %d, channel_name: %v, event:%+v, tplContent:%s, customParams:%v, userInfo:%+v, respBody: %v, err: %v", notifyRuleId, notifyChannel.Name, events[0], tplContent, customParams, sendtos, resp, err)

			sender.NotifyRecord(e.ctx, events, notifyRuleId, notifyChannel.Name, getSendTarget(customParams, sendtos), resp, err)
		} else {
			for i := range sendtos {
				resp, err := notifyChannel.SendHTTP(events, tplContent, customParams, []string{sendtos[i]}, e.notifyChannelCache.GetHttpClient(notifyChannel.ID))
				logger.Infof("notify_id: %d, channel_name: %v, event:%+v, tplContent:%s, customParams:%v, userInfo:%+v, respBody: %v, err: %v", notifyRuleId, notifyChannel.Name, events[0], tplContent, customParams, sendtos[i], resp, err)
				sender.NotifyRecord(e.ctx, events, notifyRuleId, notifyChannel.Name, getSendTarget(customParams, []string{sendtos[i]}), resp, err)
			}
		}

	case "smtp":
		notifyChannel.SendEmail(notifyRuleId, events, tplContent, sendtos, e.notifyChannelCache.GetSmtpClient(notifyChannel.ID))

	case "script":
		target, res, err := notifyChannel.SendScript(events, tplContent, customParams, sendtos)
		logger.Infof("notify_id: %d, channel_name: %v, event:%+v, tplContent:%s, customParams:%v, target:%s, res:%s, err:%v", notifyRuleId, notifyChannel.Name, events[0], tplContent, customParams, target, res, err)
		sender.NotifyRecord(e.ctx, events, notifyRuleId, notifyChannel.Name, target, res, err)
	default:
		logger.Warningf("notify_id: %d, channel_name: %v, event:%+v send type not found", notifyRuleId, notifyChannel.Name, events[0])
	}
}

func NeedBatchContacts(requestConfig *models.HTTPRequestConfig) bool {
	b, _ := json.Marshal(requestConfig)
	return strings.Contains(string(b), "$sendtos")
}

// HandleEventNotify 处理event事件的主逻辑
// event: 告警/恢复事件
// isSubscribe: 告警事件是否由subscribe的配置产生
func (e *Dispatch) HandleEventNotify(event *models.AlertCurEvent, isSubscribe bool) {
	rule := e.alertRuleCache.Get(event.RuleId)
	if rule == nil {
		return
	}

	if e.blockEventNotify(rule, event) {
		logger.Infof("block event notify: rule_id:%d event:%+v", rule.Id, event)
		return
	}

	fillUsers(event, e.userCache, e.userGroupCache)

	var (
		// 处理事件到 notifyTarget 关系,处理的notifyTarget用OrMerge进行合并
		handlers []NotifyTargetDispatch

		// 额外去掉一些订阅,处理的notifyTarget用AndMerge进行合并, 如设置 channel=false,合并后不通过这个channel发送
		// 如果实现了相关 Dispatch,可以添加到interceptors中
		interceptorHandlers []NotifyTargetDispatch
	)
	if isSubscribe {
		handlers = []NotifyTargetDispatch{NotifyGroupDispatch, EventCallbacksDispatch}
	} else {
		handlers = []NotifyTargetDispatch{NotifyGroupDispatch, GlobalWebhookDispatch, EventCallbacksDispatch}
	}

	notifyTarget := NewNotifyTarget()
	// 处理订阅关系使用OrMerge
	for _, handler := range handlers {
		notifyTarget.OrMerge(handler(rule, event, notifyTarget, e))
	}

	// 处理移除订阅关系的逻辑,比如员工离职，临时静默某个通道的策略等
	for _, handler := range interceptorHandlers {
		notifyTarget.AndMerge(handler(rule, event, notifyTarget, e))
	}

	// 处理事件发送,这里用一个goroutine处理一个event的所有发送事件
	go e.HandleEventWithNotifyRule(event, isSubscribe)
	go e.Send(rule, event, notifyTarget, isSubscribe)

	// 如果是不是订阅规则出现的event, 则需要处理订阅规则的event
	if !isSubscribe {
		e.handleSubs(event)
	}
}

func (e *Dispatch) blockEventNotify(rule *models.AlertRule, event *models.AlertCurEvent) bool {
	ruleType := rule.GetRuleType()

	// 若为机器则先看机器是否删除
	if ruleType == models.HOST {
		host, ok := e.targetCache.Get(event.TagsMap["ident"])
		if !ok || host == nil {
			return true
		}
	}

	// 恢复通知，检测规则配置是否改变
	// if event.IsRecovered && event.RuleHash != rule.Hash() {
	// 	return true
	// }

	return false
}

func (e *Dispatch) handleSubs(event *models.AlertCurEvent) {
	// handle alert subscribes
	subscribes := make([]*models.AlertSubscribe, 0)
	// rule specific subscribes
	if subs, has := e.alertSubscribeCache.Get(event.RuleId); has {
		subscribes = append(subscribes, subs...)
	}
	// global subscribes
	if subs, has := e.alertSubscribeCache.Get(0); has {
		subscribes = append(subscribes, subs...)
	}

	for _, sub := range subscribes {
		e.handleSub(sub, *event)
	}
}

// handleSub 处理订阅规则的event,注意这里event要使用值传递,因为后面会修改event的状态
func (e *Dispatch) handleSub(sub *models.AlertSubscribe, event models.AlertCurEvent) {
	if sub.IsDisabled() {
		return
	}

	if !sub.MatchCluster(event.DatasourceId) {
		return
	}

	if !sub.MatchProd(event.RuleProd) {
		return
	}

	if !common.MatchTags(event.TagsMap, sub.ITags) {
		return
	}
	// event BusiGroups filter
	if !common.MatchGroupsName(event.GroupName, sub.IBusiGroups) {
		return
	}
	if sub.ForDuration > (event.TriggerTime - event.FirstTriggerTime) {
		return
	}

	if len(sub.SeveritiesJson) != 0 {
		match := false
		for _, s := range sub.SeveritiesJson {
			if s == event.Severity || s == 0 {
				match = true
				break
			}
		}
		if !match {
			return
		}
	}

	e.Astats.CounterSubEventTotal.WithLabelValues(event.GroupName).Inc()
	sub.ModifyEvent(&event)
	event.SubRuleId = sub.Id

	LogEvent(&event, "subscribe")
	e.HandleEventNotify(&event, true)
}

func (e *Dispatch) Send(rule *models.AlertRule, event *models.AlertCurEvent, notifyTarget *NotifyTarget, isSubscribe bool) {
	needSend := e.BeforeSenderHook(event)
	if needSend {
		for channel, uids := range notifyTarget.ToChannelUserMap() {
			msgCtx := sender.BuildMessageContext(e.ctx, rule, []*models.AlertCurEvent{event},
				uids, e.userCache, e.Astats)
			e.RwLock.RLock()
			s := e.Senders[channel]
			e.RwLock.RUnlock()
			if s == nil {
				logger.Debugf("no sender for channel: %s", channel)
				continue
			}

			var event *models.AlertCurEvent
			if len(msgCtx.Events) > 0 {
				event = msgCtx.Events[0]
			}

			logger.Debugf("send to channel:%s event:%+v users:%+v", channel, event, msgCtx.Users)
			s.Send(msgCtx)
		}
	}

	// handle event callbacks
	e.SendCallbacks(rule, notifyTarget, event)

	// handle global webhooks
	if !event.OverrideGlobalWebhook() {
		if e.alerting.WebhookBatchSend {
			sender.BatchSendWebhooks(e.ctx, notifyTarget.ToWebhookMap(), event, e.Astats)
		} else {
			sender.SingleSendWebhooks(e.ctx, notifyTarget.ToWebhookMap(), event, e.Astats)
		}
	}

	// handle plugin call
	go sender.MayPluginNotify(e.ctx, e.genNoticeBytes(event), e.notifyConfigCache.
		GetNotifyScript(), e.Astats, event)

	if !isSubscribe {
		// handle ibex callbacks
		e.HandleIbex(rule, event)
	}
}

func (e *Dispatch) SendCallbacks(rule *models.AlertRule, notifyTarget *NotifyTarget, event *models.AlertCurEvent) {
	uids := notifyTarget.ToUidList()
	urls := notifyTarget.ToCallbackList()
	whMap := notifyTarget.ToWebhookMap()
	ogw := event.OverrideGlobalWebhook()
	for _, urlStr := range urls {
		if len(urlStr) == 0 {
			continue
		}

		cbCtx := sender.BuildCallBackContext(e.ctx, urlStr, rule, []*models.AlertCurEvent{event}, uids, e.userCache, e.alerting.WebhookBatchSend, e.Astats)

		if wh, ok := whMap[cbCtx.CallBackURL]; !ogw && ok && wh.Enable {
			logger.Debugf("SendCallbacks: webhook[%s] is in global conf.", cbCtx.CallBackURL)
			continue
		}

		if strings.HasPrefix(urlStr, "${ibex}") {
			e.CallBacks[models.IbexDomain].CallBack(cbCtx)
			continue
		}

		if !(strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://")) {
			cbCtx.CallBackURL = "http://" + urlStr
		}

		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			logger.Errorf("SendCallbacks: failed to url.Parse(urlStr=%s): %v", urlStr, err)
			continue
		}

		// process feishu card
		if parsedURL.Host == models.FeishuDomain && parsedURL.Query().Get("card") == "1" {
			e.CallBacks[models.FeishuCardDomain].CallBack(cbCtx)
			continue
		}

		// process lark card
		if parsedURL.Host == models.LarkDomain && parsedURL.Query().Get("card") == "1" {
			e.CallBacks[models.LarkCardDomain].CallBack(cbCtx)
			continue
		}

		callBacker, ok := e.CallBacks[parsedURL.Host]
		if ok {
			callBacker.CallBack(cbCtx)
		} else {
			e.CallBacks[models.DefaultDomain].CallBack(cbCtx)
		}
	}
}

func (e *Dispatch) HandleIbex(rule *models.AlertRule, event *models.AlertCurEvent) {
	// 解析 RuleConfig 字段
	var ruleConfig struct {
		TaskTpls []*models.Tpl `json:"task_tpls"`
	}
	json.Unmarshal([]byte(rule.RuleConfig), &ruleConfig)

	for _, t := range ruleConfig.TaskTpls {
		if t.TplId == 0 {
			continue
		}

		if len(t.Host) == 0 {
			sender.CallIbex(e.ctx, t.TplId, event.TargetIdent,
				e.taskTplsCache, e.targetCache, e.userCache, event)
			continue
		}
		for _, host := range t.Host {
			sender.CallIbex(e.ctx, t.TplId, host,
				e.taskTplsCache, e.targetCache, e.userCache, event)
		}
	}
}

type Notice struct {
	Event *models.AlertCurEvent `json:"event"`
	Tpls  map[string]string     `json:"tpls"`
}

func (e *Dispatch) genNoticeBytes(event *models.AlertCurEvent) []byte {
	// build notice body with templates
	ntpls := make(map[string]string)

	e.RwLock.RLock()
	defer e.RwLock.RUnlock()
	for filename, tpl := range e.tpls {
		var body bytes.Buffer
		if err := tpl.Execute(&body, event); err != nil {
			ntpls[filename] = err.Error()
		} else {
			ntpls[filename] = body.String()
		}
	}

	notice := Notice{Event: event, Tpls: ntpls}
	stdinBytes, err := json.Marshal(notice)
	if err != nil {
		logger.Errorf("event_notify: failed to marshal notice: %v", err)
		return nil
	}

	return stdinBytes
}

// for alerting
func fillUsers(ce *models.AlertCurEvent, uc *memsto.UserCacheType, ugc *memsto.UserGroupCacheType) {
	gids := make([]int64, 0, len(ce.NotifyGroupsJSON))
	for i := 0; i < len(ce.NotifyGroupsJSON); i++ {
		gid, err := strconv.ParseInt(ce.NotifyGroupsJSON[i], 10, 64)
		if err != nil {
			continue
		}
		gids = append(gids, gid)
	}

	ce.NotifyGroupsObj = ugc.GetByUserGroupIds(gids)

	uids := make(map[int64]struct{})
	for i := 0; i < len(ce.NotifyGroupsObj); i++ {
		ug := ce.NotifyGroupsObj[i]
		for j := 0; j < len(ug.UserIds); j++ {
			uids[ug.UserIds[j]] = struct{}{}
		}
	}

	ce.NotifyUsersObj = uc.GetByUserIds(mapKeys(uids))
}

func mapKeys(m map[int64]struct{}) []int64 {
	lst := make([]int64, 0, len(m))
	for k := range m {
		lst = append(lst, k)
	}
	return lst
}

func getSendTarget(customParams map[string]string, sendtos []string) string {
	if len(customParams) == 0 {
		return strings.Join(sendtos, ",")
	}

	values := make([]string, 0)
	for _, value := range customParams {
		runes := []rune(value)
		if len(runes) <= 4 {
			values = append(values, value)
		} else {
			maskedValue := string(runes[:len(runes)-4]) + "****"
			values = append(values, maskedValue)
		}
	}

	return strings.Join(values, ",")
}
