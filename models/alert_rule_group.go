package models

import (
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type AlertRuleGroup struct {
	Id           int64       `json:"id"`
	Name         string      `json:"name"`
	UserGroupIds string      `json:"user_group_ids"`
	CreateAt     int64       `json:"create_at"`
	CreateBy     string      `json:"create_by"`
	UpdateAt     int64       `json:"update_at"`
	UpdateBy     string      `json:"update_by"`
	UserGroups   []UserGroup `json:"user_groups" xorm:"-"`
}

func (arg *AlertRuleGroup) TableName() string {
	return "alert_rule_group"
}

func (arg *AlertRuleGroup) Validate() error {
	if str.Dangerous(arg.Name) {
		return _e("AlertRuleGroup name has invalid characters")
	}
	return nil
}

func (arg *AlertRuleGroup) Add() error {
	if err := arg.Validate(); err != nil {
		return err
	}

	num, err := AlertRuleGroupCount("name=?", arg.Name)
	if err != nil {
		return err
	}

	if num > 0 {
		return _e("AlertRuleGroup %s already exists", arg.Name)
	}

	now := time.Now().Unix()
	arg.CreateAt = now
	arg.UpdateAt = now
	return DBInsertOne(arg)
}

func AlertRuleGroupCount(where string, args ...interface{}) (num int64, err error) {
	num, err = DB.Where(where, args...).Count(new(AlertRuleGroup))
	if err != nil {
		logger.Errorf("mysql.error: count alert_rule_group fail: %v", err)
		return num, internalServerError
	}
	return num, nil
}

func (arg *AlertRuleGroup) Update(cols ...string) error {
	if err := arg.Validate(); err != nil {
		return err
	}

	_, err := DB.Where("id=?", arg.Id).Cols(cols...).Update(arg)
	if err != nil {
		logger.Errorf("mysql.error: update alert_rule_group(id=%d) fail: %v", arg.Id, err)
		return internalServerError
	}

	return nil
}

func (arg *AlertRuleGroup) FillUserGroups() error {
	ids := strings.Fields(arg.UserGroupIds)
	if len(ids) == 0 {
		return nil
	}

	ugs, err := UserGroupGetsByIdsStr(ids)
	if err != nil {
		logger.Errorf("mysql.error: UserGroupGetsByIds fail: %v", err)
		return internalServerError
	}

	arg.UserGroups = ugs
	return nil
}

func AlertRuleGroupTotal(query string) (num int64, err error) {
	if query != "" {
		q := "%" + query + "%"
		num, err = DB.Where("name like ?", q).Count(new(AlertRuleGroup))
	} else {
		num, err = DB.Count(new(AlertRuleGroup))
	}

	if err != nil {
		logger.Errorf("mysql.error: count alert_rule_group fail: %v", err)
		return 0, internalServerError
	}

	return num, nil
}

func AlertRuleGroupGets(query string, limit, offset int) ([]AlertRuleGroup, error) {
	session := DB.Limit(limit, offset).OrderBy("name")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("name like ?", q)
	}

	var objs []AlertRuleGroup
	err := session.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query alert_rule_group fail: %v", err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []AlertRuleGroup{}, nil
	}

	return objs, nil
}

func AlertRuleGroupGet(where string, args ...interface{}) (*AlertRuleGroup, error) {
	var obj AlertRuleGroup
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query alert_rule_group(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

// Del AlertRuleGroup删除，前提是下面没有AlertRule了
func (arg *AlertRuleGroup) Del() error {
	ds, err := AlertRulesOfGroup(arg.Id)
	if err != nil {
		return err
	}

	if len(ds) > 0 {
		return _e("There are still alert rules under the group")
	}

	session := DB.NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM alert_rule_group_favorite WHERE group_id=?", arg.Id); err != nil {
		logger.Errorf("mysql.error: delete alert_rule_group_favorite fail: %v", err)
		return err
	}

	if _, err := session.Exec("DELETE FROM alert_rule_group WHERE id=?", arg.Id); err != nil {
		logger.Errorf("mysql.error: delete alert_rule_group fail: %v", err)
		return err
	}

	return session.Commit()
}
