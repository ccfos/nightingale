package engine

import (
	"bytes"
	"encoding/json"
	"html/template"
	"path"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/runner"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/tplx"
	"github.com/didi/nightingale/v5/src/server/common/sender"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

var (
	rwLock sync.RWMutex
	tpls   map[string]*template.Template

	Senders map[string]sender.Sender

	// 处理事件到subscription关系,处理的subscription用OrMerge进行合并
	routers []Router

	// 额外去掉一些订阅,处理的subscription用AndMerge进行合并, 如设置 channel=false,合并后不通过这个channel发送
	// 目前还不需要这块的逻辑，暂时注释掉
	//interceptors []Router

	// 额外的订阅event逻辑处理
	subscribeRouters []Router

	//subcribeInterceptors []Router
)

func initRouters() {
	routers = make([]Router, 0)
	routers = append(routers, GroupRouter)
	routers = append(routers, GlobalWebhookRouter)
	routers = append(routers, EventCallbacksRouter)

	//interceptors = make([]Router, 0)

	subscribeRouters = make([]Router, 0)
	subscribeRouters = append(subscribeRouters, GroupRouter)

	//subcribeInterceptors = make([]Router, 0)
}

func reloadTpls() error {
	if config.C.Alerting.TemplatesDir == "" {
		config.C.Alerting.TemplatesDir = path.Join(runner.Cwd, "etc", "template")
	}

	filenames, err := file.FilesUnder(config.C.Alerting.TemplatesDir)
	if err != nil {
		return errors.WithMessage(err, "failed to exec FilesUnder")
	}

	if len(filenames) == 0 {
		return errors.New("no tpl files under " + config.C.Alerting.TemplatesDir)
	}

	tplFiles := make([]string, 0, len(filenames))
	for i := 0; i < len(filenames); i++ {
		if strings.HasSuffix(filenames[i], ".tpl") {
			tplFiles = append(tplFiles, filenames[i])
		}
	}

	if len(tplFiles) == 0 {
		return errors.New("no tpl files under " + config.C.Alerting.TemplatesDir)
	}

	tmpTpls := make(map[string]*template.Template)
	for _, tplFile := range tplFiles {
		tplpath := path.Join(config.C.Alerting.TemplatesDir, tplFile)
		tpl, err := template.New(tplFile).Funcs(tplx.TemplateFuncMap).ParseFiles(tplpath)
		if err != nil {
			return errors.WithMessage(err, "failed to parse tpl: "+tplpath)
		}
		tmpTpls[tplFile] = tpl
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
	var (
		handlers []Router
		//interceptorHandlers []Router
	)
	fillUsers(event)

	if isSubscribe {
		handlers = subscribeRouters
		//interceptorHandlers = subcribeInterceptors
	} else {
		handlers = routers
		//interceptorHandlers = interceptors
	}

	subscription := NewSubscription()
	// 处理订阅关系
	for _, handler := range handlers {
		subscription.OrMerge(handler(rule, event, subscription))
	}

	// 处理移除订阅关系的逻辑,比如员工离职，临时静默某个通道的策略等
	//for _, handler := range interceptorHandlers {
	//	subscription.AndMerge(handler(rule, event, subscription))
	//}
	Send(rule, event, subscription, isSubscribe)

	// 如果是sub规则出现的event,不用再进行后续的处理
	if isSubscribe {
		return
	}

	// handle alert subscribes
	if subs, has := memsto.AlertSubscribeCache.Get(rule.Id); has {
		for _, sub := range subs {
			handleSub(sub, *event)
		}
	}

	if subs, has := memsto.AlertSubscribeCache.Get(0); has {
		for _, sub := range subs {
			handleSub(sub, *event)
		}
	}
}

func handleSub(sub *models.AlertSubscribe, event models.AlertCurEvent) {
	if sub.IsDisabled() || !sub.MatchCluster(event.Cluster) {
		return
	}
	if !matchTags(event.TagsMap, sub.ITags) {
		return
	}

	if sub.RedefineSeverity == 1 {
		event.Severity = sub.NewSeverity
	}

	if sub.RedefineChannels == 1 {
		event.NotifyChannels = sub.NewChannels
		event.NotifyChannelsJSON = strings.Fields(sub.NewChannels)
	}

	event.NotifyGroups = sub.UserGroupIds
	event.NotifyGroupsJSON = strings.Fields(sub.UserGroupIds)
	if len(event.NotifyGroupsJSON) == 0 {
		return
	}

	LogEvent(&event, "subscribe")
	HandleEventNotify(&event, true)
}

func BuildMessageContext(rule *models.AlertRule, event *models.AlertCurEvent, uids []int64) sender.MessageContext {
	users := memsto.UserCache.GetByUserIds(uids)
	return sender.MessageContext{
		Rule:  rule,
		Event: event,
		Users: users,
	}
}

func Send(rule *models.AlertRule, event *models.AlertCurEvent, subscription *Subscription, isSubscribe bool) {
	for channel, uids := range subscription.ToChannelUserMap() {
		ctx := BuildMessageContext(rule, event, uids)
		rwLock.RLock()
		s := Senders[channel]
		rwLock.RUnlock()
		if s == nil {
			// todo
			continue
		}
		go s.Send(ctx)
	}

	// handle event callbacks
	go sender.SendCallbacks(subscription.ToCallbackList(), event)

	// handle global webhooks
	go sender.SendWebhooks(subscription.ToWebhookList(), event)

	noticeBytes := genNoticeBytes(event)

	// handle plugin call
	go sender.MayPluginNotify(noticeBytes)

	if !isSubscribe {
		// handle redis pub
		go sender.PublishToRedis(event.Cluster, noticeBytes)
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
