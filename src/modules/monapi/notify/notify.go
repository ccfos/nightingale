package notify

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/redisc"
	"github.com/didi/nightingale/src/toolkits/stats"
)

type Message struct {
	Tos              []string     `json:"tos"`
	Event            *model.Event `json:"event"`
	ClaimLink        string       `json:"claim_link"`
	StraLink         string       `json:"stra_link"`
	EventLink        string       `json:"event_link"`
	Bindings         []string     `json:"bindings"`
	NotifyType       string       `json:"notify_type"`
	Metrics          []string     `json:"metrics"`
	ReadableEndpoint string       `json:"readable_endpoint"`
	ReadableTags     string       `json:"readable_tags"`
	IsUpgrade        bool         `json:"is_upgrade"`
}

func genClaimLink(event *model.Event) string {
	eventCur, err := model.EventCurGet("hashid", event.HashId)
	if err != nil {
		logger.Errorf("get event_cur failed, err: %v, event: %+v", err, event)
		return ""
	}

	if eventCur == nil {
		return ""
	}

	return fmt.Sprintf(config.Get().Link.Claim, eventCur.Id)
}

func genStraLink(event *model.Event) string {
	return fmt.Sprintf(config.Get().Link.Stra, event.Sid)
}

func genEventLink(event *model.Event) string {
	return fmt.Sprintf(config.Get().Link.Event, event.Id)
}

func genBindings(event *model.Event) []string {
	return model.EndpointBindingsForMail([]string{event.Endpoint})
}

func genMetrics(event *model.Event) []string {
	var metricList []string
	detail, err := event.GetEventDetail()
	if err != nil {
		logger.Errorf("[genMetric] get event detail failed, event: %+v, err: %v", event, err)
	} else {
		for i := 0; i < len(detail); i++ {
			metricList = append(metricList, detail[i].Metric)
		}
	}
	return metricList
}

func genTags(event *model.Event) string {
	tagsMap := make(map[string][]string)
	detail, err := event.GetEventDetail()
	if err != nil {
		return ""
	}

	for k, v := range detail[0].Tags {
		if !config.InSlice(v, tagsMap[k]) {
			tagsMap[k] = append(tagsMap[k], v)
		}
	}

	var tagsList []string
	for k, v := range tagsMap {
		valueString := strings.Join(v, ",")
		if len(v) > 1 {
			valueString = "[" + valueString + "]"
		}
		tagsList = append(tagsList, fmt.Sprintf("%s=%s", k, valueString))
	}

	return strings.Join(tagsList, ",")
}

func genEndpoint(event *model.Event) string {
	return fmt.Sprintf("%s(%s)", event.Endpoint, event.EndpointAlias)
}

// DoNotify 除了原始event信息之外，再附加一些通过查库才能得到的信息交给下游处理
func DoNotify(event *model.Event) {
	if event == nil {
		return
	}

	message := Message{
		Event:            event,
		ClaimLink:        genClaimLink(event),
		StraLink:         genStraLink(event),
		EventLink:        genEventLink(event),
		Bindings:         genBindings(event),
		Metrics:          genMetrics(event),
		ReadableTags:     genTags(event),
		ReadableEndpoint: genEndpoint(event),
		IsUpgrade:        event.RealUpgrade,
	}

	notifyTypes := config.Get().Notify[fmt.Sprintf("p%v", event.Priority)]
	users := event.RecvUserObjs

	for i := 0; i < len(notifyTypes); i++ {
		switch notifyTypes[i] {
		case "voice":
			var tos []string
			for j := 0; j < len(users); j++ {
				tos = append(tos, users[j].Phone)
			}

			message.Tos = tos
			message.NotifyType = "voice"
			send(message)
		case "sms":
			var tos []string
			for j := 0; j < len(users); j++ {
				tos = append(tos, users[j].Phone)
			}

			message.Tos = tos
			message.NotifyType = "sms"
			send(message)
		case "mail":
			var tos []string
			for j := 0; j < len(users); j++ {
				tos = append(tos, users[j].Email)
			}

			message.Tos = tos
			message.NotifyType = "mail"
			send(message)
		case "im":
			var tos []string
			for j := 0; j < len(users); j++ {
				tos = append(tos, users[j].Im)
			}

			message.Tos = tos
			message.NotifyType = "im"
			send(message)
		default:
			logger.Errorf("not support %s to send notify, event: %+v", notifyTypes[i], event)
		}
	}
}

const notifyMessageLoggerFormat = "--->>> metric: %s, endpoint: %s, type: %s, tags: %s"

// alarm只需要把告警事件整理好，一块推送给redis，后续由各个sender来处理
func send(message Message) {
	bs, err := json.Marshal(message)
	if err != nil {
		logger.Error("json.marshal Message fail: ", err)
		return
	}

	payload := string(bs)
	logger.Debugf(
		notifyMessageLoggerFormat,
		strings.Join(message.Metrics, ","),
		message.ReadableEndpoint,
		message.NotifyType,
		message.ReadableTags)

	queue := config.Get().Queue.SenderPrefix + message.NotifyType

	rc := redisc.RedisConnPool.Get()
	defer rc.Close()

	stats.Counter.Set("redis.push", 1)
	if _, err := rc.Do("LPUSH", queue, payload); err != nil {
		logger.Errorf("LPUSH %s error: %v", queue, err)
		stats.Counter.Set("redis.push.err", 1)
	}
}
