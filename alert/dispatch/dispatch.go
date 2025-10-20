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
	"github.com/ccfos/nightingale/v6/alert/pipeline"
	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

var ShouldSkipNotify func(*ctx.Context, *models.AlertCurEvent, int64) bool
var SendByNotifyRule func(*ctx.Context, *memsto.UserCacheType, *memsto.UserGroupCacheType, *memsto.NotifyChannelCacheType,
	[]*models.AlertCurEvent, int64, *models.NotifyConfig, *models.NotifyChannelConfig, *models.MessageTemplate)

func init() {
	ShouldSkipNotify = shouldSkipNotify
	SendByNotifyRule = SendNotifyRuleMessage
}

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
	eventProcessorCache  *memsto.EventProcessorCacheType

	alerting aconf.Alerting

	Senders          map[string]sender.Sender
	CallBacks        map[string]sender.CallBacker
	tpls             map[string]*template.Template
	ExtraSenders     map[string]sender.Sender
	BeforeSenderHook func(*models.AlertCurEvent) bool
	ctx              *ctx.Context
	Astats           *astats.Stats

	RwLock sync.RWMutex
}

// 创建一个 Notify 实例
func NewDispatch(alertRuleCache *memsto.AlertRuleCacheType, userCache *memsto.UserCacheType, userGroupCache *memsto.UserGroupCacheType,
	alertSubscribeCache *memsto.AlertSubscribeCacheType, targetCache *memsto.TargetCacheType, notifyConfigCache *memsto.NotifyConfigCacheType,
	taskTplsCache *memsto.TaskTplCache, notifyRuleCache *memsto.NotifyRuleCacheType, notifyChannelCache *memsto.NotifyChannelCacheType,
	messageTemplateCache *memsto.MessageTemplateCacheType, eventProcessorCache *memsto.EventProcessorCacheType, alerting aconf.Alerting, c *ctx.Context, astats *astats.Stats) *Dispatch {
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
		eventProcessorCache:  eventProcessorCache,

		alerting: alerting,

		Senders:          make(map[string]sender.Sender),
		tpls:             make(map[string]*template.Template),
		ExtraSenders:     make(map[string]sender.Sender),
		BeforeSenderHook: func(*models.AlertCurEvent) bool { return true },

		ctx:    c,
		Astats: astats,
	}

	pipeline.Init()

	// 设置通知记录回调函数
	notifyChannelCache.SetNotifyRecordFunc(sender.NotifyRecord)

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

func (e *Dispatch) HandleEventWithNotifyRule(eventOrigin *models.AlertCurEvent) {

	if len(eventOrigin.NotifyRuleIds) > 0 {
		for _, notifyRuleId := range eventOrigin.NotifyRuleIds {
			// 深拷贝新的 event，避免并发修改 event 冲突
			eventCopy := eventOrigin.DeepCopy()

			logger.Infof("notify rule ids: %v, event: %+v", notifyRuleId, eventCopy)
			notifyRule := e.notifyRuleCache.Get(notifyRuleId)
			if notifyRule == nil {
				continue
			}

			if !notifyRule.Enable {
				continue
			}
			eventCopy.NotifyRuleId = notifyRuleId
			eventCopy.NotifyRuleName = notifyRule.Name

			var processors []models.Processor
			for _, pipelineConfig := range notifyRule.PipelineConfigs {
				if !pipelineConfig.Enable {
					continue
				}

				eventPipeline := e.eventProcessorCache.Get(pipelineConfig.PipelineId)
				if eventPipeline == nil {
					logger.Warningf("notify_id: %d, event:%+v, processor not found", notifyRuleId, eventCopy)
					continue
				}

				if !pipelineApplicable(eventPipeline, eventCopy) {
					logger.Debugf("notify_id: %d, event:%+v, pipeline_id: %d, not applicable", notifyRuleId, eventCopy, pipelineConfig.PipelineId)
					continue
				}

				processors = append(processors, e.eventProcessorCache.GetProcessorsById(pipelineConfig.PipelineId)...)
			}

			for _, processor := range processors {
				var res string
				var err error
				logger.Infof("before processor notify_id: %d, event:%+v, processor:%+v", notifyRuleId, eventCopy, processor)
				eventCopy, res, err = processor.Process(e.ctx, eventCopy)
				if eventCopy == nil {
					logger.Warningf("after processor notify_id: %d, event:%+v, processor:%+v, event is nil", notifyRuleId, eventCopy, processor)
					sender.NotifyRecord(e.ctx, []*models.AlertCurEvent{eventOrigin}, notifyRuleId, "", "", res, errors.New("drop by processor"))
					break
				}
				logger.Infof("after processor notify_id: %d, event:%+v, processor:%+v, res:%v, err:%v", notifyRuleId, eventCopy, processor, res, err)
			}

			if ShouldSkipNotify(e.ctx, eventCopy, notifyRuleId) {
				logger.Infof("notify_id: %d, event:%+v, should skip notify", notifyRuleId, eventCopy)
				continue
			}

			var defaultCfg *models.NotifyConfig
			if len(notifyRule.NotifyConfigs) > 0 {
				lastCfg := &notifyRule.NotifyConfigs[len(notifyRule.NotifyConfigs)-1]
				if lastCfg.IsDefault {
					defaultCfg = lastCfg
				}
			}

			matched := false

		for i := range notifyRule.NotifyConfigs {
			cfg := &notifyRule.NotifyConfigs[i]
			if cfg == defaultCfg {
				continue
			}

			if err := NotifyRuleMatchCheck(cfg, eventCopy); err != nil {
				logger.Infof("notify_id: %d, event:%+v, channel_id:%d, template_id: %d, notify_config:%+v, match skipped: %v", notifyRuleId, eventCopy, cfg.ChannelID, cfg.TemplateID, cfg, err)
				continue
			}

			if e.notifyByConfig(eventCopy, notifyRuleId, cfg) {
				matched = true
			}
		}

		if !matched && defaultCfg != nil {
			if e.notifyByConfig(eventCopy, notifyRuleId, defaultCfg) {
				matched = true
			}
		}

		if !matched {
			logger.Errorf("notify_id: %d, event:%+v, no notify_config matched", notifyRuleId, eventCopy)
		}
		}
	}
}

func (e *Dispatch) notifyByConfig(event *models.AlertCurEvent, notifyRuleId int64, cfg *models.NotifyConfig) bool {
	notifyChannel := e.notifyChannelCache.Get(cfg.ChannelID)
	messageTemplate := e.messageTemplateCache.Get(cfg.TemplateID)
	if notifyChannel == nil {
		sender.NotifyRecord(e.ctx, []*models.AlertCurEvent{event}, notifyRuleId, fmt.Sprintf("notify_channel_id:%d", cfg.ChannelID), "", "", errors.New("notify_channel not found"))
		logger.Warningf("notify_id: %d, event:%+v, channel_id:%d, template_id: %d, notify_channel not found", notifyRuleId, event, cfg.ChannelID, cfg.TemplateID)
		return false
	}

	if notifyChannel.RequestType != "flashduty" && messageTemplate == nil {
		logger.Warningf("notify_id: %d, channel_name: %v, event:%+v, template_id: %d, message_template not found", notifyRuleId, notifyChannel.Ident, event, cfg.TemplateID)
		sender.NotifyRecord(e.ctx, []*models.AlertCurEvent{event}, notifyRuleId, notifyChannel.Name, "", "", errors.New("message_template not found"))
		return false
	}

	go SendByNotifyRule(e.ctx, e.userCache, e.userGroupCache, e.notifyChannelCache, []*models.AlertCurEvent{event}, notifyRuleId, cfg, notifyChannel, messageTemplate)
	return true
}

func shouldSkipNotify(ctx *ctx.Context, event *models.AlertCurEvent, notifyRuleId int64) bool {
	if event == nil {
		// 如果 eventCopy 为 nil，说明 eventCopy 被 processor drop 掉了, 不再发送通知
		return true
	}

	if event.IsRecovered && event.NotifyRecovered == 0 {
		// 如果 eventCopy 是恢复事件，且 NotifyRecovered 为 0，则不发送通知
		return true
	}
	return false
}

func pipelineApplicable(pipeline *models.EventPipeline, event *models.AlertCurEvent) bool {
	if pipeline == nil {
		return true
	}

	if !pipeline.FilterEnable {
		return true
	}

	tagMatch := true
	if len(pipeline.LabelFilters) > 0 {
		for i := range pipeline.LabelFilters {
			if pipeline.LabelFilters[i].Func == "" {
				pipeline.LabelFilters[i].Func = pipeline.LabelFilters[i].Op
			}
		}

		tagFilters, err := models.ParseTagFilter(pipeline.LabelFilters)
		if err != nil {
			logger.Errorf("pipeline applicable failed to parse tag filter: %v event:%+v pipeline:%+v", err, event, pipeline)
			return false
		}
		tagMatch = common.MatchTags(event.TagsMap, tagFilters)
	}

	attributesMatch := true
	if len(pipeline.AttrFilters) > 0 {
		tagFilters, err := models.ParseTagFilter(pipeline.AttrFilters)
		if err != nil {
			logger.Errorf("pipeline applicable failed to parse tag filter: %v event:%+v pipeline:%+v err:%v", tagFilters, event, pipeline, err)
			return false
		}

		attributesMatch = common.MatchTags(event.JsonTagsAndValue(), tagFilters)
	}

	return tagMatch && attributesMatch
}

func NotifyRuleMatchCheck(notifyConfig *models.NotifyConfig, event *models.AlertCurEvent) error {
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

	if !timeMatch {
		return fmt.Errorf("event time not match time filter")
	}

	severityMatch := false
	for i := range notifyConfig.Severities {
		if notifyConfig.Severities[i] == event.Severity {
			severityMatch = true
		}
	}

	if !severityMatch {
		return fmt.Errorf("event severity not match severity filter")
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
			return fmt.Errorf("failed to parse tag filter: %v", err)
		}
		tagMatch = common.MatchTags(event.TagsMap, tagFilters)
	}

	if !tagMatch {
		return fmt.Errorf("event tag not match tag filter")
	}

	attributesMatch := true
	if len(notifyConfig.Attributes) > 0 {
		tagFilters, err := models.ParseTagFilter(notifyConfig.Attributes)
		if err != nil {
			logger.Errorf("notify send failed to parse tag filter: %v event:%+v notify_config:%+v err:%v", tagFilters, event, notifyConfig, err)
			return fmt.Errorf("failed to parse tag filter: %v", err)
		}

		attributesMatch = common.MatchTags(event.JsonTagsAndValue(), tagFilters)
	}

	if !attributesMatch {
		return fmt.Errorf("event attributes not match attributes filter")
	}

	logger.Infof("notify send timeMatch:%v severityMatch:%v tagMatch:%v attributesMatch:%v event:%+v notify_config:%+v", timeMatch, severityMatch, tagMatch, attributesMatch, event, notifyConfig)
	return nil
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

func SendNotifyRuleMessage(ctx *ctx.Context, userCache *memsto.UserCacheType, userGroupCache *memsto.UserGroupCacheType, notifyChannelCache *memsto.NotifyChannelCacheType,
	events []*models.AlertCurEvent, notifyRuleId int64, notifyConfig *models.NotifyConfig, notifyChannel *models.NotifyChannelConfig, messageTemplate *models.MessageTemplate) {
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

	sendtos, flashDutyChannelIDs, customParams := GetNotifyConfigParams(notifyConfig, contactKey, userCache, userGroupCache)

	switch notifyChannel.RequestType {
	case "flashduty":
		if len(flashDutyChannelIDs) == 0 {
			flashDutyChannelIDs = []int64{0} // 如果 flashduty 通道没有配置，则使用 0, 给 SendFlashDuty 判断使用, 不给 flashduty 传 channel_id 参数
		}

		for i := range flashDutyChannelIDs {
			start := time.Now()
			respBody, err := notifyChannel.SendFlashDuty(events, flashDutyChannelIDs[i], notifyChannelCache.GetHttpClient(notifyChannel.ID))
			respBody = fmt.Sprintf("duration: %d ms %s", time.Since(start).Milliseconds(), respBody)
			logger.Infof("notify_id: %d, channel_name: %v, event:%+v, IntegrationUrl: %v dutychannel_id: %v, respBody: %v, err: %v", notifyRuleId, notifyChannel.Name, events[0], notifyChannel.RequestConfig.FlashDutyRequestConfig.IntegrationUrl, flashDutyChannelIDs[i], respBody, err)
			sender.NotifyRecord(ctx, events, notifyRuleId, notifyChannel.Name, strconv.FormatInt(flashDutyChannelIDs[i], 10), respBody, err)
		}

	case "http":
		// 使用队列模式处理 http 通知
		// 创建通知任务
		task := &memsto.NotifyTask{
			Events:        events,
			NotifyRuleId:  notifyRuleId,
			NotifyChannel: notifyChannel,
			TplContent:    tplContent,
			CustomParams:  customParams,
			Sendtos:       sendtos,
		}

		// 将任务加入队列
		success := notifyChannelCache.EnqueueNotifyTask(task)
		if !success {
			logger.Errorf("failed to enqueue notify task for channel %d, notify_id: %d", notifyChannel.ID, notifyRuleId)
			// 如果入队失败，记录错误通知
			sender.NotifyRecord(ctx, events, notifyRuleId, notifyChannel.Name, getSendTarget(customParams, sendtos), "", errors.New("failed to enqueue notify task, queue is full"))
		}

	case "smtp":
		notifyChannel.SendEmail(notifyRuleId, events, tplContent, sendtos, notifyChannelCache.GetSmtpClient(notifyChannel.ID))

	case "script":
		start := time.Now()
		target, res, err := notifyChannel.SendScript(events, tplContent, customParams, sendtos)
		res = fmt.Sprintf("duration: %d ms %s", time.Since(start).Milliseconds(), res)
		logger.Infof("notify_id: %d, channel_name: %v, event:%+v, tplContent:%s, customParams:%v, target:%s, res:%s, err:%v", notifyRuleId, notifyChannel.Name, events[0], tplContent, customParams, target, res, err)
		sender.NotifyRecord(ctx, events, notifyRuleId, notifyChannel.Name, target, res, err)
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
	go e.HandleEventWithNotifyRule(event)
	if event.IsRecovered && event.NotifyRecovered == 0 {
		return
	}

	rule := e.alertRuleCache.Get(event.RuleId)
	if rule == nil {
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

	go e.Send(rule, event, notifyTarget, isSubscribe)

	// 如果是不是订阅规则出现的event, 则需要处理订阅规则的event
	if !isSubscribe {
		e.handleSubs(event)
	}
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

	if !sub.MatchCate(event.Cate) {
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

	if event.IsRecovered {
		// 恢复事件不需要走故障自愈的逻辑
		return
	}

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
