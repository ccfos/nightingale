package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

const PUSH = 0
const PULL = 1
const ALERT_RULE_ACTIVE = 0
const ALERT_RULE_DISABLED = 1

type AlertRule struct {
	Id                 int64           `json:"id"`
	GroupId            int64           `json:"group_id"`
	Name               string          `json:"name"`
	Type               int             `json:"type"` // 0: nightingale, 1: prometheus
	Expression         json.RawMessage `json:"expression"`
	Status             int             `json:"status"` // 0: active, 1: disabled
	AppendTags         string          `json:"append_tags"`
	EnableStime        string          `json:"enable_stime"`
	EnableEtime        string          `json:"enable_etime"`
	EnableDaysOfWeek   string          `json:"enable_days_of_week"`
	RecoveryNotify     int             `json:"recovery_notify"`
	Priority           int             `json:"priority"`
	NotifyChannels     string          `json:"notify_channels"`
	NotifyGroups       string          `json:"notify_groups"`
	NotifyUsers        string          `json:"notify_users"`
	Callbacks          string          `json:"callbacks"`
	RunbookUrl         string          `json:"runbook_url"`
	Note               string          `json:"note"`
	CreateAt           int64           `json:"create_at"`
	CreateBy           string          `json:"create_by"`
	UpdateAt           int64           `json:"update_at"`
	UpdateBy           string          `json:"update_by"`
	AlertDuration      int             `json:"alert_duration"` // 告警统计周期，PULL模型会当做P8S的for时间
	PushExpr           PushExpression  `xorm:"-" json:"-"`
	PullExpr           PullExpression  `xorm:"-" json:"-"`
	FirstMetric        string          `xorm:"-" json:"-"` // Exps里可能有多个metric，只取第一个，给后续制作map使用
	NotifyUsersDetail  []*User         `xorm:"-" json:"notify_users_detail"`
	NotifyGroupsDetail []*UserGroup    `xorm:"-" json:"notify_groups_detail"`
}

type PushExpression struct {
	TagFilters    []TagFilter `json:"tags_filters"`
	ResFilters    []ResFilter `json:"res_filters"`
	Exps          []Exp       `json:"trigger_conditions"`
	TogetherOrAny int         `json:"together_or_any"` // 所有触发还是触发一条即可，=0所有 =1一条
}

type PullExpression struct {
	PromQl             string `json:"promql"`              // promql 最终表达式
	EvaluationInterval int    `json:"evaluation_interval"` // promql pull 计算周期
}

type ResFilter struct {
	Func string `json:"func"`
	// * InClasspath -> 可以内存里做个大map，host->classpath，然后看host对应的classpath中是否有某一个满足InClasspath的条件
	// * NotInClasspath
	// * InClasspathPrefix -> 可以内存里做个大map，host->classpath，然后看host对应的classpath中是否有某一个满足InClasspathPrefix的条件
	// * NotInClasspathPrefix
	// * InResourceList
	// * NotInResourceList
	// * HasPrefixString
	// * NoPrefixString
	// * HasSuffixString
	// * NoSuffixString
	// * ContainsString
	// * NotContainsString
	// * MatchRegexp
	// * NotMatchRegexp
	Params []string `json:"params"`
}

type TagFilter struct {
	Key  string `json:"key"`
	Func string `json:"func"`
	// * InList
	// * NotInList
	// * HasPrefixString
	// * NoPrefixString
	// * HasSuffixString
	// * NoSuffixString
	// * ContainsString
	// * NotContainsString
	// * MatchRegexp
	// * NotMatchRegexp
	Params []string `json:"params"`
}

type Exp struct {
	Optr      string  `json:"optr"`      //>,<,=,!=
	Func      string  `json:"func"`      //all,max,min
	Metric    string  `json:"metric"`    //metric
	Params    []int   `json:"params"`    //连续n秒
	Threshold float64 `json:"threshold"` //阈值
}

func (ar *AlertRule) Decode() error {
	if ar.Type == PUSH {
		err := json.Unmarshal(ar.Expression, &ar.PushExpr)
		if err != nil {
			logger.Warningf("decode alert rule(%d): unmarshal push expression(%s) error: %v", ar.Id, string(ar.Expression), err)
			return err
		}

		if len(ar.PushExpr.Exps) < 1 {
			logger.Warningf("decode alert rule(%d): exps size is zero", ar.Id)
			return err
		}

		ar.FirstMetric = ar.PushExpr.Exps[0].Metric
	} else {
		err := json.Unmarshal(ar.Expression, &ar.PullExpr)
		if err != nil {
			logger.Warningf("decode alert rule(%d): unmarshal pull expression(%s) error: %v", ar.Id, string(ar.Expression), err)
			return err
		}
	}

	return nil
}

func (ar *AlertRule) TableName() string {
	return "alert_rule"
}

func (ar *AlertRule) Validate() error {
	if str.Dangerous(ar.Name) {
		return _e("AlertRule name has invalid characters")
	}

	if err := ar.Decode(); err != nil {
		return _e("AlertRule expression is invalid")
	}

	if ar.Type == PUSH {
		if ar.AlertDuration <= 0 {
			ar.AlertDuration = 60
		}

		for _, filter := range ar.PushExpr.ResFilters {
			// 参数不能是空的，即不能一个参数都没有
			if len(filter.Params) == 0 {
				return _e("Resource filter(Func:%s)'s param invalid", filter.Func)
			}

			// 对于每个参数而言，不能包含空格，不能是空
			for i := range filter.Params {
				if strings.ContainsAny(filter.Params[i], " \r\n\t") {
					return _e("Resource filter(Func:%s)'s param invalid", filter.Func)
				}

				if filter.Params[i] == "" {
					return _e("Resource filter(Func:%s)'s param invalid", filter.Func)
				}
			}

			if strings.Contains(filter.Func, "Regexp") {
				for i := range filter.Params {
					_, err := regexp.Compile(filter.Params[i])
					if err != nil {
						return _e("Regexp: %s cannot be compiled", filter.Params[i])
					}
				}
			}
		}

		for _, filter := range ar.PushExpr.TagFilters {
			// 参数不能是空的，即不能一个参数都没有
			if len(filter.Params) == 0 {
				return _e("Tags filter(Func:%s)'s param invalid", filter.Func)
			}

			// 对于每个参数而言，不能包含空格，不能是空
			for i := range filter.Params {
				if strings.ContainsAny(filter.Params[i], " \r\n\t") {
					return _e("Tags filter(Func:%s)'s param invalid", filter.Func)
				}

				if filter.Params[i] == "" {
					return _e("Tags filter(Func:%s)'s param invalid", filter.Func)
				}
			}

			if strings.Contains(filter.Func, "Regexp") {
				for i := range filter.Params {
					_, err := regexp.Compile(filter.Params[i])
					if err != nil {
						return _e("Regexp: %s cannot be compiled", filter.Params[i])
					}
				}
			}
		}
	}

	if ar.Type == PULL {
		if ar.AlertDuration <= 0 {
			ar.AlertDuration = 60
		}
		if ar.PullExpr.PromQl == "" {
			return _e("promql empty")
		}
		_, err := parser.ParseExpr(ar.PullExpr.PromQl)

		if err != nil {
			return _e("promql parse error:%s", err.Error())
		}
		if ar.PullExpr.EvaluationInterval <= 0 {
			ar.PullExpr.EvaluationInterval = 15
		}
	}

	ar.AppendTags = strings.TrimSpace(ar.AppendTags)
	arr := strings.Fields(ar.AppendTags)
	for i := 0; i < len(arr); i++ {
		// 如果有appendtags，那就要校验一下格式了
		if len(strings.Split(arr[i], "=")) != 2 {
			return _e("AppendTags(%s) invalid", arr[i])
		}
	}

	// notifyGroups notifyUsers check
	gids := strings.Fields(ar.NotifyGroups)
	for i := 0; i < len(gids); i++ {
		if _, err := strconv.ParseInt(gids[i], 10, 64); err != nil {
			// 这个如果真的非法了肯定是恶意流量，不用i18n
			return fmt.Errorf("NotifyGroups(%s) invalid", ar.NotifyGroups)
		}
	}

	uids := strings.Fields(ar.NotifyUsers)
	for i := 0; i < len(uids); i++ {
		if _, err := strconv.ParseInt(uids[i], 10, 64); err != nil {
			// 这个如果真的非法了肯定是恶意流量，不用i18n
			return fmt.Errorf("NotifyUsers(%s) invalid", ar.NotifyUsers)
		}
	}

	return nil
}

func AlertRuleCount(where string, args ...interface{}) (num int64, err error) {
	num, err = DB.Where(where, args...).Count(new(AlertRule))
	if err != nil {
		logger.Errorf("mysql.error: count alert_rule fail: %v", err)
		return num, internalServerError
	}
	return num, nil
}

func (ar *AlertRule) Add() error {
	if err := ar.Validate(); err != nil {
		return err
	}

	num, err := AlertRuleCount("group_id=? and name=?", ar.GroupId, ar.Name)
	if err != nil {
		return err
	}

	if num > 0 {
		return _e("Alert rule %s already exists", ar.Name)
	}

	now := time.Now().Unix()
	ar.CreateAt = now
	ar.UpdateAt = now
	return DBInsertOne(ar)
}

func (ar *AlertRule) Update(cols ...string) error {
	if err := ar.Validate(); err != nil {
		return err
	}

	_, err := DB.Where("id=?", ar.Id).Cols(cols...).Update(ar)
	if err != nil {
		logger.Errorf("mysql.error: update alert_rule(id=%d) fail: %v", ar.Id, err)
		return internalServerError
	}

	return nil
}

func AlertRuleUpdateStatus(ids []int64, status int) error {
	_, err := DB.Exec("UPDATE alert_rule SET status=? WHERE id in ("+str.IdsString(ids)+")", status)
	return err
}

func AlertRuleUpdateNotifyGroup(ids []int64, NotifyGroups string) error {
	_, err := DB.Exec("UPDATE alert_rule SET notify_groups = ? where id in ("+str.IdsString(ids)+")", NotifyGroups)
	return err
}

func AlertRuleTotal(query string) (num int64, err error) {
	if query != "" {
		q := "%" + query + "%"
		num, err = DB.Where("name like ?", q).Count(new(AlertRule))
	} else {
		num, err = DB.Count(new(AlertRule))
	}

	if err != nil {
		logger.Errorf("mysql.error: count alert_rule fail: %v", err)
		return 0, internalServerError
	}

	return num, nil
}

func AlertRuleGets(query string, limit, offset int) ([]AlertRule, error) {
	session := DB.Limit(limit, offset).OrderBy("name")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("name like ?", q)
	}

	var objs []AlertRule
	err := session.Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query alert_rule fail: %v", err)
		return objs, internalServerError
	}

	return objs, nil
}

func AlertRulesOfGroup(groupId int64) ([]AlertRule, error) {
	var objs []AlertRule
	err := DB.Where("group_id=?", groupId).OrderBy("name").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query alert_rule of group(id=%d) fail: %v", groupId, err)
		return objs, internalServerError
	}

	if len(objs) == 0 {
		return []AlertRule{}, nil
	}

	return objs, nil
}

func AlertRuleGet(where string, args ...interface{}) (*AlertRule, error) {
	var obj AlertRule
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query alert_rule(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func (ar *AlertRule) Del() error {
	_, err := DB.Where("id=?", ar.Id).Delete(new(AlertRule))
	if err != nil {
		logger.Errorf("mysql.error: delete alert_rule fail: %v", err)
		return internalServerError
	}
	return nil
}

func AlertRulesDel(ids []int64) error {
	if len(ids) == 0 {
		return fmt.Errorf("param ids is empty")
	}

	_, err := DB.Exec("DELETE FROM alert_rule where id in (" + str.IdsString(ids) + ")")
	if err != nil {
		logger.Errorf("mysql.error: delete alert_rule(%v) fail: %v", ids, err)
		return internalServerError
	}

	return nil
}

func AlertRuleUpdateGroup(alertRuleIds []int64, groupId int64) error {
	if len(alertRuleIds) == 0 {
		return fmt.Errorf("param alertRuleIds is empty")
	}

	_, err := DB.Exec("UPDATE alert_rule SET group_id = ? where id in ("+str.IdsString(alertRuleIds)+")", groupId)
	if err != nil {
		logger.Errorf("mysql.error: update alert_rule(group_id=%d) fail: %v", groupId, err)
		return internalServerError
	}

	return nil
}

func AllAlertRules() ([]*AlertRule, error) {
	var objs []*AlertRule
	err := DB.Find(&objs)
	return objs, err
}

type AlertRuleStatistic struct {
	Count       int64 `json:"count"`
	MaxUpdateAt int64 `json:"max_update_at"`
}

func GetAlertRuleStatistic() (AlertRuleStatistic, error) {
	var obj AlertRuleStatistic
	_, err := DB.SQL("select count(1) as count, max(update_at) as max_update_at from alert_rule").Get(&obj)
	return obj, err
}
