package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/slice"
)

type EventCur struct {
	Id           int64     `json:"id"`
	Sid          int64     `json:"sid"`
	Sname        string    `json:"sname"`
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
	Nid          int64     `json:"nid"`
	IgnoreAlert  int       `json:"ignore_alert"`
	Claimants    string    `json:"claimants"`
	NeedUpgrade  int       `json:"need_upgrade"`
	AlertUpgrade string    `json:"alert_upgrade"`
	CurNid       string    `json:"cur_nid"`
	WorkGroups   []int     `json:"work_groups" xorm:"-"`
}

func UpdateEventCurPriority(hashid uint64, priority int) error {
	sql := "update event_cur set priority=? where hashid=?"
	_, err := DB["mon"].Exec(sql, priority, hashid)

	return err
}

func SaveEventCur(eventCur *EventCur) error {
	session := DB["mon"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	has, err := session.Where("hashid=?", eventCur.HashId).Get(new(EventCur))
	if err != nil {
		session.Rollback()
		return err
	}

	if has {
		if _, err := session.Where("hashid=?", eventCur.HashId).Cols("sid", "sname", "node_path", "cur_node_path", "endpoint", "priority", "category", "status", "etime", "detail", "value", "info", "users", "groups", "runbook", "nid", "alert_upgrade", "need_upgrade", "endpoint_alias").Update(eventCur); err != nil {
			session.Rollback()
			return err
		}
	} else {
		if _, err := session.Insert(eventCur); err != nil {
			session.Rollback()
			return err
		}
	}

	if err := session.Commit(); err != nil {
		session.Rollback()
		return err
	}

	return nil
}

func UpdateClaimantsById(userId, id int64) error {
	var obj EventCur
	has, err := DB["mon"].Where("id=?", id).Cols("claimants").Get(&obj)

	if err != nil {
		return err
	}

	if !has {
		return fmt.Errorf("event not exists")
	}

	var users []int64
	if err = json.Unmarshal([]byte(obj.Claimants), &users); err != nil {
		return err
	}

	users = append(users, userId)
	data, err := json.Marshal(slice.UniqueInt64(users))
	if err != nil {
		return err
	}

	_, err = DB["mon"].Exec("update event_cur set claimants=? where id=?", string(data), id)
	return err
}

func UpdateClaimantsByNodePath(userId int64, nodePath string) error {
	var objs []EventCur

	session := DB["mon"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if err := session.Where("node_path = ?", nodePath).Find(&objs); err != nil {
		session.Rollback()
		return err
	}

	for i := 0; i < len(objs); i++ {
		var users []int64
		if err := json.Unmarshal([]byte(objs[i].Claimants), &users); err != nil {
			session.Rollback()
			return err
		}

		users = append(users, userId)
		data, err := json.Marshal(slice.UniqueInt64(users))
		if err != nil {
			session.Rollback()
			return err
		}

		_, err = session.Exec("update event_cur set claimants=? where id=?", string(data), objs[i].Id)
		if err != nil {
			session.Rollback()
			return err
		}
	}

	if err := session.Commit(); err != nil {
		session.Rollback()
		return err
	}

	return nil
}

func EventCurDel(hashid uint64) error {
	_, err := DB["mon"].Where("hashid=?", hashid).Delete(new(EventCur))
	return err
}

func SaveEventCurStatus(hashid uint64, status string) error {
	sql := "update event_cur set status = status | ? where hashid = ?"
	_, err := DB["mon"].Exec(sql, GetStatus(status), hashid)

	return err
}

func EventCurTotal(stime, etime int64, nodePath, query string, priorities, sendTypes []string) (int64, error) {
	session := DB["mon"].Where("etime > ? and etime < ? and (node_path = ? or node_path like ?) and ignore_alert=0", stime, etime, nodePath, nodePath+".%")
	if len(priorities) > 0 && priorities[0] != "" {
		session = session.In("priority", priorities)
	}

	if len(sendTypes) > 0 && sendTypes[0] != "" {
		session = session.In("status", GetFlagsByStatus(sendTypes))
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

	total, err := session.Count(new(EventCur))
	return total, err
}

func EventCurGets(stime, etime int64, nodePath, query string, priorities, sendTypes []string, limit, offset int) ([]EventCur, error) {
	var obj []EventCur

	session := DB["mon"].Where("etime > ? and etime < ? and (node_path = ? or node_path like ?) and ignore_alert=0", stime, etime, nodePath, nodePath+".%")
	if len(priorities) > 0 && priorities[0] != "" {
		session = session.In("priority", priorities)
	}

	if len(sendTypes) > 0 && sendTypes[0] != "" {
		session = session.In("status", GetFlagsByStatus(sendTypes))
	}

	if query != "" {
		fields := strings.Fields(query)
		for i := 0; i < len(fields); i++ {
			if fields[i] == "" {
				continue
			}

			q := "%" + fields[i] + "%"
			session = session.Where("sname like ? or endpoint like ? ", q, q)
		}
	}

	err := session.Desc("etime").Limit(limit, offset).Find(&obj)

	return obj, err
}

func EventCurGet(col string, value interface{}) (*EventCur, error) {
	var obj EventCur
	has, err := DB["mon"].Where(col+"=?", value).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (e *EventCur) EventIgnore() error {
	_, err := DB["mon"].Exec("delete from event_cur where id=?", e.Id)
	return err
}

func DelEventCurOlder(ts int64, batch int) error {
	sql := "delete from event_cur where etime < ? limit ?"
	_, err := DB["mon"].Exec(sql, ts, batch)

	return err
}
