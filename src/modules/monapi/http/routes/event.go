package routes

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/toolkits/pkg/errors"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/model"
	jsoniter "github.com/json-iterator/go"
)

type eventData struct {
	Id           int64               `json:"id"`
	Sid          int64               `json:"sid"`
	Sname        string              `json:"sname"`
	NodePath     string              `json:"node_path"`
	Nid          int64               `json:"nid"`
	Endpoint     string              `json:"endpoint"`
	Priority     int                 `json:"priority"`
	EventType    string              `json:"event_type"` // alert|recovery
	Category     int                 `json:"category"`
	HashId       uint64              `json:"hashid"  xorm:"hashid"`
	Etime        int64               `json:"etime"`
	Value        string              `json:"value"`
	Info         string              `json:"info"`
	Tags         string              `json:"tags"`
	Created      time.Time           `json:"created" xorm:"created"`
	Detail       []model.EventDetail `json:"detail"`
	Users        []string            `json:"users"`
	Groups       []string            `json:"groups"`
	Status       []string            `json:"status"`
	Claimants    []string            `json:"claimants,omitempty"`
	NeedUpgrade  int                 `json:"need_upgrade"`
	AlertUpgrade AlertUpgrade        `json:"alert_upgrade"`
}

type AlertUpgrade struct {
	Users    []string `json:"users"`
	Groups   []string `json:"groups"`
	Duration int      `json:"duration"`
	Level    int      `json:"level"`
}

func eventCurGets(c *gin.Context) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	stime := mustQueryInt64(c, "stime")
	etime := mustQueryInt64(c, "etime")
	nodePath := mustQueryStr(c, "nodepath")

	limit := queryInt(c, "limit", 20)

	priorities := queryStr(c, "priorities", "")
	sendtypes := queryStr(c, "sendtypes", "")
	query := queryStr(c, "query", "")

	total, err := model.EventCurTotal(stime, etime, nodePath, query, strings.Split(priorities, ","), strings.Split(sendtypes, ","))
	errors.Dangerous(err)

	events, err := model.EventCurGets(stime, etime, nodePath, query, strings.Split(priorities, ","), strings.Split(sendtypes, ","), limit, offset(c, limit, total))
	errors.Dangerous(err)

	datList := []eventData{}
	for i := 0; i < len(events); i++ {
		users, err := model.UserNameGetByIds(events[i].Users)
		errors.Dangerous(err)

		groups, err := model.TeamNameGetsByIds(events[i].Groups)
		errors.Dangerous(err)

		claimants, err := model.UserNameGetByIds(events[i].Claimants)
		errors.Dangerous(err)

		var detail []model.EventDetail
		err = json.Unmarshal([]byte(events[i].Detail), &detail)
		errors.Dangerous(err)

		var tags string
		if len(detail) > 0 {
			tags = dataobj.SortedTags(detail[0].Tags)
		}

		alertUpgrade, err := model.EventAlertUpgradeUnMarshal(events[i].AlertUpgrade)
		errors.Dangerous(err)

		alertUsers, err := model.UserNameGetByIds(alertUpgrade.Users)
		errors.Dangerous(err)

		alertGroups, err := model.TeamNameGetsByIds(alertUpgrade.Groups)
		errors.Dangerous(err)

		dat := eventData{
			Id:          events[i].Id,
			Sid:         events[i].Sid,
			Sname:       events[i].Sname,
			NodePath:    events[i].NodePath,
			Endpoint:    events[i].Endpoint,
			Priority:    events[i].Priority,
			EventType:   events[i].EventType,
			Category:    events[i].Category,
			HashId:      events[i].HashId,
			Etime:       events[i].Etime,
			Value:       events[i].Value,
			Info:        events[i].Info,
			Tags:        tags,
			Created:     events[i].Created,
			Nid:         events[i].Nid,
			Users:       users,
			Groups:      groups,
			Detail:      detail,
			Status:      model.StatusConvert(model.GetStatusByFlag(events[i].Status)),
			Claimants:   claimants,
			NeedUpgrade: events[i].NeedUpgrade,
			AlertUpgrade: AlertUpgrade{
				Groups:   alertGroups,
				Users:    alertUsers,
				Duration: alertUpgrade.Duration,
				Level:    alertUpgrade.Level,
			},
		}

		datList = append(datList, dat)
	}

	renderData(c, map[string]interface{}{
		"total": total,
		"list":  datList,
	}, nil)
}

func eventHisGets(c *gin.Context) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	stime := mustQueryInt64(c, "stime")
	etime := mustQueryInt64(c, "etime")
	nodePath := mustQueryStr(c, "nodepath")

	limit := queryInt(c, "limit", 20)

	priorities := queryStr(c, "priorities", "")
	sendtypes := queryStr(c, "sendtypes", "")
	query := queryStr(c, "query", "")
	eventType := queryStr(c, "type", "")

	total, err := model.EventTotal(stime, etime, nodePath, query, eventType, strings.Split(priorities, ","), strings.Split(sendtypes, ","))
	errors.Dangerous(err)

	events, err := model.EventGets(stime, etime, nodePath, query, eventType, strings.Split(priorities, ","), strings.Split(sendtypes, ","), limit, offset(c, limit, total))
	errors.Dangerous(err)

	datList := []eventData{}
	for i := 0; i < len(events); i++ {
		users, err := model.UserNameGetByIds(events[i].Users)
		errors.Dangerous(err)

		groups, err := model.TeamNameGetsByIds(events[i].Groups)
		errors.Dangerous(err)

		var detail []model.EventDetail
		err = json.Unmarshal([]byte(events[i].Detail), &detail)
		errors.Dangerous(err)

		var tags string
		if len(detail) > 0 {
			tags = dataobj.SortedTags(detail[0].Tags)
		}

		alertUpgrade, err := model.EventAlertUpgradeUnMarshal(events[i].AlertUpgrade)
		errors.Dangerous(err)

		alertUsers, err := model.UserNameGetByIds(alertUpgrade.Users)
		errors.Dangerous(err)

		alertGroups, err := model.TeamNameGetsByIds(alertUpgrade.Groups)
		errors.Dangerous(err)

		dat := eventData{
			Id:          events[i].Id,
			Sid:         events[i].Sid,
			Sname:       events[i].Sname,
			NodePath:    events[i].NodePath,
			Endpoint:    events[i].Endpoint,
			Priority:    events[i].Priority,
			EventType:   events[i].EventType,
			Category:    events[i].Category,
			HashId:      events[i].HashId,
			Etime:       events[i].Etime,
			Value:       events[i].Value,
			Info:        events[i].Info,
			Tags:        tags,
			Created:     events[i].Created,
			Nid:         events[i].Nid,
			Users:       users,
			Groups:      groups,
			Detail:      detail,
			Status:      model.StatusConvert(model.GetStatusByFlag(events[i].Status)),
			NeedUpgrade: events[i].NeedUpgrade,
			AlertUpgrade: AlertUpgrade{
				Groups:   alertGroups,
				Users:    alertUsers,
				Duration: alertUpgrade.Duration,
				Level:    alertUpgrade.Level,
			},
		}

		datList = append(datList, dat)
	}

	renderData(c, map[string]interface{}{
		"total": total,
		"list":  datList,
	}, nil)
}

func eventCurDel(c *gin.Context) {
	eventCur := mustEventCur(urlParamInt64(c, "id"))
	renderMessage(c, eventCur.EventIgnore())
}

func eventHisGetById(c *gin.Context) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	event := mustEvent(urlParamInt64(c, "id"))

	users, err := model.UserNameGetByIds(event.Users)
	errors.Dangerous(err)

	groups, err := model.TeamNameGetsByIds(event.Groups)
	errors.Dangerous(err)

	var detail []model.EventDetail
	err = json.Unmarshal([]byte(event.Detail), &detail)
	errors.Dangerous(err)

	tagsList := []string{}
	for k, v := range detail[0].Tags {
		tagsList = append(tagsList, fmt.Sprintf("%s=%s", k, v))
	}

	alertUpgrade, err := model.EventAlertUpgradeUnMarshal(event.AlertUpgrade)
	errors.Dangerous(err)

	alertUsers, err := model.UserNameGetByIds(alertUpgrade.Users)
	errors.Dangerous(err)

	alertGroups, err := model.TeamNameGetsByIds(alertUpgrade.Groups)
	errors.Dangerous(err)

	dat := eventData{
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
		Tags:        strings.Join(tagsList, ","),
		Created:     event.Created,
		Nid:         event.Nid,
		Users:       users,
		Groups:      groups,
		Detail:      detail,
		Status:      model.StatusConvert(model.GetStatusByFlag(event.Status)),
		NeedUpgrade: event.NeedUpgrade,
		AlertUpgrade: AlertUpgrade{
			Groups:   alertGroups,
			Users:    alertUsers,
			Duration: alertUpgrade.Duration,
			Level:    alertUpgrade.Level,
		},
	}

	renderData(c, dat, nil)
}

func eventCurGetById(c *gin.Context) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	eventCur := mustEventCur(urlParamInt64(c, "id"))

	users, err := model.UserNameGetByIds(eventCur.Users)
	errors.Dangerous(err)

	groups, err := model.TeamNameGetsByIds(eventCur.Groups)
	errors.Dangerous(err)

	claimants, err := model.UserNameGetByIds(eventCur.Claimants)
	errors.Dangerous(err)

	var detail []model.EventDetail
	err = json.Unmarshal([]byte(eventCur.Detail), &detail)
	errors.Dangerous(err)

	tagsList := []string{}
	for k, v := range detail[0].Tags {
		tagsList = append(tagsList, fmt.Sprintf("%s=%s", k, v))
	}

	alertUpgrade, err := model.EventAlertUpgradeUnMarshal(eventCur.AlertUpgrade)
	errors.Dangerous(err)

	alertUsers, err := model.UserNameGetByIds(alertUpgrade.Users)
	errors.Dangerous(err)

	alertGroups, err := model.TeamNameGetsByIds(alertUpgrade.Groups)

	dat := eventData{
		Id:          eventCur.Id,
		Sid:         eventCur.Sid,
		Sname:       eventCur.Sname,
		NodePath:    eventCur.NodePath,
		Endpoint:    eventCur.Endpoint,
		Priority:    eventCur.Priority,
		EventType:   eventCur.EventType,
		Category:    eventCur.Category,
		HashId:      eventCur.HashId,
		Etime:       eventCur.Etime,
		Value:       eventCur.Value,
		Info:        eventCur.Info,
		Tags:        strings.Join(tagsList, ","),
		Created:     eventCur.Created,
		Nid:         eventCur.Nid,
		Users:       users,
		Groups:      groups,
		Detail:      detail,
		Status:      model.StatusConvert(model.GetStatusByFlag(eventCur.Status)),
		Claimants:   claimants,
		NeedUpgrade: eventCur.NeedUpgrade,
		AlertUpgrade: AlertUpgrade{
			Groups:   alertGroups,
			Users:    alertUsers,
			Duration: alertUpgrade.Duration,
			Level:    alertUpgrade.Level,
		},
	}

	renderData(c, dat, nil)
}

type claimForm struct {
	Id       int64  `json:"id"`
	NodePath string `json:"nodepath"`
}

func eventCurClaim(c *gin.Context) {
	me := loginUser(c)

	var f claimForm
	errors.Dangerous(c.ShouldBind(&f))

	id := f.Id
	nodePath := f.NodePath

	if id == 0 && nodePath == "" {
		errors.Dangerous("id and nodepath is blank")
	}

	if id != 0 && nodePath != "" {
		errors.Dangerous("illegal params")
	}

	if id != 0 {
		renderMessage(c, model.UpdateClaimantsById(me.Id, id))
		return
	}

	renderMessage(c, model.UpdateClaimantsByNodePath(me.Id, nodePath))
}
