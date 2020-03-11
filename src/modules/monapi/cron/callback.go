package cron

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/mcache"
	"github.com/didi/nightingale/src/modules/monapi/redisc"
	"github.com/didi/nightingale/src/toolkits/stats"
)

type CallbackEvent struct {
	Id          int64               `json:"id"`
	Sid         int64               `json:"sid"`
	Sname       string              `json:"sname"`
	NodePath    string              `json:"node_path"`
	Nid         int64               `json:"nid"`
	Endpoint    string              `json:"endpoint"`
	Priority    int                 `json:"priority"`
	EventType   string              `json:"event_type"` // alert|recovery
	Category    int                 `json:"category"`
	Status      uint16              `json:"status"`
	HashId      uint64              `json:"hashid"  xorm:"hashid"`
	Etime       int64               `json:"etime"`
	Value       string              `json:"value"`
	Info        string              `json:"info"`
	LastUpdator string              `json:"last_updator"`
	Created     time.Time           `json:"created" xorm:"created"`
	Groups      []string            `json:"groups"`
	Users       []string            `json:"users"`
	Detail      []model.EventDetail `json:"detail"`
}

func CallbackConsumer() {
	queue := config.Get().Queue.Callback
	for {
		callbackEvent := PopCallbackEvent(queue)
		if callbackEvent == nil {
			time.Sleep(time.Second)
			continue
		}

		go doCallback(callbackEvent)
	}
}

func NeedCallback(sid int64) bool {
	stra, exists := mcache.StraCache.GetById(sid)
	if !exists {
		return false
	}

	if stra.Callback != "" {
		return true
	}
	return false
}

func PushCallbackEvent(event *model.Event) error {
	callbackQueue := config.Get().Queue.Callback

	es, err := json.Marshal(event)
	if err != nil {
		logger.Errorf("callbackEvent marshal to json fail, err: %v", err)
		return err
	}

	rc := redisc.RedisConnPool.Get()
	defer rc.Close()

	if _, err := rc.Do("LPUSH", callbackQueue, string(es)); err != nil {
		logger.Errorf("lpush %+v error: %v", string(es), err)
		return err
	}

	return nil
}

func PopCallbackEvent(queue string) *model.Event {
	rc := redisc.RedisConnPool.Get()
	defer rc.Close()

	ret, err := redis.String(rc.Do("RPOP", queue))
	if err != nil {
		if err != redis.ErrNil {
			logger.Errorf("rpop queue:%s failed, err: %v", queue, err)
		}
		return nil
	}

	if ret == "" || ret == "nil" {
		return nil
	}

	event := new(model.Event)
	if err := json.Unmarshal([]byte(ret), event); err != nil {
		logger.Errorf("unmarshal redis reply fail, err: %v", err)
		return nil
	}

	return event
}

func doCallback(event *model.Event) {
	stra, exists := mcache.StraCache.GetById(event.Sid)
	if !exists {
		logger.Errorf("sid not found, event: %v", event)
		return
	}

	var detail []model.EventDetail

	if err := json.Unmarshal([]byte(event.Detail), &detail); err != nil {
		logger.Errorf("event detail unmarshal failed, err: %v", err)
	}

	users, err := model.UserNameGetByIds(event.Users)
	if err != nil {
		logger.Errorf("username get by id failed, err: %v", err)
	}

	groups, err := model.TeamNameGetsByIds(event.Groups)
	if err != nil {
		logger.Errorf("team name get by id failed, err: %v", err)
	}

	callbackEvent := CallbackEvent{
		Id:          event.Id,
		Sid:         event.Sid,
		Sname:       event.Sname,
		NodePath:    event.NodePath,
		Endpoint:    event.Endpoint,
		Priority:    event.Priority,
		EventType:   event.EventType,
		Category:    event.Category,
		HashId:      event.HashId,
		Etime:       event.Etime,
		Value:       event.Value,
		Info:        event.Info,
		Created:     event.Created,
		Nid:         event.Nid,
		Detail:      detail,
		Groups:      groups,
		Users:       users,
		LastUpdator: stra.LastUpdator,
	}

	url := stra.Callback
	if url == "" {
		return
	}

	if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
		url = "http://" + url
	}

	resp, code, err := httplib.PostJSON(url, 5*time.Second, callbackEvent, map[string]string{})
	if err != nil {
		stats.Counter.Set("callback.err", 1)
		logger.Errorf("callback[%s] fail, callback content: %+v, resp: %s, code: %d, err: %v", url, callbackEvent, string(resp), code, err)
	} else {
		stats.Counter.Set("callback.count", 1)
		logger.Infof("callback[%s] succ, callback content: %+v, resp: %s, code: %d", url, callbackEvent, string(resp), code)
	}
}
