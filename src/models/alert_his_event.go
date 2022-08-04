package models

import (
	"strconv"
	"strings"
)

type AlertHisEvent struct {
	Id                 int64       `json:"id" gorm:"primaryKey"`
	IsRecovered        int         `json:"is_recovered"`
	Cluster            string      `json:"cluster"`
	GroupId            int64       `json:"group_id"`
	GroupName          string      `json:"group_name"` // busi group name
	Hash               string      `json:"hash"`
	RuleId             int64       `json:"rule_id"`
	RuleName           string      `json:"rule_name"`
	RuleNote           string      `json:"rule_note"`
	RuleProd           string      `json:"rule_prod"`
	RuleAlgo           string      `json:"rule_algo"`
	Severity           int         `json:"severity"`
	PromForDuration    int         `json:"prom_for_duration"`
	PromQl             string      `json:"prom_ql"`
	PromEvalInterval   int         `json:"prom_eval_interval"`
	Callbacks          string      `json:"-"`
	CallbacksJSON      []string    `json:"callbacks" gorm:"-"`
	RunbookUrl         string      `json:"runbook_url"`
	NotifyRecovered    int         `json:"notify_recovered"`
	NotifyChannels     string      `json:"-"`
	NotifyChannelsJSON []string    `json:"notify_channels" gorm:"-"`
	NotifyGroups       string      `json:"-"`
	NotifyGroupsJSON   []string    `json:"notify_groups" gorm:"-"`
	NotifyGroupsObj    []UserGroup `json:"notify_groups_obj" gorm:"-"`
	TargetIdent        string      `json:"target_ident"`
	TargetNote         string      `json:"target_note"`
	TriggerTime        int64       `json:"trigger_time"`
	TriggerValue       string      `json:"trigger_value"`
	RecoverTime        int64       `json:"recover_time"`
	LastEvalTime       int64       `json:"last_eval_time"`
	Tags               string      `json:"-"`
	TagsJSON           []string    `json:"tags" gorm:"-"`
	NotifyCurNumber    int         `json:"notify_cur_number"`  // notify: current number
	FirstTriggerTime   int64       `json:"first_trigger_time"` // 连续告警的首次告警时间
}

func (e *AlertHisEvent) TableName() string {
	return "alert_his_event"
}

func (e *AlertHisEvent) Add() error {
	return Insert(e)
}

func (e *AlertHisEvent) DB2FE() {
	e.NotifyChannelsJSON = strings.Fields(e.NotifyChannels)
	e.NotifyGroupsJSON = strings.Fields(e.NotifyGroups)
	e.CallbacksJSON = strings.Fields(e.Callbacks)
	e.TagsJSON = strings.Split(e.Tags, ",,")
}

func (e *AlertHisEvent) FillNotifyGroups(cache map[int64]*UserGroup) error {
	// some user-group already deleted ?
	count := len(e.NotifyGroupsJSON)
	if count == 0 {
		e.NotifyGroupsObj = []UserGroup{}
		return nil
	}

	for i := range e.NotifyGroupsJSON {
		id, err := strconv.ParseInt(e.NotifyGroupsJSON[i], 10, 64)
		if err != nil {
			continue
		}

		ug, has := cache[id]
		if has {
			e.NotifyGroupsObj = append(e.NotifyGroupsObj, *ug)
			continue
		}

		ug, err = UserGroupGetById(id)
		if err != nil {
			return err
		}

		if ug != nil {
			e.NotifyGroupsObj = append(e.NotifyGroupsObj, *ug)
			cache[id] = ug
		}
	}

	return nil
}

func AlertHisEventTotal(prod string, bgid, stime, etime int64, severity int, recovered int, clusters []string, query string) (int64, error) {
	session := DB().Model(&AlertHisEvent{}).Where("last_eval_time between ? and ? and rule_prod = ?", stime, etime, prod)

	if bgid > 0 {
		session = session.Where("group_id = ?", bgid)
	}

	if severity >= 0 {
		session = session.Where("severity = ?", severity)
	}

	if recovered >= 0 {
		session = session.Where("is_recovered = ?", recovered)
	}

	if len(clusters) > 0 {
		session = session.Where("cluster in ?", clusters)
	}

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("rule_name like ? or tags like ?", qarg, qarg)
		}
	}

	return Count(session)
}

func AlertHisEventGets(prod string, bgid, stime, etime int64, severity int, recovered int, clusters []string, query string, limit, offset int) ([]AlertHisEvent, error) {
	session := DB().Where("last_eval_time between ? and ? and rule_prod = ?", stime, etime, prod)

	if bgid > 0 {
		session = session.Where("group_id = ?", bgid)
	}

	if severity >= 0 {
		session = session.Where("severity = ?", severity)
	}

	if recovered >= 0 {
		session = session.Where("is_recovered = ?", recovered)
	}

	if len(clusters) > 0 {
		session = session.Where("cluster in ?", clusters)
	}

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("rule_name like ? or tags like ?", qarg, qarg)
		}
	}

	var lst []AlertHisEvent
	err := session.Order("id desc").Limit(limit).Offset(offset).Find(&lst).Error

	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}

	return lst, err
}

func AlertHisEventGet(where string, args ...interface{}) (*AlertHisEvent, error) {
	var lst []*AlertHisEvent
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].DB2FE()
	lst[0].FillNotifyGroups(make(map[int64]*UserGroup))

	return lst[0], nil
}

func AlertHisEventGetById(id int64) (*AlertHisEvent, error) {
	return AlertHisEventGet("id=?", id)
}
