package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/didi/nightingale/src/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type eventData struct {
	Id           int64                `json:"id"`
	Sid          int64                `json:"sid"`
	Sname        string               `json:"sname"`
	NodePath     string               `json:"node_path"`
	CurNid       string               `json:"cur_id"`
	CurNodePath  string               `json:"cur_node_path"`
	Nid          int64                `json:"nid"`
	Endpoint     string               `json:"endpoint"`
	Priority     int                  `json:"priority"`
	EventType    string               `json:"event_type"` // alert|recovery
	Category     int                  `json:"category"`
	HashId       uint64               `json:"hashid"  xorm:"hashid"`
	Etime        int64                `json:"etime"`
	Value        string               `json:"value"`
	Info         string               `json:"info"`
	Tags         string               `json:"tags"`
	Created      time.Time            `json:"created" xorm:"created"`
	Detail       []models.EventDetail `json:"detail"`
	Users        []string             `json:"users"`
	Groups       []string             `json:"groups"`
	Status       []string             `json:"status"`
	Claimants    []string             `json:"claimants,omitempty"`
	NeedUpgrade  int                  `json:"need_upgrade"`
	AlertUpgrade AlertUpgrade         `json:"alert_upgrade"`
	Runbook      string               `json:"runbook"`
}

type AlertUpgrade struct {
	Users    []string `json:"users"`
	Groups   []string `json:"groups"`
	Duration int      `json:"duration"`
	Level    int      `json:"level"`
}

func eventCurGets(c *gin.Context) {

	stime := queryInt64(c, "stime", 0)
	etime := queryInt64(c, "etime", 0)

	hours := queryInt64(c, "hours", 0)
	now := time.Now().Unix()
	if hours != 0 {
		stime = now - 3600*hours
		etime = now + 3600*24
	}

	if stime != 0 && etime == 0 {
		etime = now + 3600*24
	}

	if stime == 0 && hours == 0 {
		dangerous(fmt.Errorf("stime and hours is nil"))
	}

	nodePath := queryStr(c, "nodepath", "")

	limit := queryInt(c, "limit", 20)

	priorities := queryStr(c, "priorities", "")
	sendtypes := queryStr(c, "sendtypes", "")
	query := queryStr(c, "query", "")

	total, err := models.EventCurTotal(stime, etime, nodePath, query, strings.Split(priorities, ","), strings.Split(sendtypes, ","))
	errors.Dangerous(err)

	events, err := models.EventCurGets(stime, etime, nodePath, query, strings.Split(priorities, ","), strings.Split(sendtypes, ","), limit, offset(c, limit, total))
	errors.Dangerous(err)

	datList := []eventData{}
	for i := 0; i < len(events); i++ {
		users, err := models.GetUsersNameByIds(events[i].Users)
		errors.Dangerous(err)

		groups, err := models.GetTeamsNameByIds(events[i].Groups)
		errors.Dangerous(err)

		claimants, err := models.GetUsersNameByIds(events[i].Claimants)
		errors.Dangerous(err)

		var detail []models.EventDetail
		err = json.Unmarshal([]byte(events[i].Detail), &detail)
		if err != nil {
			logger.Error("unmarshl event:%v detail err:%v", events[i], err)
			continue
		}

		tagsList := []string{}
		if len(detail) > 0 {
			for k, v := range detail[0].Tags {
				tagsList = append(tagsList, fmt.Sprintf("%s=%s", k, v))
			}
		}

		alertUpgrade, err := models.EventAlertUpgradeUnMarshal(events[i].AlertUpgrade)
		errors.Dangerous(err)

		alertUsers, err := models.GetUsersNameByIds(alertUpgrade.Users)
		errors.Dangerous(err)

		alertGroups, err := models.GetTeamsNameByIds(alertUpgrade.Groups)
		errors.Dangerous(err)

		dat := eventData{
			Id:          events[i].Id,
			Sid:         events[i].Sid,
			Sname:       events[i].Sname,
			NodePath:    events[i].NodePath,
			CurNid:      events[i].CurNid,
			CurNodePath: events[i].CurNodePath,
			Endpoint:    events[i].Endpoint,
			Priority:    events[i].Priority,
			EventType:   events[i].EventType,
			Category:    events[i].Category,
			HashId:      events[i].HashId,
			Etime:       events[i].Etime,
			Value:       events[i].Value,
			Info:        events[i].Info,
			Tags:        strings.Join(tagsList, ","),
			Created:     events[i].Created,
			Nid:         events[i].Nid,
			Users:       users,
			Groups:      groups,
			Runbook:     events[i].Runbook,
			Detail:      detail,
			Status:      models.StatusConvert(models.GetStatusByFlag(events[i].Status)),
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

	stime := queryInt64(c, "stime", 0)
	etime := queryInt64(c, "etime", 0)

	hours := queryInt64(c, "hours", 0)
	now := time.Now().Unix()
	if hours != 0 {
		stime = now - 3600*hours
		etime = now + 3600*24
	}

	if stime != 0 && etime == 0 {
		etime = now + 3600*24
	}

	if stime == 0 && hours == 0 {
		dangerous(fmt.Errorf("stime and hours is nil"))
	}

	nodePath := queryStr(c, "nodepath", "")

	limit := queryInt(c, "limit", 20)

	priorities := queryStr(c, "priorities", "")
	sendtypes := queryStr(c, "sendtypes", "")
	query := queryStr(c, "query", "")
	eventType := queryStr(c, "type", "")

	total, err := models.EventTotal(stime, etime, nodePath, query, eventType, strings.Split(priorities, ","), strings.Split(sendtypes, ","))
	errors.Dangerous(err)

	events, err := models.EventGets(stime, etime, nodePath, query, eventType, strings.Split(priorities, ","), strings.Split(sendtypes, ","), limit, offset(c, limit, total))
	errors.Dangerous(err)

	datList := []eventData{}
	for i := 0; i < len(events); i++ {
		users, err := models.GetUsersNameByIds(events[i].Users)
		errors.Dangerous(err)

		groups, err := models.GetTeamsNameByIds(events[i].Groups)
		errors.Dangerous(err)

		var detail []models.EventDetail
		err = json.Unmarshal([]byte(events[i].Detail), &detail)
		if err != nil {
			logger.Error("unmarshl event:%v detail err:%v", events[i], err)
			continue
		}

		tagsList := []string{}
		if len(detail) > 0 {
			for k, v := range detail[0].Tags {
				tagsList = append(tagsList, fmt.Sprintf("%s=%s", k, v))
			}
		}

		alertUpgrade, err := models.EventAlertUpgradeUnMarshal(events[i].AlertUpgrade)
		errors.Dangerous(err)

		alertUsers, err := models.GetUsersNameByIds(alertUpgrade.Users)
		errors.Dangerous(err)

		alertGroups, err := models.GetTeamsNameByIds(alertUpgrade.Groups)
		errors.Dangerous(err)

		dat := eventData{
			Id:          events[i].Id,
			Sid:         events[i].Sid,
			Sname:       events[i].Sname,
			NodePath:    events[i].NodePath,
			CurNid:      events[i].CurNid,
			CurNodePath: events[i].CurNodePath,
			Endpoint:    events[i].Endpoint,
			Priority:    events[i].Priority,
			EventType:   events[i].EventType,
			Category:    events[i].Category,
			HashId:      events[i].HashId,
			Etime:       events[i].Etime,
			Value:       events[i].Value,
			Info:        events[i].Info,
			Tags:        strings.Join(tagsList, ","),
			Created:     events[i].Created,
			Nid:         events[i].Nid,
			Runbook:     events[i].Runbook,
			Users:       users,
			Groups:      groups,
			Detail:      detail,
			Status:      models.StatusConvert(models.GetStatusByFlag(events[i].Status)),
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

	event := mustEvent(urlParamInt64(c, "id"))

	users, err := models.GetUsersNameByIds(event.Users)
	errors.Dangerous(err)

	groups, err := models.GetTeamsNameByIds(event.Groups)
	errors.Dangerous(err)

	var detail []models.EventDetail
	err = json.Unmarshal([]byte(event.Detail), &detail)
	errors.Dangerous(err)

	tagsList := []string{}
	if len(detail) > 0 {
		for k, v := range detail[0].Tags {
			tagsList = append(tagsList, fmt.Sprintf("%s=%s", k, v))
		}
	}

	alertUpgrade, err := models.EventAlertUpgradeUnMarshal(event.AlertUpgrade)
	errors.Dangerous(err)

	alertUsers, err := models.GetUsersNameByIds(alertUpgrade.Users)
	errors.Dangerous(err)

	alertGroups, err := models.GetTeamsNameByIds(alertUpgrade.Groups)
	errors.Dangerous(err)

	dat := eventData{
		Id:          event.Id,
		Sid:         event.Sid,
		Sname:       event.Sname,
		NodePath:    event.NodePath,
		CurNid:      event.CurNid,
		CurNodePath: event.CurNodePath,
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
		Runbook:     event.Runbook,
		Detail:      detail,
		Status:      models.StatusConvert(models.GetStatusByFlag(event.Status)),
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

	eventCur := mustEventCur(urlParamInt64(c, "id"))

	users, err := models.GetUsersNameByIds(eventCur.Users)
	errors.Dangerous(err)

	groups, err := models.GetTeamsNameByIds(eventCur.Groups)
	errors.Dangerous(err)

	claimants, err := models.GetUsersNameByIds(eventCur.Claimants)
	errors.Dangerous(err)

	var detail []models.EventDetail
	err = json.Unmarshal([]byte(eventCur.Detail), &detail)
	errors.Dangerous(err)

	tagsList := []string{}
	if len(detail) > 0 {
		for k, v := range detail[0].Tags {
			tagsList = append(tagsList, fmt.Sprintf("%s=%s", k, v))
		}
	}

	alertUpgrade, err := models.EventAlertUpgradeUnMarshal(eventCur.AlertUpgrade)
	errors.Dangerous(err)

	alertUsers, err := models.GetUsersNameByIds(alertUpgrade.Users)
	errors.Dangerous(err)

	alertGroups, err := models.GetTeamsNameByIds(alertUpgrade.Groups)

	dat := eventData{
		Id:          eventCur.Id,
		Sid:         eventCur.Sid,
		Sname:       eventCur.Sname,
		NodePath:    eventCur.NodePath,
		CurNid:      eventCur.CurNid,
		CurNodePath: eventCur.CurNodePath,
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
		Runbook:     eventCur.Runbook,
		Detail:      detail,
		Status:      models.StatusConvert(models.GetStatusByFlag(eventCur.Status)),
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
	username := loginUsername(c)
	users, err := models.UserGetByNames([]string{username})
	errors.Dangerous(err)

	if len(users) < 1 {
		errors.Dangerous("user not found")
	}

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
		renderMessage(c, models.UpdateClaimantsById(users[0].Id, id))
		return
	}

	renderMessage(c, models.UpdateClaimantsByNodePath(users[0].Id, nodePath))
}
