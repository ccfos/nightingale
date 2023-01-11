package engine

import (
	"bytes"
	"encoding/json"
	"html/template"
	"sync"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/common/sender"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/toolkits/pkg/logger"
)

var (
	rwLock sync.RWMutex
	tpls   map[string]*template.Template

	Senders map[string]sender.Sender

	// 处理事件到subscription关系,处理的subscription用OrMerge进行合并
	routers []Router

	// 额外去掉一些订阅,处理的subscription用AndMerge进行合并, 如设置 channel=false,合并后不通过这个channel发送
	interceptors []Router

	// 额外的订阅event逻辑处理
	subscribeRouters []Router

	subscribeInterceptors []Router
)

func initRouters() {
	routers = []Router{GroupRouter, GlobalWebhookRouter, EventCallbacksRouter}
	interceptors = []Router{}

	subscribeRouters = []Router{GroupRouter}
	subscribeInterceptors = []Router{}
}

func reloadTpls() error {
	tmpTpls, err := config.C.Alerting.ListTpls()
	if err != nil {
		return err
	}

	senders := map[string]sender.Sender{
		models.Email:    sender.NewSender(models.Email, tmpTpls),
		models.Dingtalk: sender.NewSender(models.Dingtalk, tmpTpls),
		models.Wecom:    sender.NewSender(models.Wecom, tmpTpls),
		models.Feishu:   sender.NewSender(models.Feishu, tmpTpls),
		models.Mm:       sender.NewSender(models.Mm, tmpTpls),
		models.Telegram: sender.NewSender(models.Telegram, tmpTpls),
	}

	rwLock.Lock()
	tpls = tmpTpls
	Senders = senders
	rwLock.Unlock()
	return nil
}

func HandleEventNotify(event *models.AlertCurEvent, isSubscribe bool) {
	rule := memsto.AlertRuleCache.Get(event.RuleId)
	if rule == nil {
		return
	}
	fillUsers(event)

	var (
		handlers            []Router
		interceptorHandlers []Router
	)
	if isSubscribe {
		handlers = subscribeRouters
		interceptorHandlers = subscribeInterceptors
	} else {
		handlers = routers
		interceptorHandlers = interceptors
	}

	subscription := NewSubscription()
	// 处理订阅关系使用OrMerge
	for _, handler := range handlers {
		subscription.OrMerge(handler(rule, event, subscription))
	}

	// 处理移除订阅关系的逻辑,比如员工离职，临时静默某个通道的策略等
	for _, handler := range interceptorHandlers {
		subscription.AndMerge(handler(rule, event, subscription))
	}

	// 处理事件发送,这里用一个goroutine处理一个event的所有发送事件
	go Send(rule, event, subscription, isSubscribe)

	// 如果是sub规则出现的event,不用再进行后续的处理
	if isSubscribe {
		return
	}

	// handle alert subscribes
	subscribes := make([]*models.AlertSubscribe, 0)
	// rule specific subscribes
	if subs, has := memsto.AlertSubscribeCache.Get(rule.Id); has {
		subscribes = append(subscribes, subs...)
	}
	// global subscribes
	if subs, has := memsto.AlertSubscribeCache.Get(0); has {
		subscribes = append(subscribes, subs...)
	}
	for _, sub := range subscribes {
		handleSub(sub, *event)
	}
}

func handleSub(sub *models.AlertSubscribe, event models.AlertCurEvent) {
	if sub.IsDisabled() || !sub.MatchCluster(event.Cluster) {
		return
	}
	if !matchTags(event.TagsMap, sub.ITags) {
		return
	}

	sub.ModifyEvent(&event)
	LogEvent(&event, "subscribe")
	HandleEventNotify(&event, true)
}

func Send(rule *models.AlertRule, event *models.AlertCurEvent, subscription *Subscription, isSubscribe bool) {
	for channel, uids := range subscription.ToChannelUserMap() {
		ctx := sender.BuildMessageContext(rule, event, uids)
		rwLock.RLock()
		s := Senders[channel]
		rwLock.RUnlock()
		if s == nil {
			logger.Warningf("no sender for channel: %s", channel)
			continue
		}
		s.Send(ctx)
	}

	// handle event callbacks
	callbacks := subscription.ToCallbackList()
	if len(callbacks) > 0 {
		sender.SendCallbacks(subscription.ToCallbackList(), event)
	}

	// handle global webhooks
	webhooks := subscription.ToWebhookList()
	if len(webhooks) > 0 {
		sender.SendWebhooks(subscription.ToWebhookList(), event)
	}

	noticeBytes := genNoticeBytes(event)

	// handle plugin call
	go sender.MayPluginNotify(noticeBytes)

	if !isSubscribe {
		// handle redis pub
		sender.PublishToRedis(event.Cluster, noticeBytes)
	}
}

type Notice struct {
	Event *models.AlertCurEvent `json:"event"`
	Tpls  map[string]string     `json:"tpls"`
}

func genNoticeBytes(event *models.AlertCurEvent) []byte {
	// build notice body with templates
	ntpls := make(map[string]string)

	rwLock.RLock()
	defer rwLock.RUnlock()
	for filename, tpl := range tpls {
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
