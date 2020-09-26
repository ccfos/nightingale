package alarm

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/acache"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/redisc"

	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

type CallbackEvent struct {
	Id          int64                `json:"id"`
	Sid         int64                `json:"sid"`
	Sname       string               `json:"sname"`
	NodePath    string               `json:"node_path"`
	Nid         int64                `json:"nid"`
	Endpoint    string               `json:"endpoint"`
	Priority    int                  `json:"priority"`
	EventType   string               `json:"event_type"` // alert|recovery
	Category    int                  `json:"category"`
	Status      uint16               `json:"status"`
	HashId      uint64               `json:"hashid"  xorm:"hashid"`
	Etime       int64                `json:"etime"`
	Value       string               `json:"value"`
	Info        string               `json:"info"`
	LastUpdator string               `json:"last_updator"`
	Created     time.Time            `json:"created" xorm:"created"`
	Groups      []string             `json:"groups"`
	Users       []string             `json:"users"`
	Detail      []models.EventDetail `json:"detail"`
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
	stra, exists := acache.StraCache.GetById(sid)
	if !exists {
		return false
	}

	if stra.Callback != "" {
		return true
	}
	return false
}

func PushCallbackEvent(event *models.Event) error {
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

func PopCallbackEvent(queue string) *models.Event {
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

	event := new(models.Event)
	if err := json.Unmarshal([]byte(ret), event); err != nil {
		logger.Errorf("unmarshal redis reply fail, err: %v", err)
		return nil
	}

	return event
}

func doCallback(event *models.Event) {
	stra, exists := acache.StraCache.GetById(event.Sid)
	if !exists {
		logger.Errorf("sid not found, event: %v", event)
		return
	}

	detail := []models.EventDetail{}

	if err := json.Unmarshal([]byte(event.Detail), &detail); err != nil {
		logger.Errorf("event detail unmarshal failed, err: %v", err)
	}

	userIds := []int64{}
	if err := json.Unmarshal([]byte(event.Users), &userIds); err != nil {
		logger.Errorf("event detail unmarshal event.users, err: %v", err)
	}

	users, err := models.UserGetByIds(userIds)
	if err != nil {
		logger.Errorf("get users err: %v", err)
	}

	usernames := []string{}
	for _, user := range users {
		usernames = append(usernames, user.Username)
	}

	idsStrArr := strings.Split(event.Groups, ",")
	teamIds := []int64{}
	for _, tid := range idsStrArr {
		id, _ := strconv.ParseInt(tid, 10, 64)
		teamIds = append(teamIds, id)
	}

	teams, err := models.TeamGetByIds(teamIds)
	if err != nil {
		logger.Errorf("get teams err: %v", err)
	}

	groups := []string{}
	for _, team := range teams {
		groups = append(groups, team.Name)
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
		Users:       usernames,
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
		logger.Errorf("callback[%s] fail, callback content: %+v, resp: %s, err: %v, code:%d", url, callbackEvent, string(resp), err, code)
	} else {
		logger.Infof("callback[%s] succ, callback content: %+v, resp: %s, code:%d", url, callbackEvent, string(resp), code)
	}
}
