package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/src/webapi/config"
)

type AlertRule struct {
	Id                    int64       `json:"id" gorm:"primaryKey"`
	GroupId               int64       `json:"group_id"`                      // busi group id
	Cate                  string      `json:"cate"`                          // alert rule cate (prometheus|elasticsearch)
	Cluster               string      `json:"cluster"`                       // take effect by clusters, seperated by space
	Name                  string      `json:"name"`                          // rule name
	Note                  string      `json:"note"`                          // will sent in notify
	Prod                  string      `json:"prod"`                          // product empty means n9e
	Algorithm             string      `json:"algorithm"`                     // algorithm (''|holtwinters), empty means threshold
	AlgoParams            string      `json:"-" gorm:"algo_params"`          // params algorithm need
	AlgoParamsJson        interface{} `json:"algo_params" gorm:"-"`          //
	Delay                 int         `json:"delay"`                         // Time (in seconds) to delay evaluation
	Severity              int         `json:"severity"`                      // 1: Emergency 2: Warning 3: Notice
	Disabled              int         `json:"disabled"`                      // 0: enabled, 1: disabled
	PromForDuration       int         `json:"prom_for_duration"`             // prometheus for, unit:s
	PromQl                string      `json:"prom_ql"`                       // just one ql
	PromEvalInterval      int         `json:"prom_eval_interval"`            // unit:s
	EnableStime           string      `json:"-"`                             // split by space: "00:00 10:00 12:00"
	EnableStimeJSON       string      `json:"enable_stime" gorm:"-"`         // for fe
	EnableStimesJSON      []string    `json:"enable_stimes" gorm:"-"`        // for fe
	EnableEtime           string      `json:"-"`                             // split by space: "00:00 10:00 12:00"
	EnableEtimeJSON       string      `json:"enable_etime" gorm:"-"`         // for fe
	EnableEtimesJSON      []string    `json:"enable_etimes" gorm:"-"`        // for fe
	EnableDaysOfWeek      string      `json:"-"`                             // eg: "0 1 2 3 4 5 6 ; 0 1 2"
	EnableDaysOfWeekJSON  []string    `json:"enable_days_of_week" gorm:"-"`  // for fe
	EnableDaysOfWeeksJSON [][]string  `json:"enable_days_of_weeks" gorm:"-"` // for fe
	EnableInBG            int         `json:"enable_in_bg"`                  // 0: global 1: enable one busi-group
	NotifyRecovered       int         `json:"notify_recovered"`              // whether notify when recovery
	NotifyChannels        string      `json:"-"`                             // split by space: sms voice email dingtalk wecom
	NotifyChannelsJSON    []string    `json:"notify_channels" gorm:"-"`      // for fe
	NotifyGroups          string      `json:"-"`                             // split by space: 233 43
	NotifyGroupsObj       []UserGroup `json:"notify_groups_obj" gorm:"-"`    // for fe
	NotifyGroupsJSON      []string    `json:"notify_groups" gorm:"-"`        // for fe
	NotifyRepeatStep      int         `json:"notify_repeat_step"`            // notify repeat interval, unit: min
	NotifyMaxNumber       int         `json:"notify_max_number"`             // notify: max number
	RecoverDuration       int64       `json:"recover_duration"`              // unit: s
	Callbacks             string      `json:"-"`                             // split by space: http://a.com/api/x http://a.com/api/y'
	CallbacksJSON         []string    `json:"callbacks" gorm:"-"`            // for fe
	RunbookUrl            string      `json:"runbook_url"`                   // sop url
	AppendTags            string      `json:"-"`                             // split by space: service=n9e mod=api
	AppendTagsJSON        []string    `json:"append_tags" gorm:"-"`          // for fe
	CreateAt              int64       `json:"create_at"`
	CreateBy              string      `json:"create_by"`
	UpdateAt              int64       `json:"update_at"`
	UpdateBy              string      `json:"update_by"`
}

func (ar *AlertRule) TableName() string {
	return "alert_rule"
}

func (ar *AlertRule) Verify() error {
	if ar.GroupId < 0 {
		return fmt.Errorf("GroupId(%d) invalid", ar.GroupId)
	}

	if ar.Cluster == "" {
		return errors.New("cluster is blank")
	}

	if IsClusterAll(ar.Cluster) {
		ar.Cluster = ClusterAll
	}

	if str.Dangerous(ar.Name) {
		return errors.New("Name has invalid characters")
	}

	if ar.Name == "" {
		return errors.New("name is blank")
	}

	if ar.PromQl == "" {
		return errors.New("prom_ql is blank")
	}

	if ar.PromEvalInterval <= 0 {
		ar.PromEvalInterval = 15
	}

	// check in front-end
	// if _, err := parser.ParseExpr(ar.PromQl); err != nil {
	// 	return errors.New("prom_ql parse error: %")
	// }

	ar.AppendTags = strings.TrimSpace(ar.AppendTags)
	arr := strings.Fields(ar.AppendTags)
	for i := 0; i < len(arr); i++ {
		if len(strings.Split(arr[i], "=")) != 2 {
			return fmt.Errorf("AppendTags(%s) invalid", arr[i])
		}
	}

	gids := strings.Fields(ar.NotifyGroups)
	for i := 0; i < len(gids); i++ {
		if _, err := strconv.ParseInt(gids[i], 10, 64); err != nil {
			return fmt.Errorf("NotifyGroups(%s) invalid", ar.NotifyGroups)
		}
	}

	channels := strings.Fields(ar.NotifyChannels)
	if len(channels) > 0 {
		nlst := make([]string, 0, len(channels))
		for i := 0; i < len(channels); i++ {
			if config.LabelAndKeyHasKey(config.C.NotifyChannels, channels[i]) {
				nlst = append(nlst, channels[i])
			}
		}
		ar.NotifyChannels = strings.Join(nlst, " ")
	} else {
		ar.NotifyChannels = ""
	}

	return nil
}

func (ar *AlertRule) Add() error {
	if err := ar.Verify(); err != nil {
		return err
	}

	exists, err := AlertRuleExists(0, ar.GroupId, ar.Cluster, ar.Name)
	if err != nil {
		return err
	}

	if exists {
		return errors.New("AlertRule already exists")
	}

	now := time.Now().Unix()
	ar.CreateAt = now
	ar.UpdateAt = now

	return Insert(ar)
}

func (ar *AlertRule) Update(arf AlertRule) error {
	if ar.Name != arf.Name {
		exists, err := AlertRuleExists(ar.Id, ar.GroupId, ar.Cluster, arf.Name)
		if err != nil {
			return err
		}

		if exists {
			return errors.New("AlertRule already exists")
		}
	}

	err := arf.FE2DB()
	if err != nil {
		return err
	}

	arf.Id = ar.Id
	arf.GroupId = ar.GroupId
	arf.CreateAt = ar.CreateAt
	arf.CreateBy = ar.CreateBy
	arf.UpdateAt = time.Now().Unix()

	err = arf.Verify()
	if err != nil {
		return err
	}
	return DB().Model(ar).Select("*").Updates(arf).Error
}

func (ar *AlertRule) UpdateFieldsMap(fields map[string]interface{}) error {
	return DB().Model(ar).Updates(fields).Error
}

func (ar *AlertRule) FillNotifyGroups(cache map[int64]*UserGroup) error {
	// some user-group already deleted ?
	count := len(ar.NotifyGroupsJSON)
	if count == 0 {
		ar.NotifyGroupsObj = []UserGroup{}
		return nil
	}

	exists := make([]string, 0, count)
	delete := false
	for i := range ar.NotifyGroupsJSON {
		id, _ := strconv.ParseInt(ar.NotifyGroupsJSON[i], 10, 64)

		ug, has := cache[id]
		if has {
			exists = append(exists, ar.NotifyGroupsJSON[i])
			ar.NotifyGroupsObj = append(ar.NotifyGroupsObj, *ug)
			continue
		}

		ug, err := UserGroupGetById(id)
		if err != nil {
			return err
		}

		if ug == nil {
			delete = true
		} else {
			exists = append(exists, ar.NotifyGroupsJSON[i])
			ar.NotifyGroupsObj = append(ar.NotifyGroupsObj, *ug)
			cache[id] = ug
		}
	}

	if delete {
		// some user-group already deleted
		ar.NotifyGroupsJSON = exists
		ar.NotifyGroups = strings.Join(exists, " ")
		DB().Model(ar).Update("notify_groups", ar.NotifyGroups)
	}

	return nil
}

func (ar *AlertRule) FE2DB() error {
	if len(ar.EnableStimesJSON) > 0 {
		ar.EnableStime = strings.Join(ar.EnableStimesJSON, " ")
		ar.EnableEtime = strings.Join(ar.EnableEtimesJSON, " ")
	} else {
		ar.EnableStime = ar.EnableStimeJSON
		ar.EnableEtime = ar.EnableEtimeJSON
	}

	if len(ar.EnableDaysOfWeeksJSON) > 0 {
		for i := 0; i < len(ar.EnableDaysOfWeeksJSON); i++ {
			if len(ar.EnableDaysOfWeeksJSON) == 1 {
				ar.EnableDaysOfWeek = strings.Join(ar.EnableDaysOfWeeksJSON[i], " ")
			} else {
				if i == len(ar.EnableDaysOfWeeksJSON)-1 {
					ar.EnableDaysOfWeek += strings.Join(ar.EnableDaysOfWeeksJSON[i], " ")
				} else {
					ar.EnableDaysOfWeek += strings.Join(ar.EnableDaysOfWeeksJSON[i], " ") + ";"
				}
			}
		}
	} else {
		ar.EnableDaysOfWeek = strings.Join(ar.EnableDaysOfWeekJSON, " ")
	}

	ar.NotifyChannels = strings.Join(ar.NotifyChannelsJSON, " ")
	ar.NotifyGroups = strings.Join(ar.NotifyGroupsJSON, " ")
	ar.Callbacks = strings.Join(ar.CallbacksJSON, " ")
	ar.AppendTags = strings.Join(ar.AppendTagsJSON, " ")
	algoParamsByte, err := json.Marshal(ar.AlgoParamsJson)
	if err != nil {
		return fmt.Errorf("marshal algo_params err:%v", err)
	}

	ar.AlgoParams = string(algoParamsByte)
	return nil
}

func (ar *AlertRule) DB2FE() {
	ar.EnableStimesJSON = strings.Fields(ar.EnableStime)
	ar.EnableEtimesJSON = strings.Fields(ar.EnableEtime)
	if len(ar.EnableEtimesJSON) > 0 {
		ar.EnableStimeJSON = ar.EnableStimesJSON[0]
		ar.EnableEtimeJSON = ar.EnableEtimesJSON[0]
	}

	cache := strings.Split(ar.EnableDaysOfWeek, ";")
	for i := 0; i < len(cache); i++ {
		ar.EnableDaysOfWeeksJSON = append(ar.EnableDaysOfWeeksJSON, strings.Fields(cache[i]))
	}
	if len(ar.EnableDaysOfWeeksJSON) > 0 {
		ar.EnableDaysOfWeekJSON = ar.EnableDaysOfWeeksJSON[0]
	}

	ar.NotifyChannelsJSON = strings.Fields(ar.NotifyChannels)
	ar.NotifyGroupsJSON = strings.Fields(ar.NotifyGroups)
	ar.CallbacksJSON = strings.Fields(ar.Callbacks)
	ar.AppendTagsJSON = strings.Fields(ar.AppendTags)
	json.Unmarshal([]byte(ar.AlgoParams), &ar.AlgoParamsJson)
}

func AlertRuleDels(ids []int64, bgid ...int64) error {
	for i := 0; i < len(ids); i++ {
		session := DB().Where("id = ?", ids[i])
		if len(bgid) > 0 {
			session = session.Where("group_id = ?", bgid[0])
		}
		ret := session.Delete(&AlertRule{})
		if ret.Error != nil {
			return ret.Error
		}

		// 说明确实删掉了，把相关的活跃告警也删了，这些告警永远都不会恢复了，而且策略都没了，说明没人关心了
		if ret.RowsAffected > 0 {
			DB().Where("rule_id = ?", ids[i]).Delete(new(AlertCurEvent))
		}
	}

	return nil
}

func AlertRuleExists(id, groupId int64, cluster, name string) (bool, error) {
	session := DB().Where("id <> ? and group_id = ? and name = ?", id, groupId, name)

	var lst []AlertRule
	err := session.Find(&lst).Error
	if err != nil {
		return false, err
	}
	if len(lst) == 0 {
		return false, nil
	}

	// match cluster
	for _, r := range lst {
		if MatchCluster(r.Cluster, cluster) {
			return true, nil
		}
	}
	return false, nil
}

func AlertRuleGets(groupId int64) ([]AlertRule, error) {
	session := DB().Where("group_id=?", groupId).Order("name")

	var lst []AlertRule
	err := session.Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}

	return lst, err
}

func AlertRuleGetsByCluster(cluster string) ([]*AlertRule, error) {
	session := DB().Where("disabled = ? and prod = ?", 0, "")

	if cluster != "" {
		session = session.Where("(cluster like ? or cluster = ?)", "%"+cluster+"%", ClusterAll)
	}

	var lst []*AlertRule
	err := session.Find(&lst).Error
	if err != nil {
		return lst, err
	}

	if len(lst) == 0 {
		return lst, nil
	}

	if cluster == "" {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
		return lst, nil
	}

	lr := make([]*AlertRule, 0, len(lst))
	for _, r := range lst {
		if MatchCluster(r.Cluster, cluster) {
			r.DB2FE()
			lr = append(lr, r)
		}
	}

	return lr, err
}

func AlertRulesGetsBy(prods []string, query, algorithm, cluster string, cates []string, disabled int) ([]*AlertRule, error) {
	session := DB().Where("prod in (?)", prods)

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("append_tags like ?", qarg)
		}
	}

	if algorithm != "" {
		session = session.Where("algorithm = ?", algorithm)
	}

	if cluster != "" {
		session = session.Where("cluster like ?", "%"+cluster+"%")
	}

	if len(cates) != 0 {
		session = session.Where("cate in (?)", cates)
	}

	if disabled != -1 {
		session = session.Where("disabled = ?", disabled)
	}

	var lst []*AlertRule
	err := session.Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}

	return lst, err
}

func AlertRuleGet(where string, args ...interface{}) (*AlertRule, error) {
	var lst []*AlertRule
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].DB2FE()

	return lst[0], nil
}

func AlertRuleGetById(id int64) (*AlertRule, error) {
	return AlertRuleGet("id=?", id)
}

func AlertRuleGetName(id int64) (string, error) {
	var names []string
	err := DB().Model(new(AlertRule)).Where("id = ?", id).Pluck("name", &names).Error
	if err != nil {
		return "", err
	}

	if len(names) == 0 {
		return "", nil
	}

	return names[0], nil
}

func AlertRuleStatistics(cluster string) (*Statistics, error) {
	session := DB().Model(&AlertRule{}).Select("count(*) as total", "max(update_at) as last_updated").Where("disabled = ? and prod = ?", 0, "")

	if cluster != "" {
		//  简略的判断，当一个clustername是另一个clustername的substring的时候，会出现stats与预期不符，不影响使用
		session = session.Where("(cluster like ? or cluster = ?)", "%"+cluster+"%", ClusterAll)
	}

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func (ar *AlertRule) IsPrometheusRule() bool {
	return ar.Algorithm == "" && (ar.Cate == "" || strings.ToLower(ar.Cate) == "prometheus")
}

func (ar *AlertRule) GenerateNewEvent() *AlertCurEvent {
	event := &AlertCurEvent{}
	ar.UpdateEvent(event)
	return event
}

func (ar *AlertRule) UpdateEvent(event *AlertCurEvent) {
	if event == nil {
		return
	}
	event.GroupId = ar.GroupId
	event.Cate = ar.Cate
	event.RuleId = ar.Id
	event.RuleName = ar.Name
	event.RuleNote = ar.Note
	event.RuleProd = ar.Prod
	event.RuleAlgo = ar.Algorithm
	event.Severity = ar.Severity
	event.PromForDuration = ar.PromForDuration
	event.PromQl = ar.PromQl
	event.PromEvalInterval = ar.PromEvalInterval
	event.Callbacks = ar.Callbacks
	event.CallbacksJSON = ar.CallbacksJSON
	event.RunbookUrl = ar.RunbookUrl
	event.NotifyRecovered = ar.NotifyRecovered
	event.NotifyChannels = ar.NotifyChannels
	event.NotifyChannelsJSON = ar.NotifyChannelsJSON
	event.NotifyGroups = ar.NotifyGroups
	event.NotifyGroupsJSON = ar.NotifyGroupsJSON
}
