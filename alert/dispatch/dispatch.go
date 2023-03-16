package dispatch

import (
	"bytes"
	"encoding/json"
	"html/template"
	"strconv"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
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

	alerting aconf.Alerting

	senders map[string]sender.Sender
	tpls    map[string]*template.Template

	ctx *ctx.Context

	RwLock sync.RWMutex
}

// 创建一个 Notify 实例
func NewDispatch(alertRuleCache *memsto.AlertRuleCacheType, userCache *memsto.UserCacheType, userGroupCache *memsto.UserGroupCacheType,
	alertSubscribeCache *memsto.AlertSubscribeCacheType, targetCache *memsto.TargetCacheType, notifyConfigCache *memsto.NotifyConfigCacheType,
	alerting aconf.Alerting, ctx *ctx.Context) *Dispatch {
	notify := &Dispatch{
		alertRuleCache:      alertRuleCache,
		userCache:           userCache,
		userGroupCache:      userGroupCache,
		alertSubscribeCache: alertSubscribeCache,
		targetCache:         targetCache,
		notifyConfigCache:   notifyConfigCache,

		alerting: alerting,

		senders: make(map[string]sender.Sender),
		tpls:    make(map[string]*template.Template),

		ctx: ctx,
	}
	return notify
}

func (e *Dispatch) ReloadTpls() error {
	err := e.relaodTpls()
	if err != nil {
		logger.Error("failed to reload tpls: %v", err)
	}

	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := e.relaodTpls(); err != nil {
			logger.Warning("failed to reload tpls:", err)
		}
	}
}

func (e *Dispatch) relaodTpls() error {
	tmpTpls, err := models.ListTpls(e.ctx)
	if err != nil {
		return err
	}
	smtp := e.notifyConfigCache.GetSMTP()

	senders := map[string]sender.Sender{
		models.Email:    sender.NewSender(models.Email, tmpTpls, smtp),
		models.Dingtalk: sender.NewSender(models.Dingtalk, tmpTpls, smtp),
		models.Wecom:    sender.NewSender(models.Wecom, tmpTpls, smtp),
		models.Feishu:   sender.NewSender(models.Feishu, tmpTpls, smtp),
		models.Mm:       sender.NewSender(models.Mm, tmpTpls, smtp),
		models.Telegram: sender.NewSender(models.Telegram, tmpTpls, smtp),
	}

	e.RwLock.Lock()
	e.tpls = tmpTpls
	e.senders = senders
	e.RwLock.Unlock()
	return nil
}

// HandleEventNotify 处理event事件的主逻辑
// event: 告警/恢复事件
// isSubscribe: 告警事件是否由subscribe的配置产生
func (e *Dispatch) HandleEventNotify(event *models.AlertCurEvent, isSubscribe bool) {
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

	// 处理事件发送,这里用一个goroutine处理一个event的所有发送事件
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
	if sub.IsDisabled() || !sub.MatchCluster(event.DatasourceId) {
		return
	}
	if !common.MatchTags(event.TagsMap, sub.ITags) {
		return
	}
	if sub.ForDuration > (event.TriggerTime - event.FirstTriggerTime) {
		return
	}
	sub.ModifyEvent(&event)
	LogEvent(&event, "subscribe")
	e.HandleEventNotify(&event, true)
}

func (e *Dispatch) Send(rule *models.AlertRule, event *models.AlertCurEvent, notifyTarget *NotifyTarget, isSubscribe bool) {
	for channel, uids := range notifyTarget.ToChannelUserMap() {
		ctx := sender.BuildMessageContext(rule, event, uids, e.userCache)
		e.RwLock.RLock()
		s := e.senders[channel]
		e.RwLock.RUnlock()
		if s == nil {
			logger.Warningf("no sender for channel: %s", channel)
			continue
		}
		logger.Debugf("send event: %s, channel: %s", event.Hash, channel)
		for i := 0; i < len(ctx.Users); i++ {
			logger.Debug("send event to user: ", ctx.Users[i])
		}
		s.Send(ctx)
	}

	// handle event callbacks
	sender.SendCallbacks(e.ctx, notifyTarget.ToCallbackList(), event, e.targetCache, e.notifyConfigCache.GetIbex())

	// handle global webhooks
	sender.SendWebhooks(notifyTarget.ToWebhookList(), event)

	// handle plugin call
	go sender.MayPluginNotify(e.genNoticeBytes(event), e.notifyConfigCache.GetNotifyScript())
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
