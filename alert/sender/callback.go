package sender

import (
	"html/template"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

type (
	// CallBacker 进行回调的接口
	CallBacker interface {
		CallBack(ctx CallBackContext)
	}

	// CallBackContext 回调时所需的上下文
	CallBackContext struct {
		Ctx         *ctx.Context
		CallBackURL string
		Users       []*models.User
		Rule        *models.AlertRule
		Events      []*models.AlertCurEvent
		Stats       *astats.Stats
	}

	DefaultCallBacker struct{}
)

func BuildCallBackContext(ctx *ctx.Context, callBackURL string, rule *models.AlertRule, events []*models.AlertCurEvent,
	uids []int64, userCache *memsto.UserCacheType, stats *astats.Stats) CallBackContext {
	users := userCache.GetByUserIds(uids)

	newCallBackUrl, _ := events[0].ParseURL(callBackURL)
	return CallBackContext{
		Ctx:         ctx,
		CallBackURL: newCallBackUrl,
		Rule:        rule,
		Events:      events,
		Users:       users,
		Stats:       stats,
	}
}

func ExtractAtsParams(rawURL string) []string {
	ans := make([]string, 0, 1)
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		logger.Errorf("ExtractAtsParams(url=%s), err: %v", rawURL, err)
		return ans
	}

	queryParams := parsedURL.Query()
	atParam := queryParams.Get("ats")
	if atParam == "" {
		return ans
	}

	// Split the atParam by comma and return the result as a slice
	return strings.Split(atParam, ",")
}

func NewCallBacker(
	key string,
	targetCache *memsto.TargetCacheType,
	userCache *memsto.UserCacheType,
	taskTplCache *memsto.TaskTplCache,
	tpls map[string]*template.Template,
) CallBacker {

	switch key {
	case models.IbexDomain: // Distribute to Ibex
		return &IbexCallBacker{
			targetCache:  targetCache,
			userCache:    userCache,
			taskTplCache: taskTplCache,
		}
	case models.DefaultDomain: // default callback
		return &DefaultCallBacker{}
	case models.DingtalkDomain:
		return &DingtalkSender{tpl: tpls[models.Dingtalk]}
	case models.WecomDomain:
		return &WecomSender{tpl: tpls[models.Wecom]}
	case models.FeishuDomain:
		return &FeishuSender{tpl: tpls[models.Feishu]}
	case models.FeishuCardDomain:
		return &FeishuCardSender{tpl: tpls[models.FeishuCard]}
	//case models.Mm:
	//	return &MmSender{tpl: tpls[models.Mm]}
	case models.TelegramDomain:
		return &TelegramSender{tpl: tpls[models.Telegram]}
	}

	return nil
}

func (c *DefaultCallBacker) CallBack(ctx CallBackContext) {
	if len(ctx.CallBackURL) == 0 || len(ctx.Events) == 0 {
		return
	}

	event := ctx.Events[0]

	ctx.Stats.AlertNotifyTotal.WithLabelValues("rule_callback").Inc()
	resp, code, err := poster.PostJSON(ctx.CallBackURL, 5*time.Second, event, 3)
	if err != nil {
		logger.Errorf("event_callback_fail(rule_id=%d url=%s), event:%+v, resp: %s, err: %v, code: %d",
			event.RuleId, ctx.CallBackURL, event, string(resp), err, code)
		ctx.Stats.AlertNotifyErrorTotal.WithLabelValues("rule_callback").Inc()
	} else {
		logger.Infof("event_callback_succ(rule_id=%d url=%s), event:%+v, resp: %s, code: %d",
			event.RuleId, ctx.CallBackURL, event, string(resp), code)
	}
}

func doSendAndRecord(ctx *ctx.Context, url, token string, body interface{}, channel string,
	stats *astats.Stats, event *models.AlertCurEvent) {
	res, err := doSend(url, body, channel, stats)
	doRecord(ctx, event, channel, token, res, err)
}

func doRecord(ctx *ctx.Context, evt *models.AlertCurEvent, channel, target, res string, err error) {
	noti := models.NewNotificationRecord(evt, channel, target)
	if err != nil {
		noti.SetStatus(models.NotiStatusFailure)
		noti.SetDetails(err.Error())
	} else if res != "" {
		noti.SetDetails(string(res))
	}

	if err := noti.Add(ctx); err != nil {
		logger.Errorf("Add noti failed, err: %v", err)
	}
}

func doSend(url string, body interface{}, channel string, stats *astats.Stats) (string, error) {
	stats.AlertNotifyTotal.WithLabelValues(channel).Inc()

	res, code, err := poster.PostJSON(url, time.Second*5, body, 3)
	if err != nil {
		logger.Errorf("%s_sender: result=fail url=%s code=%d error=%v req:%v response=%s", channel, url, code, err, body, string(res))
		stats.AlertNotifyErrorTotal.WithLabelValues(channel).Inc()
		return "", err
	}

	logger.Infof("%s_sender: result=succ url=%s code=%d req:%v response=%s", channel, url, code, body, string(res))
	return string(res), nil
}

type TaskCreateReply struct {
	Err string `json:"err"`
	Dat int64  `json:"dat"` // task.id
}
