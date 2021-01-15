package models

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/didi/nightingale/src/modules/monapi/config"
)

type Event struct {
	Id           int64     `json:"id"`
	Sid          int64     `json:"sid"`
	Sname        string    `json:"sname"`
	Nid          int64     `json:"nid"`
	NodePath     string    `json:"node_path"`
	CurNodePath  string    `json:"cur_node_path"`
	Endpoint     string    `json:"endpoint"`
	Priority     int       `json:"priority"`
	EventType    string    `json:"event_type"` // alert|recovery
	Category     int       `json:"category"`
	Status       uint16    `json:"status"`
	HashId       uint64    `json:"hashid"  xorm:"hashid"`
	Etime        int64     `json:"etime"`
	Value        string    `json:"value"`
	Info         string    `json:"info"`
	Created      time.Time `json:"created" xorm:"created"`
	Detail       string    `json:"detail"`
	Users        string    `json:"users"`
	Groups       string    `json:"groups"`
	Runbook      string    `json:"runbook"`
	NeedUpgrade  int       `json:"need_upgrade"`
	AlertUpgrade string    `json:"alert_upgrade"`
	RecvUserIDs  []int64   `json:"recv_user_ids" xorm:"-"`
	RealUpgrade  bool      `json:"real_upgrade" xorm:"-"`
	WorkGroups   []int     `json:"work_groups" xorm:"-"`
	CurNid       string    `json:"cur_nid"`
}

type EventDetail struct {
	Metric     string              `json:"metric"`
	Tags       map[string]string   `json:"tags"`
	Points     []*EventDetailPoint `json:"points"`
	PredPoints []*EventDetailPoint `json:"pred_points,omitempty"` // 预测值, 预测值不为空时, 现场值对应的是实际值
}

type EventDetailPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
	Extra     string  `json:"extra"`
}

type EventAlertUpgrade struct {
	Users    string `json:"users"`
	Groups   string `json:"groups"`
	Duration int    `json:"duration"`
	Level    int    `json:"level"`
}

func ParseEtime(etime int64) string {
	t := time.Unix(etime, 0)
	return t.Format("2006-01-02 15:04:05")
}

type EventSlice []*Event

func (e EventSlice) Len() int {
	return len(e)
}

func (e EventSlice) Less(i, j int) bool {
	return e[i].Etime < e[j].Etime
}

func (e EventSlice) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func SaveEvent(event *Event) error {
	_, err := DB["mon"].Insert(event)
	return err
}

func SaveEventStatus(id int64, status string) error {
	sql := "update event set status = status | ? where id = ?"
	_, err := DB["mon"].Exec(sql, GetStatus(status), id)
	return err
}

func UpdateEventPriority(id int64, priority int) error {
	sql := "update event set priority=? where id=?"
	_, err := DB["mon"].Exec(sql, priority, id)

	return err
}

func (e *Event) GetEventDetail() ([]EventDetail, error) {
	detail := []EventDetail{}

	err := json.Unmarshal([]byte(e.Detail), &detail)
	return detail, err
}

func EventTotal(stime, etime int64, nodePath, query, eventType string, priorities, sendTypes []string) (int64, error) {
	sql := "etime > ? and etime < ?"
	sqlParamValue := []interface{}{stime, etime}
	if nodePath != "" {
		sql += " and (node_path = ? or node_path like ?) "
		sqlParamValue = []interface{}{stime, etime, nodePath, nodePath + ".%"}
	}

	session := DB["mon"].Where(sql, sqlParamValue...)
	if len(priorities) > 0 && priorities[0] != "" {
		session = session.In("priority", priorities)
	}

	if len(sendTypes) > 0 && sendTypes[0] != "" {
		session = session.In("status", GetFlagsByStatus(sendTypes))
	}

	if eventType != "" {
		session = session.Where("event_type=?", eventType)
	}

	if query != "" {
		fields := strings.Fields(query)
		for i := 0; i < len(fields); i++ {
			if fields[i] == "" {
				continue
			}

			q := "%" + fields[i] + "%"
			session = session.Where("sname like ? or endpoint like ?", q, q)
		}
	}

	total, err := session.Count(new(Event))
	return total, err
}

func EventGets(stime, etime int64, nodePath, query, eventType string, priorities, sendTypes []string, limit, offset int) ([]Event, error) {
	var objs []Event

	sql := "etime > ? and etime < ?"
	sqlParamValue := []interface{}{stime, etime}
	if nodePath != "" {
		sql += " and (node_path = ? or node_path like ?) "
		sqlParamValue = []interface{}{stime, etime, nodePath, nodePath + ".%"}
	}

	session := DB["mon"].Where(sql, sqlParamValue...)
	if len(priorities) > 0 && priorities[0] != "" {
		session = session.In("priority", priorities)
	}

	if len(sendTypes) > 0 && sendTypes[0] != "" {
		session = session.In("status", GetFlagsByStatus(sendTypes))
	}

	if eventType != "" {
		session = session.Where("event_type=?", eventType)
	}

	if query != "" {
		fields := strings.Fields(query)
		for i := 0; i < len(fields); i++ {
			if fields[i] == "" {
				continue
			}

			q := "%" + fields[i] + "%"
			session = session.Where("sname like ? or endpoint like ?", q, q)
		}
	}

	err := session.Desc("etime").Limit(limit, offset).Find(&objs)

	return objs, err
}

func EventGet(col string, value interface{}) (*Event, error) {
	var obj Event
	has, err := DB["mon"].Where(col+"=?", value).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func DelEventOlder(ts int64, batch int) error {
	sql := "delete from event where etime < ? limit ?"
	_, err := DB["mon"].Exec(sql, ts, batch)

	return err
}

func EventDelById(id interface{}) error {
	_, err := DB["mon"].Where("id=?", id).Delete(new(Event))
	return err
}

func EventAlertUpgradeUnMarshal(str string) (EventAlertUpgrade, error) {
	var obj EventAlertUpgrade
	if strings.TrimSpace(str) == "" {
		return EventAlertUpgrade{
			Users:    "[]",
			Groups:   "[]",
			Duration: 0,
			Level:    0,
		}, nil
	}

	err := json.Unmarshal([]byte(str), &obj)
	return obj, err
}

func EventCnt(hashid uint64, stime, etime int64, isUpgrade bool) (int64, error) {
	session := DB["mon"].Where("hashid = ? and event_type = ? and etime between ? and ?", hashid, config.ALERT, stime, etime)

	if isUpgrade {
		return session.In("status", GetFlagsByStatus([]string{STATUS_UPGRADE, STATUS_SEND})).Count(new(Event))
	}

	return session.In("status", GetFlagsByStatus([]string{STATUS_SEND})).Count(new(Event))
}

func EventAlertUpgradeMarshal(alertUpgrade AlertUpgrade) (string, error) {
	eventAlertUpgrade := EventAlertUpgrade{
		Duration: alertUpgrade.Duration,
		Level:    alertUpgrade.Level,
	}

	if alertUpgrade.Users == nil {
		eventAlertUpgrade.Users = "[]"
	} else {
		upgradeUsers, err := json.Marshal(alertUpgrade.Users)
		if err != nil {
			return "", err
		}

		eventAlertUpgrade.Users = string(upgradeUsers)
	}

	if alertUpgrade.Groups == nil {
		eventAlertUpgrade.Groups = "[]"
	} else {
		upgradeGroups, err := json.Marshal(alertUpgrade.Groups)
		if err != nil {
			return "", err
		}

		eventAlertUpgrade.Groups = string(upgradeGroups)
	}

	alertUpgradebytes, err := json.Marshal(eventAlertUpgrade)

	return string(alertUpgradebytes), err
}
