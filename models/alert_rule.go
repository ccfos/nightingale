package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/robfig/cron/v3"

	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
	"github.com/tidwall/match"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

const (
	METRIC = "metric"
	LOG    = "logging"
	HOST   = "host"
	LOKI   = "loki"

	PROMETHEUS    = "prometheus"
	TDENGINE      = "tdengine"
	ELASTICSEARCH = "elasticsearch"

	CLICKHOUSE = "ck"
)

const (
	AlertRuleEnabled  = 0
	AlertRuleDisabled = 1

	AlertRuleEnableInGlobalBG = 0
	AlertRuleEnableInOneBG    = 1

	AlertRuleNotNotifyRecovered = 0
	AlertRuleNotifyRecovered    = 1

	AlertRuleNotifyRepeatStep60Min = 60

	AlertRuleRecoverDuration0Sec = 0
)

type AlertRule struct {
	Id                    int64                  `json:"id" gorm:"primaryKey"`
	GroupId               int64                  `json:"group_id"` // busi group id
	Cate                  string                 `json:"cate"`     // alert rule cate (prometheus|elasticsearch)
	DatasourceIds         string                 `json:"-" gorm:"datasource_ids"`
	DatasourceIdsJson     []int64                `json:"datasource_ids,omitempty" gorm:"-"`                                      // alert rule list page use this field
	DatasourceQueries     []DatasourceQuery      `json:"datasource_queries" gorm:"datasource_queries;type:text;serializer:json"` // datasource queries
	Cluster               string                 `json:"cluster"`                                                                // take effect by clusters, seperated by space
	Name                  string                 `json:"name"`                                                                   // rule name
	Note                  string                 `json:"note"`                                                                   // will sent in notify
	Prod                  string                 `json:"prod"`                                                                   // product empty means n9e
	Algorithm             string                 `json:"algorithm"`                                                              // algorithm (''|holtwinters), empty means threshold
	AlgoParams            string                 `json:"-" gorm:"algo_params"`                                                   // params algorithm need
	AlgoParamsJson        interface{}            `json:"algo_params" gorm:"-"`                                                   // for fe
	Delay                 int                    `json:"delay"`                                                                  // Time (in seconds) to delay evaluation
	Severity              int                    `json:"severity"`                                                               // 1: Emergency 2: Warning 3: Notice
	Severities            []int                  `json:"severities" gorm:"-"`                                                    // 1: Emergency 2: Warning 3: Notice
	Disabled              int                    `json:"disabled"`                                                               // 0: enabled, 1: disabled
	PromForDuration       int                    `json:"prom_for_duration"`                                                      // prometheus for, unit:s
	PromQl                string                 `json:"prom_ql"`                                                                // just one ql
	RuleConfig            string                 `json:"-" gorm:"rule_config"`                                                   // rule config
	RuleConfigJson        interface{}            `json:"rule_config" gorm:"-"`                                                   // rule config for fe
	EventRelabelConfig    []*pconf.RelabelConfig `json:"event_relabel_config" gorm:"-"`                                          // event relabel config
	PromEvalInterval      int                    `json:"prom_eval_interval"`                                                     // unit:s
	EnableStime           string                 `json:"-"`                                                                      // split by space: "00:00 10:00 12:00"
	EnableStimeJSON       string                 `json:"enable_stime" gorm:"-"`                                                  // for fe
	EnableStimesJSON      []string               `json:"enable_stimes" gorm:"-"`                                                 // for fe
	EnableEtime           string                 `json:"-"`                                                                      // split by space: "00:00 10:00 12:00"
	EnableEtimeJSON       string                 `json:"enable_etime" gorm:"-"`                                                  // for fe
	EnableEtimesJSON      []string               `json:"enable_etimes" gorm:"-"`                                                 // for fe
	EnableDaysOfWeek      string                 `json:"-"`                                                                      // eg: "0 1 2 3 4 5 6 ; 0 1 2"
	EnableDaysOfWeekJSON  []string               `json:"enable_days_of_week" gorm:"-"`                                           // for fe
	EnableDaysOfWeeksJSON [][]string             `json:"enable_days_of_weeks" gorm:"-"`                                          // for fe
	EnableInBG            int                    `json:"enable_in_bg"`                                                           // 0: global 1: enable one busi-group
	NotifyRecovered       int                    `json:"notify_recovered"`                                                       // whether notify when recovery
	NotifyChannels        string                 `json:"-"`                                                                      // split by space: sms voice email dingtalk wecom
	NotifyChannelsJSON    []string               `json:"notify_channels" gorm:"-"`                                               // for fe
	NotifyGroups          string                 `json:"-"`                                                                      // split by space: 233 43
	NotifyGroupsObj       []UserGroup            `json:"notify_groups_obj" gorm:"-"`                                             // for fe
	NotifyGroupsJSON      []string               `json:"notify_groups" gorm:"-"`                                                 // for fe
	NotifyRepeatStep      int                    `json:"notify_repeat_step"`                                                     // notify repeat interval, unit: min
	NotifyMaxNumber       int                    `json:"notify_max_number"`                                                      // notify: max number
	RecoverDuration       int64                  `json:"recover_duration"`                                                       // unit: s
	Callbacks             string                 `json:"-"`                                                                      // split by space: http://a.com/api/x http://a.com/api/y'
	CallbacksJSON         []string               `json:"callbacks" gorm:"-"`                                                     // for fe
	RunbookUrl            string                 `json:"runbook_url"`                                                            // sop url
	AppendTags            string                 `json:"-"`                                                                      // split by space: service=n9e mod=api
	AppendTagsJSON        []string               `json:"append_tags" gorm:"-"`                                                   // for fe
	Annotations           string                 `json:"-"`                                                                      //
	AnnotationsJSON       map[string]string      `json:"annotations" gorm:"-"`                                                   // for fe
	ExtraConfig           string                 `json:"-" gorm:"extra_config"`                                                  // extra config
	ExtraConfigJSON       interface{}            `json:"extra_config" gorm:"-"`                                                  // for fe
	CreateAt              int64                  `json:"create_at"`
	CreateBy              string                 `json:"create_by"`
	UpdateAt              int64                  `json:"update_at"`
	UpdateBy              string                 `json:"update_by"`
	UUID                  int64                  `json:"uuid" gorm:"-"` // tpl identifier
	CurEventCount         int64                  `json:"cur_event_count" gorm:"-"`
	UpdateByNickname      string                 `json:"update_by_nickname" gorm:"-"` // for fe
	CronPattern           string                 `json:"cron_pattern"`
}

type ChildVarConfig struct {
	ParamVal        []map[string]ParamQuery `json:"param_val"`
	ChildVarConfigs *ChildVarConfig         `json:"child_var_configs"`
}

type ParamQuery struct {
	ParamType string      `json:"param_type"` // host、device、enum、threshold 三种类型
	Query     interface{} `json:"query"`
}

type VarConfig struct {
	ParamVal        []ParamQueryForFirst `json:"param_val"`
	ChildVarConfigs *ChildVarConfig      `json:"child_var_configs"`
}

// ParamQueryForFirst 同 ParamQuery，仅在第一层出现
type ParamQueryForFirst struct {
	Name      string      `json:"name"`
	ParamType string      `json:"param_type"`
	Query     interface{} `json:"query"`
}

type Tpl struct {
	TplId   int64    `json:"tpl_id"`
	TplName string   `json:"tpl_name"`
	Host    []string `json:"host"`
}

type RuleConfig struct {
	Version               string                 `json:"version,omitempty"`
	EventRelabelConfig    []*pconf.RelabelConfig `json:"event_relabel_config,omitempty"`
	TaskTpls              []*Tpl                 `json:"task_tpls,omitempty"`
	Queries               interface{}            `json:"queries,omitempty"`
	Triggers              []Trigger              `json:"triggers,omitempty"`
	Inhibit               bool                   `json:"inhibit,omitempty"`
	PromQl                string                 `json:"prom_ql,omitempty"`
	Severity              int                    `json:"severity,omitempty"`
	AlgoParams            interface{}            `json:"algo_params,omitempty"`
	OverrideGlobalWebhook bool                   `json:"override_global_webhook,omitempty"`
}

type PromRuleConfig struct {
	Queries    []PromQuery `json:"queries"`
	Inhibit    bool        `json:"inhibit"`
	PromQl     string      `json:"prom_ql"`
	Severity   int         `json:"severity"`
	AlgoParams interface{} `json:"algo_params"`
}

type RecoverJudge int

const (
	Origin               RecoverJudge = 0
	NotRecoverWhenNoData RecoverJudge = 1
	RecoverOnCondition   RecoverJudge = 2
)

type RecoverConfig struct {
	JudgeType  RecoverJudge `json:"judge_type"`
	RecoverExp string       `json:"recover_exp"`
}

type HostRuleConfig struct {
	Queries  []HostQuery   `json:"queries"`
	Triggers []HostTrigger `json:"triggers"`
	Inhibit  bool          `json:"inhibit"`
}

type PromQuery struct {
	PromQl        string        `json:"prom_ql"`
	Severity      int           `json:"severity"`
	VarEnabled    bool          `json:"var_enabled"`
	VarConfig     VarConfig     `json:"var_config"`
	RecoverConfig RecoverConfig `json:"recover_config"`
	Unit          string        `json:"unit"`
}

type HostTrigger struct {
	Type     string `json:"type"`
	Duration int    `json:"duration"`
	Percent  int    `json:"percent"`
	Severity int    `json:"severity"`
}

type RuleQuery struct {
	Version           string        `json:"version"`
	Inhibit           bool          `json:"inhibit"`
	Queries           []interface{} `json:"queries"`
	ExpTriggerDisable bool          `json:"exp_trigger_disable"`
	Triggers          []Trigger     `json:"triggers"`
	NodataTrigger     NodataTrigger `json:"nodata_trigger"`
	AnomalyTrigger    interface{}   `json:"anomaly_trigger"`
	TriggerType       TriggerType   `json:"trigger_type,omitempty"` // 在告警事件中使用
}

type NodataTrigger struct {
	Enable             bool `json:"enable"`
	Severity           int  `json:"severity"`
	ResolveAfterEnable bool `json:"resolve_after_enable"`
	ResolveAfter       int  `json:"resolve_after"` // 单位秒
}

type Trigger struct {
	Expressions interface{} `json:"expressions"`
	Mode        int         `json:"mode"`
	Exp         string      `json:"exp"`
	Severity    int         `json:"severity"`

	Type          string        `json:"type,omitempty"`
	Duration      int           `json:"duration,omitempty"`
	Percent       int           `json:"percent,omitempty"`
	Joins         []Join        `json:"joins"`
	JoinRef       string        `json:"join_ref"`
	RecoverConfig RecoverConfig `json:"recover_config"`
}

type Join struct {
	JoinType string   `json:"join_type"`
	Ref      string   `json:"ref"`
	On       []string `json:"on"`
}

var DataSourceQueryAll = DatasourceQuery{
	MatchType: 2,
	Op:        "in",
	Values:    []interface{}{DatasourceIdAll},
}

type DatasourceQuery struct {
	MatchType int           `json:"match_type"`
	Op        string        `json:"op"`
	Values    []interface{} `json:"values"`
}

// GetDatasourceIDsByDatasourceQueries 从 datasourceQueries 中获取 datasourceIDs
// 查询分为精确\模糊匹配，逻辑有 in 与 not in
// idMap 为当前 datasourceQueries 对应的数据源全集
// nameMap 为所有 datasource 的 name 到 id 的映射，用于名称的模糊匹配
func GetDatasourceIDsByDatasourceQueries[T any](datasourceQueries []DatasourceQuery, idMap map[int64]T, nameMap map[string]int64) []int64 {
	if len(datasourceQueries) == 0 {
		return nil
	}

	// 所有 query 取交集，初始集合为全集
	curIDs := make(map[int64]struct{})
	for id, _ := range idMap {
		curIDs[id] = struct{}{}
	}

	for i := range datasourceQueries {
		// 每次 query 都在 curIDs 的基础上得到 dsIDs
		dsIDs := make(map[int64]struct{})
		q := datasourceQueries[i]
		if q.MatchType == 0 {
			// 精确匹配转为 id 匹配
			idValues := make([]int64, 0, len(q.Values))
			for v := range q.Values {
				var val int64
				switch v := q.Values[v].(type) {
				case int64:
					val = v
				case int:
					val = int64(v)
				case float64:
					val = int64(v)
				case float32:
					val = int64(v)
				case int8:
					val = int64(v)
				case int16:
					val = int64(v)
				case int32:
					val = int64(v)
				default:
					continue
				}
				idValues = append(idValues, int64(val))
			}

			if q.Op == "in" {
				if len(idValues) == 1 && idValues[0] == DatasourceIdAll {
					for id := range curIDs {
						dsIDs[id] = struct{}{}
					}
				} else {
					for idx := range idValues {
						if _, exist := curIDs[idValues[idx]]; exist {
							dsIDs[idValues[idx]] = struct{}{}
						}
					}
				}
			} else if q.Op == "not in" {
				for idx := range idValues {
					delete(curIDs, idValues[idx])
				}
				dsIDs = curIDs
			}
		} else if q.MatchType == 1 {
			// 模糊匹配使用 datasource name
			if q.Op == "in" {
				for dsName, dsID := range nameMap {
					if _, exist := curIDs[dsID]; exist {
						for idx := range q.Values {
							if _, ok := q.Values[idx].(string); !ok {
								continue
							}

							if match.Match(dsName, q.Values[idx].(string)) {
								dsIDs[nameMap[dsName]] = struct{}{}
							}
						}
					}
				}
			} else if q.Op == "not in" {
				for dsName, _ := range nameMap {
					for idx := range q.Values {
						if _, ok := q.Values[idx].(string); !ok {
							continue
						}

						if match.Match(dsName, q.Values[idx].(string)) {
							delete(curIDs, nameMap[dsName])
						}
					}
				}
				dsIDs = curIDs
			}
		} else if q.MatchType == 2 {
			// 全部数据源
			for id := range curIDs {
				dsIDs[id] = struct{}{}
			}
		}

		curIDs = dsIDs
		if len(curIDs) == 0 {
			break
		}
	}

	dsIds := make([]int64, 0, len(curIDs))
	for c := range curIDs {
		dsIds = append(dsIds, c)
	}

	return dsIds
}

func GetHostsQuery(queries []HostQuery) []map[string]interface{} {
	var query []map[string]interface{}
	for _, q := range queries {
		m := make(map[string]interface{})
		switch q.Key {
		case "group_ids":
			ids := ParseInt64(q.Values)
			if q.Op == "==" {
				m["target_busi_group.group_id in (?)"] = ids
			} else {
				m["target.ident not in (select target_ident "+
					"from target_busi_group where group_id in (?))"] = ids
			}
		case "tags":
			lst := []string{}
			for _, v := range q.Values {
				if v == nil {
					continue
				}
				lst = append(lst, v.(string))
			}
			if q.Op == "==" {
				blank := " "
				for _, tag := range lst {
					m["tags like ?"+blank] = "%" + tag + "%"
					m["host_tags like ?"+blank] = "%" + tag + "%"
					blank += " "
				}
			} else {
				var args []interface{}
				var query []string
				for _, tag := range lst {
					query = append(query, "tags not like ?",
						"(host_tags not like ? or host_tags is null)")
					args = append(args, "%"+tag+"%", "%"+tag+"%")
				}
				m[strings.Join(query, " and ")] = args
			}
		case "hosts":
			lst := []string{}
			for _, v := range q.Values {
				if v == nil {
					continue
				}
				lst = append(lst, v.(string))
			}
			if q.Op == "==" {
				m["ident in (?)"] = lst
			} else if q.Op == "!=" {
				m["ident not in (?)"] = lst
			} else if q.Op == "=~" {
				blank := " "
				for _, host := range lst {
					m["ident like ?"+blank] = strings.ReplaceAll(host, "*", "%")
					blank += " "
				}
			} else if q.Op == "!~" {
				var args []interface{}
				var query []string
				for _, host := range lst {
					query = append(query, "ident not like ?")
					args = append(args, strings.ReplaceAll(host, "*", "%"))
				}
				m[strings.Join(query, " and ")] = args
			}
		}
		query = append(query, m)
	}
	return query
}

func ParseInt64(values []interface{}) []int64 {
	b, _ := json.Marshal(values)
	var ret []int64
	json.Unmarshal(b, &ret)
	return ret
}

type HostQuery struct {
	Key    string        `json:"key"`
	Op     string        `json:"op"`
	Values []interface{} `json:"values"`
}

func Str2Int(arr []string) []int64 {
	var ret []int64
	for _, v := range arr {
		i, _ := strconv.ParseInt(v, 10, 64)
		ret = append(ret, i)
	}
	return ret
}

func (ar *AlertRule) TableName() string {
	return "alert_rule"
}

func (ar *AlertRule) Verify() error {
	if ar.GroupId < 0 {
		return fmt.Errorf("GroupId(%d) invalid", ar.GroupId)
	}

	//if IsAllDatasource(ar.DatasourceIdsJson) {
	//	ar.DatasourceIdsJson = []int64{0}
	//}

	if str.Dangerous(ar.Name) {
		return errors.New("Name has invalid characters")
	}

	if ar.Name == "" {
		return errors.New("name is blank")
	}

	if ar.Prod == "" {
		ar.Prod = METRIC
	}

	if ar.Cate == "" {
		ar.Cate = PROMETHEUS
	}

	if ar.RuleConfig == "" {
		return errors.New("rule_config is blank")
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

	if err := ar.validateCronPattern(); err != nil {
		return err
	}

	return nil
}

func (ar *AlertRule) validateCronPattern() error {
	if ar.CronPattern == "" {
		return nil
	}

	// 创建一个临时的 cron scheduler 来验证表达式
	scheduler := cron.New(cron.WithSeconds())

	// 尝试添加一个空函数来验证 cron 表达式
	_, err := scheduler.AddFunc(ar.CronPattern, func() {})
	if err != nil {
		return fmt.Errorf("invalid cron pattern: %s, error: %v", ar.CronPattern, err)
	}

	return nil
}

func (ar *AlertRule) Add(ctx *ctx.Context) error {
	if err := ar.Verify(); err != nil {
		return err
	}

	exists, err := AlertRuleExists(ctx, 0, ar.GroupId, ar.Name)
	if err != nil {
		return err
	}

	if exists {
		return errors.New("AlertRule already exists")
	}

	now := time.Now().Unix()
	ar.CreateAt = now
	ar.UpdateAt = now

	return Insert(ctx, ar)
}

func (ar *AlertRule) Update(ctx *ctx.Context, arf AlertRule) error {
	if ar.Name != arf.Name {
		exists, err := AlertRuleExists(ctx, ar.Id, ar.GroupId, arf.Name)
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
	return DB(ctx).Model(ar).Select("*").Updates(arf).Error
}

func (ar *AlertRule) UpdateColumn(ctx *ctx.Context, column string, value interface{}) error {
	if value == nil {
		return nil
	}

	if column == "datasource_ids" {
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return DB(ctx).Model(ar).UpdateColumn(column, string(b)).Error
	}

	if column == "severity" {
		severity := int(value.(float64))
		if ar.Cate == PROMETHEUS {
			var ruleConfig PromRuleConfig
			err := json.Unmarshal([]byte(ar.RuleConfig), &ruleConfig)
			if err != nil {
				return err
			}

			if len(ruleConfig.Queries) < 1 {
				ruleConfig.Severity = severity
				b, err := json.Marshal(ruleConfig)
				if err != nil {
					return err
				}
				return DB(ctx).Model(ar).UpdateColumn("rule_config", string(b)).Error
			}

			if len(ruleConfig.Queries) != 1 {
				return nil
			}

			ruleConfig.Queries[0].Severity = severity
			b, err := json.Marshal(ruleConfig)
			if err != nil {
				return err
			}
			return DB(ctx).Model(ar).UpdateColumn("rule_config", string(b)).Error
		} else if ar.Cate == HOST {
			var ruleConfig HostRuleConfig
			err := json.Unmarshal([]byte(ar.RuleConfig), &ruleConfig)
			if err != nil {
				return err
			}

			if len(ruleConfig.Triggers) != 1 {
				return nil
			}

			ruleConfig.Triggers[0].Severity = severity

			b, err := json.Marshal(ruleConfig)
			if err != nil {
				return err
			}
			return DB(ctx).Model(ar).UpdateColumn("rule_config", string(b)).Error
		} else {
			var ruleConfig RuleQuery
			err := json.Unmarshal([]byte(ar.RuleConfig), &ruleConfig)
			if err != nil {
				return err
			}

			if len(ruleConfig.Triggers) != 1 {
				return nil
			}

			ruleConfig.Triggers[0].Severity = severity
			b, err := json.Marshal(ruleConfig)
			if err != nil {
				return err
			}
			return DB(ctx).Model(ar).UpdateColumn("rule_config", string(b)).Error
		}
	}

	if column == "runbook_url" {
		url := value.(string)

		err := json.Unmarshal([]byte(ar.Annotations), &ar.AnnotationsJSON)
		if err != nil {
			return err
		}

		if ar.AnnotationsJSON == nil {
			ar.AnnotationsJSON = make(map[string]string)
		}

		ar.AnnotationsJSON["runbook_url"] = url

		b, err := json.Marshal(ar.AnnotationsJSON)
		if err != nil {
			return err
		}

		return DB(ctx).Model(ar).UpdateColumn("annotations", string(b)).Error
	}

	if column == "annotations" {
		newAnnotations := value.(map[string]interface{})
		ar.AnnotationsJSON = make(map[string]string)
		for k, v := range newAnnotations {
			ar.AnnotationsJSON[k] = v.(string)
		}
		b, err := json.Marshal(ar.AnnotationsJSON)
		if err != nil {
			return err
		}
		return DB(ctx).Model(ar).UpdateColumn("annotations", string(b)).Error
	}

	return DB(ctx).Model(ar).UpdateColumn(column, value).Error
}

func (ar *AlertRule) UpdateFieldsMap(ctx *ctx.Context, fields map[string]interface{}) error {
	return DB(ctx).Model(ar).Updates(fields).Error
}

func (ar *AlertRule) FillDatasourceQueries() error {
	// 兼容旧逻辑，将 datasourceIds 转换为 datasourceQueries
	if len(ar.DatasourceQueries) == 0 && len(ar.DatasourceIds) != 0 {
		datasourceQueries := DatasourceQuery{
			MatchType: 0,
			Op:        "in",
			Values:    make([]interface{}, 0),
		}

		var values []int
		if ar.DatasourceIds != "" {
			json.Unmarshal([]byte(ar.DatasourceIds), &values)

		}

		for i := range values {
			if values[i] == 0 {
				// 0 表示所有数据源
				datasourceQueries.MatchType = 2
				break
			}
			datasourceQueries.Values = append(datasourceQueries.Values, values[i])
		}
		ar.DatasourceQueries = []DatasourceQuery{datasourceQueries}
	}
	return nil
}

func (ar *AlertRule) FillSeverities() error {
	if ar.RuleConfig != "" {
		var rule RuleQuery
		if err := json.Unmarshal([]byte(ar.RuleConfig), &rule); err != nil {
			return err
		}

		m := make(map[int]struct{})
		if (ar.Cate == PROMETHEUS || ar.Cate == LOKI) && rule.Version != "v2" {
			var rule PromRuleConfig
			if err := json.Unmarshal([]byte(ar.RuleConfig), &rule); err != nil {
				return err
			}

			if len(rule.Queries) == 0 {
				ar.Severities = append(ar.Severities, rule.Severity)
				return nil
			}
			for i := range rule.Queries {
				m[rule.Queries[i].Severity] = struct{}{}
			}
		} else {
			for i := range rule.Triggers {
				m[rule.Triggers[i].Severity] = struct{}{}
			}
		}

		for k := range m {
			ar.Severities = append(ar.Severities, k)
		}
	}
	return nil
}

func (ar *AlertRule) FillNotifyGroups(ctx *ctx.Context, cache map[int64]*UserGroup) error {
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

		ug, err := UserGroupGetById(ctx, id)
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
		DB(ctx).Model(ar).Update("notify_groups", ar.NotifyGroups)
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

	if ar.RuleConfigJson == nil {
		query := PromQuery{
			PromQl:   ar.PromQl,
			Severity: ar.Severity,
		}
		ar.RuleConfigJson = PromRuleConfig{
			Queries: []PromQuery{query},
		}
	}

	// json.Marshal  RuleConfigJson
	if ar.RuleConfigJson != nil {
		b, err := json.Marshal(ar.RuleConfigJson)
		if err != nil {
			return fmt.Errorf("marshal rule_config err:%v", err)
		}
		ar.RuleConfig = string(b)
		ar.PromQl = ""
	}

	if ar.AnnotationsJSON != nil {
		b, err := json.Marshal(ar.AnnotationsJSON)
		if err != nil {
			return fmt.Errorf("marshal annotations err:%v", err)
		}
		ar.Annotations = string(b)
	}

	if ar.ExtraConfigJSON != nil {
		b, err := json.Marshal(ar.ExtraConfigJSON)
		if err != nil {
			return fmt.Errorf("marshal extra_config err:%v", err)
		}
		ar.ExtraConfig = string(b)
	}

	return nil
}

func (ar *AlertRule) DB2FE() error {
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
	json.Unmarshal([]byte(ar.RuleConfig), &ar.RuleConfigJson)
	json.Unmarshal([]byte(ar.Annotations), &ar.AnnotationsJSON)
	json.Unmarshal([]byte(ar.ExtraConfig), &ar.ExtraConfigJSON)

	// 解析 RuleConfig 字段
	var ruleConfig struct {
		EventRelabelConfig []*pconf.RelabelConfig `json:"event_relabel_config"`
	}
	json.Unmarshal([]byte(ar.RuleConfig), &ruleConfig)
	ar.EventRelabelConfig = ruleConfig.EventRelabelConfig

	// 兼容旧逻辑填充 cron_pattern
	if ar.CronPattern == "" && ar.PromEvalInterval != 0 {
		ar.CronPattern = fmt.Sprintf("@every %ds", ar.PromEvalInterval)
	}

	err := ar.FillDatasourceQueries()
	if err != nil {
		return err
	}

	return nil
}

func AlertRuleDels(ctx *ctx.Context, ids []int64, bgid ...int64) error {
	for i := 0; i < len(ids); i++ {
		session := DB(ctx).Where("id = ?", ids[i])
		if len(bgid) > 0 {
			session = session.Where("group_id = ?", bgid[0])
		}
		ret := session.Delete(&AlertRule{})
		if ret.Error != nil {
			return ret.Error
		}

		// 说明确实删掉了，把相关的活跃告警也删了，这些告警永远都不会恢复了，而且策略都没了，说明没���关心了
		if ret.RowsAffected > 0 {
			DB(ctx).Where("rule_id = ?", ids[i]).Delete(new(AlertCurEvent))
		}
	}

	return nil
}

func AlertRuleExists(ctx *ctx.Context, id, groupId int64, name string) (bool, error) {
	session := DB(ctx).Where("id <> ? and group_id = ? and name = ?", id, groupId, name)

	var lst []AlertRule
	err := session.Find(&lst).Error
	if err != nil {
		return false, err
	}
	if len(lst) == 0 {
		return false, nil
	}

	return false, nil
}

func GetAlertRuleIdsByTaskId(ctx *ctx.Context, taskId int64) ([]int64, error) {
	tpl := "%\"tpl_id\":" + fmt.Sprint(taskId) + "}%"
	cb := "{ibex}/" + fmt.Sprint(taskId) + "%"
	session := DB(ctx).Where("rule_config like ? or callbacks like ?", tpl, cb)

	var lst []AlertRule
	var ids []int64
	err := session.Find(&lst).Error
	if err != nil || len(lst) == 0 {
		return ids, err
	}

	for i := 0; i < len(lst); i++ {
		ids = append(ids, lst[i].Id)
	}

	return ids, nil
}

func AlertRuleGets(ctx *ctx.Context, groupId int64) ([]AlertRule, error) {
	session := DB(ctx).Where("group_id=?", groupId).Order("name")

	var lst []AlertRule
	err := session.Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}

	return lst, err
}

func AlertRuleGetsByBGIds(ctx *ctx.Context, bgids []int64) ([]AlertRule, error) {
	session := DB(ctx)
	if len(bgids) > 0 {
		session = session.Where("group_id in (?)", bgids).Order("name")
	}

	var lst []AlertRule
	err := session.Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}

	return lst, err
}

func AlertRuleGetsAll(ctx *ctx.Context) ([]*AlertRule, error) {
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]*AlertRule](ctx, "/v1/n9e/alert-rules?disabled=0")
		if err != nil {
			return nil, err
		}
		for i := 0; i < len(lst); i++ {
			lst[i].FE2DB()
		}
		return lst, err
	}

	session := DB(ctx).Where("disabled = ?", 0)

	var lst []*AlertRule
	err := session.Find(&lst).Error
	if err != nil {
		return lst, err
	}

	if len(lst) == 0 {
		return lst, nil
	}

	for i := 0; i < len(lst); i++ {
		lst[i].DB2FE()
	}
	return lst, nil
}

func AlertRulesGetsBy(ctx *ctx.Context, prods []string, query, algorithm, cluster string,
	cates []string, disabled int) ([]*AlertRule, error) {
	session := DB(ctx)

	if len(prods) > 0 {
		session = session.Where("prod in (?)", prods)
	}

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

func AlertRuleGet(ctx *ctx.Context, where string, args ...interface{}) (*AlertRule, error) {
	var lst []*AlertRule
	err := DB(ctx).Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].DB2FE()

	return lst[0], nil
}

func AlertRuleGetById(ctx *ctx.Context, id int64) (*AlertRule, error) {
	return AlertRuleGet(ctx, "id=?", id)
}

func AlertRuleGetsByIds(ctx *ctx.Context, ids []int64) ([]AlertRule, error) {
	lst := make([]AlertRule, 0, len(ids))
	err := DB(ctx).Model(new(AlertRule)).Where("id in ?", ids).Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}
	return lst, err
}

func AlertRuleStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=alert_rule")
		return s, err
	}

	session := DB(ctx).Model(&AlertRule{}).Select("count(*) as total", "max(update_at) as last_updated").Where("disabled = ?", 0)

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func (ar *AlertRule) IsPrometheusRule() bool {
	return ar.Prod == METRIC && ar.Cate == PROMETHEUS
}

func (ar *AlertRule) IsLokiRule() bool {
	return ar.Prod == LOKI || ar.Cate == LOKI
}

func (ar *AlertRule) IsHostRule() bool {
	return ar.Prod == HOST
}

func (ar *AlertRule) IsTdengineRule() bool {
	return ar.Cate == TDENGINE
}

func (ar *AlertRule) GetRuleType() string {
	if ar.Prod == METRIC || ar.Prod == LOG {
		return ar.Cate
	}

	return ar.Prod
}

func (ar *AlertRule) IsClickHouseRule() bool {
	return ar.Cate == CLICKHOUSE
}

func (ar *AlertRule) IsElasticSearch() bool {
	return ar.Cate == ELASTICSEARCH
}

func (ar *AlertRule) GenerateNewEvent(ctx *ctx.Context) *AlertCurEvent {
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
	event.PromForDuration = ar.PromForDuration
	event.Callbacks = ar.Callbacks
	event.CallbacksJSON = ar.CallbacksJSON
	event.RunbookUrl = ar.RunbookUrl
	event.NotifyRecovered = ar.NotifyRecovered
	event.NotifyChannels = ar.NotifyChannels
	event.NotifyChannelsJSON = ar.NotifyChannelsJSON
	event.NotifyGroups = ar.NotifyGroups
	event.NotifyGroupsJSON = ar.NotifyGroupsJSON
}

func AlertRuleUpgradeToV6(ctx *ctx.Context, dsm map[string]Datasource) error {
	var lst []*AlertRule
	err := DB(ctx).Find(&lst).Error
	if err != nil {
		return err
	}

	for i := 0; i < len(lst); i++ {
		var ids []int64
		if lst[i].Cluster == "$all" {
			ids = append(ids, 0)
		} else {
			clusters := strings.Fields(lst[i].Cluster)
			for j := 0; j < len(clusters); j++ {
				if ds, exists := dsm[clusters[j]]; exists {
					ids = append(ids, ds.Id)
				}
			}
		}

		b, err := json.Marshal(ids)
		if err != nil {
			continue
		}
		lst[i].DatasourceIds = string(b)

		if lst[i].PromQl == "" {
			continue
		}

		ruleConfig := PromRuleConfig{
			Queries: []PromQuery{
				{
					PromQl:   lst[i].PromQl,
					Severity: lst[i].Severity,
				},
			},
		}
		b, _ = json.Marshal(ruleConfig)
		lst[i].RuleConfig = string(b)

		m := make(map[string]string)
		if lst[i].RunbookUrl != "" {
			m["runbook_url"] = lst[i].RunbookUrl

			b, err = json.Marshal(m)
			if err != nil {
				continue
			}

			lst[i].Annotations = string(b)
		}

		if lst[i].Prod == "" {
			lst[i].Prod = METRIC
		}

		if lst[i].Cate == "" {
			lst[i].Cate = PROMETHEUS
		}

		err = lst[i].UpdateFieldsMap(ctx, map[string]interface{}{
			"datasource_ids": lst[i].DatasourceIds,
			"annotations":    lst[i].Annotations,
			"rule_config":    lst[i].RuleConfig,
			"prod":           lst[i].Prod,
			"cate":           lst[i].Cate,
		})
		if err != nil {
			logger.Errorf("update alert rule:%d datasource ids failed, %v", lst[i].Id, err)
		}

	}
	return nil
}

func GetTargetsOfHostAlertRule(ctx *ctx.Context, engineName string) (map[string]map[int64][]string, error) {
	if !ctx.IsCenter {
		m, err := poster.GetByUrls[map[string]map[int64][]string](ctx, "/v1/n9e/targets-of-alert-rule?engine_name="+engineName)
		return m, err
	}

	m := make(map[string]map[int64][]string)
	hostAlertRules, err := AlertRulesGetsBy(ctx, []string{"host"}, "", "", "", []string{}, 0)
	if err != nil {
		return m, err
	}

	for i := 0; i < len(hostAlertRules); i++ {
		var rule *HostRuleConfig
		if err := json.Unmarshal([]byte(hostAlertRules[i].RuleConfig), &rule); err != nil {
			logger.Errorf("rule:%d rule_config:%s, error:%v", hostAlertRules[i].Id, hostAlertRules[i].RuleConfig, err)
			continue
		}

		if rule == nil {
			logger.Errorf("rule:%d rule_config:%s, error:rule is nil", hostAlertRules[i].Id, hostAlertRules[i].RuleConfig)
			continue
		}

		query := GetHostsQuery(rule.Queries)
		session := TargetFilterQueryBuild(ctx, query, 0, 0)
		var lst []*Target
		err := session.Find(&lst).Error
		if err != nil {
			logger.Errorf("failed to query targets: %v", err)
			continue
		}

		for _, target := range lst {
			if _, exists := m[target.EngineName]; !exists {
				m[target.EngineName] = make(map[int64][]string)
			}

			if _, exists := m[target.EngineName][hostAlertRules[i].Id]; !exists {
				m[target.EngineName][hostAlertRules[i].Id] = []string{}
			}

			m[target.EngineName][hostAlertRules[i].Id] = append(m[target.EngineName][hostAlertRules[i].Id], target.Ident)
			logger.Debugf("get_targets_of_alert_rule engine:%s, rule:%d, target:%s", target.EngineName, hostAlertRules[i].Id, target.Ident)
		}
	}

	return m, nil
}

func (ar *AlertRule) Copy(ctx *ctx.Context) (*AlertRule, error) {
	newAr := &AlertRule{}
	err := copier.Copy(newAr, ar)
	if err != nil {
		logger.Errorf("copy alert rule failed, %v", err)
	}
	return newAr, err
}

func InsertAlertRule(ctx *ctx.Context, ars []*AlertRule) error {
	if len(ars) == 0 {
		return nil
	}
	return DB(ctx).Create(ars).Error
}

func (ar *AlertRule) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%s_%s", ar.Id, ar.DatasourceIds, ar.RuleConfig))
}
