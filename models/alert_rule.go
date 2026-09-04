package models

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/robfig/cron/v3"

	"github.com/jinzhu/copier"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/tidwall/match"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

const (
	METRIC = "metric"
	LOG    = "logging"
	HOST   = "host"
	LOKI   = "loki"

	PROMETHEUS    = "prometheus"
	TDENGINE      = "tdengine"
	IOTDB         = "iotdb"
	ELASTICSEARCH = "elasticsearch"
	MYSQL         = "mysql"
	POSTGRESQL    = "pgsql"
	DORIS         = "doris"
	OPENSEARCH    = "opensearch"

	CLICKHOUSE   = "ck"
	VICTORIALOGS = "victorialogs"
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

const (
	SeverityEmergency = 1
	SeverityWarning   = 2
	SeverityNotice    = 3
	SeverityLowest    = 4
)

type AlertRule struct {
	Id                    int64                  `json:"id" gorm:"primaryKey"`
	GroupId               int64                  `json:"group_id"`                                                               // busi group id
	Cate                  string                 `json:"cate"`                                                                   // alert rule cate (prometheus|elasticsearch)
	DatasourceIds         string                 `json:"-" gorm:"datasource_ids"`                                                // Deprecated: use DatasourceQueries instead
	DatasourceIdsJson     []int64                `json:"datasource_ids,omitempty" gorm:"-"`                                      // alert rule list page use this field
	DatasourceQueries     []DatasourceQuery      `json:"datasource_queries" gorm:"datasource_queries;type:text;serializer:json"` // datasource queries
	Cluster               string                 `json:"cluster"`                                                                // Deprecated: use DatasourceQueries instead                                                           // take effect by clusters, separated by space
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
	PromForDuration       int                    `json:"prom_for_duration"`                                                      // Deprecated: use cron pattern instead                                                     // prometheus for, unit:s
	PromQl                string                 `json:"prom_ql"`                                                                // just one ql
	RuleConfig            string                 `json:"-" gorm:"rule_config"`                                                   // rule config
	RuleConfigJson        interface{}            `json:"rule_config" gorm:"-"`                                                   // rule config for fe
	EventRelabelConfig    []*pconf.RelabelConfig `json:"event_relabel_config" gorm:"-"`                                          // event relabel config
	PromEvalInterval      int                    `json:"prom_eval_interval"`                                                     // unit:s
	EnableStime           string                 `json:"-"`                                                                      // Deprecated                                                                  // split by space: "00:00 10:00 12:00"
	EnableStimeJSON       string                 `json:"enable_stime" gorm:"-"`                                                  // Deprecated                                               // for fe
	EnableStimesJSON      []string               `json:"enable_stimes" gorm:"-"`                                                 // for fe
	EnableEtime           string                 `json:"-"`                                                                      // Deprecated                                                                // split by space: "00:00 10:00 12:00"
	EnableEtimeJSON       string                 `json:"enable_etime" gorm:"-"`                                                  // Deprecated                                             // for fe
	EnableEtimesJSON      []string               `json:"enable_etimes" gorm:"-"`                                                 // for fe
	EnableDaysOfWeek      string                 `json:"-"`                                                                      // Deprecated                                                             // eg: "0 1 2 3 4 5 6 ; 0 1 2"
	EnableDaysOfWeekJSON  []string               `json:"enable_days_of_week" gorm:"-"`                                           // Deprecated                                         // for fe
	EnableDaysOfWeeksJSON [][]string             `json:"enable_days_of_weeks" gorm:"-"`                                          // for fe
	EnableInBG            int                    `json:"enable_in_bg"`                                                           // 0: global 1: enable one busi-group
	NotifyRecovered       int                    `json:"notify_recovered"`                                                       // whether notify when recovery
	NotifyChannels        string                 `json:"-"`                                                                      // Deprecated                                                        // split by space: sms voice email dingtalk wecom
	NotifyChannelsJSON    []string               `json:"notify_channels" gorm:"-"`                                               // Deprecated                                            // for fe
	NotifyGroups          string                 `json:"-"`                                                                      // Deprecated                                            // split by space: 233 43
	NotifyGroupsObj       []UserGroup            `json:"notify_groups_obj" gorm:"-"`                                             // Deprecated                                         // for fe
	NotifyGroupsJSON      []string               `json:"notify_groups" gorm:"-"`                                                 // Deprecated                                          // for fe
	NotifyRepeatStep      int                    `json:"notify_repeat_step"`                                                     // notify repeat interval, unit: min
	NotifyMaxNumber       int                    `json:"notify_max_number"`                                                      // notify: max number
	RecoverDuration       int64                  `json:"recover_duration"`                                                       // unit: s
	Callbacks             string                 `json:"-"`                                                                      // Deprecated                                                             // split by space: http://a.com/api/x http://a.com/api/y'
	CallbacksJSON         []string               `json:"callbacks" gorm:"-"`                                                     // Deprecated                                                 // for fe
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
	TimeZone              string                 `json:"time_zone" gorm:"default:''"` // timezone for alert rule, e.g. "Asia/Shanghai", "UTC", empty for default
	NotifyRuleIds         []int64                `json:"notify_rule_ids" gorm:"serializer:json"`
	PipelineConfigs       []PipelineConfig       `json:"pipeline_configs" gorm:"serializer:json"`
	NotifyVersion         int                    `json:"notify_version"` // 0: old, 1: new
}

type ChildVarConfig struct {
	ParamVal        []map[string]ParamQuery `json:"param_val"`
	ChildVarConfigs *ChildVarConfig         `json:"child_var_configs"`
}

func (c ChildVarConfig) MarshalJSON() ([]byte, error) {
	if c.ParamVal == nil {
		c.ParamVal = []map[string]ParamQuery{}
	}
	type Alias ChildVarConfig
	return json.Marshal(Alias(c))
}

type ParamQuery struct {
	ParamType string      `json:"param_type"` // host、device、enum、threshold 三种类型
	Query     interface{} `json:"query"`
}

type VarConfig struct {
	ParamVal        []ParamQueryForFirst `json:"param_val"`
	ChildVarConfigs *ChildVarConfig      `json:"child_var_configs"`
}

func (v VarConfig) MarshalJSON() ([]byte, error) {
	if v.ParamVal == nil {
		v.ParamVal = []ParamQueryForFirst{}
	}
	type Alias VarConfig
	return json.Marshal(Alias(v))
}

// ruleConfigArrayKeys 是 rule_config 里语义上是数组的 key。
// v8.x 的类型化 marshal（老式 prom_ql 入参、Prom YAML 导入、v5 升 v6）会把这些 key 写成 null 落库，
// 前端编辑页拿到后原样回传，null 就一直留在库里；API 消费方对 null 和 [] 的处理往往不同。
// 对象类型的 key（如 child_var_configs）不在名单里：null 表示"没有下一层"，改成 {} 只会再套一层空。
var ruleConfigArrayKeys = map[string]struct{}{
	"queries":              {},
	"triggers":             {},
	"param_val":            {},
	"joins":                {},
	"on":                   {},
	"task_tpls":            {},
	"event_relabel_config": {},
}

// normalizeRuleConfigNulls 递归遍历 json.Unmarshal 到 interface{} 的 rule_config，
// 把白名单 key 下的 null 改成空数组。只处理 map / slice，类型化结构体原样返回（它们自己的 MarshalJSON 已兜底）。
// 就地修改并返回同一个值，方便链式赋值。
func normalizeRuleConfigNulls(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, val := range x {
			if val == nil {
				if _, ok := ruleConfigArrayKeys[k]; ok {
					x[k] = []interface{}{}
				}
				continue
			}
			x[k] = normalizeRuleConfigNulls(val)
		}
		return x
	case []interface{}:
		for i := range x {
			x[i] = normalizeRuleConfigNulls(x[i])
		}
		return x
	default:
		return v
	}
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
				case string:
					// 前端下发的是 id，但 AI 生成、模板导入、API 直调的规则里 values 可能是
					// 数字字符串或数据源名。静默丢弃会让规则解析不出数据源（引擎不评估、
					// test-fire 报数据源未匹配）且难以排查，这里做兼容。
					// 先按名字匹配再按数字解析：数据源名允许是纯数字（名为 "5" 的数据源 id 未必是 5），
					// 名字是用户显式配置的，优先级高于把字符串当 id 猜
					if id, ok := nameMap[v]; ok {
						val = id
					} else if n, err := strconv.ParseInt(v, 10, 64); err == nil {
						val = n
					} else {
						continue
					}
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
			if len(ids) == 0 {
				// 没有有效的 group_id，跳过该过滤项，避免生成 `group_id IN ()` 这种非法 SQL。
				continue
			}
			hasZero := false
			nonZeroIds := make([]int64, 0, len(ids))
			for _, id := range ids {
				if id == 0 {
					hasZero = true
				} else {
					nonZeroIds = append(nonZeroIds, id)
				}
			}
			// 注意：以下分支依赖 TargetFilterQueryBuild 在外层对 target_busi_group 使用 LEFT JOIN，
			// 才能让 target_ident IS NULL 表示「该 target 未归组」。如果外层换成 INNER JOIN，
			// `== [0]` 与 `!= [0]` 的语义都会被打破。
			if q.Op == "==" {
				switch {
				case hasZero && len(nonZeroIds) == 0:
					m["target_busi_group.target_ident IS NULL"] = nil
				case hasZero && len(nonZeroIds) > 0:
					m["(target_busi_group.target_ident IS NULL OR target_busi_group.group_id IN (?))"] = nonZeroIds
				default:
					m["target_busi_group.group_id in (?)"] = nonZeroIds
				}
			} else {
				switch {
				case hasZero && len(nonZeroIds) == 0:
					m["target_busi_group.target_ident IS NOT NULL"] = nil
				case hasZero && len(nonZeroIds) > 0:
					m["target_busi_group.target_ident IS NOT NULL AND NOT EXISTS (SELECT 1 FROM target_busi_group tbg WHERE tbg.target_ident = target.ident AND tbg.group_id IN (?))"] = nonZeroIds
				default:
					m["NOT EXISTS (SELECT 1 FROM target_busi_group tbg WHERE tbg.target_ident = target.ident AND tbg.group_id IN (?))"] = nonZeroIds
				}
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

	ar.Name = strings.TrimSpace(ar.Name)
	if ar.Name == "" {
		return errors.New("name is blank")
	}

	if ar.TimeZone != "" {
		_, err := time.LoadLocation(ar.TimeZone)
		if err != nil {
			return fmt.Errorf("invalid timezone: %s", ar.TimeZone)
		}
	}

	if str.Dangerous(ar.Name) {
		return errors.New("Name has invalid characters")
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

	if err := ar.ValidateRuleConfig(); err != nil {
		return err
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
	appendTagKeys := make(map[string]struct{})
	for i := 0; i < len(arr); i++ {
		if !strings.Contains(arr[i], "=") {
			return fmt.Errorf("AppendTags(%s) invalid", arr[i])
		}
		pair := strings.SplitN(arr[i], "=", 2)
		if _, exists := appendTagKeys[pair[0]]; exists {
			return fmt.Errorf("AppendTags has duplicate key: %s", pair[0])
		}
		appendTagKeys[pair[0]] = struct{}{}
	}

	gids := strings.Fields(ar.NotifyGroups)
	for i := 0; i < len(gids); i++ {
		if _, err := strconv.ParseInt(gids[i], 10, 64); err != nil {
			return fmt.Errorf("NotifyGroups(%s) invalid", ar.NotifyGroups)
		}
	}

	// 生效时段一致性校验：开始时间、结束时间、生效星期三者段数必须一致，且配了时段时星期不能为空。
	// 否则运行时 mute 判断会因三个数组长度不一致而索引越界 panic（如只选了生效星期却没配起止时间）。
	// 校验前端传入的原始数组，不依赖 FE2DB 的序列化结果。
	enableStimeCount := len(ar.EnableStimesJSON)
	if enableStimeCount == 0 && ar.EnableStimeJSON != "" {
		enableStimeCount = 1 // 兼容旧版单时段字段
	}
	enableEtimeCount := len(ar.EnableEtimesJSON)
	if enableEtimeCount == 0 && ar.EnableEtimeJSON != "" {
		enableEtimeCount = 1
	}
	// 只统计非空的星期分组：DB2FE 对未配置生效时段（空 EnableDaysOfWeek）会 append 一个空分组，
	// 未配置的规则（clone 时占绝大多数）不应被误判为时段不一致而拒绝。
	enableWeekCount := 0
	for _, days := range ar.EnableDaysOfWeeksJSON {
		if len(days) > 0 {
			enableWeekCount++
		}
	}
	if enableWeekCount == 0 && len(ar.EnableDaysOfWeekJSON) > 0 {
		enableWeekCount = 1 // 兼容旧版单分组字段 enable_days_of_week（与 FE2DB 的复数优先、复数空才回退单数保持一致）
	}
	if enableStimeCount != enableEtimeCount || enableStimeCount != enableWeekCount {
		return fmt.Errorf("invalid effective time span: start times(%d), end times(%d) and days-of-week groups(%d) must have the same count",
			enableStimeCount, enableEtimeCount, enableWeekCount)
	}

	if err := ar.ValidateEffectiveTimes(); err != nil {
		return err
	}

	if err := ar.validateCronPattern(); err != nil {
		return err
	}

	if err := ar.validateEventRelabelConfig(); err != nil {
		return err
	}

	if ar.NotifyVersion == 0 {
		// 如果是旧版本，则清空 NotifyRuleIds
		ar.NotifyRuleIds = []int64{}
	}

	if ar.NotifyVersion > 0 {
		// 如果是新版本，则清空旧的通知媒介和通知组
		ar.NotifyChannelsJSON = []string{}
		ar.NotifyGroupsJSON = []string{}
		ar.NotifyChannels = ""
		ar.NotifyGroups = ""
		ar.Callbacks = ""
		ar.CallbacksJSON = []string{}
	}

	return nil
}

type alertRuleConfigForVerify struct {
	Version  string `json:"version"`
	Severity *int   `json:"severity"`
	Queries  []struct {
		PromQl   string `json:"prom_ql"`
		Severity int    `json:"severity"`
	} `json:"queries"`
	ExpTriggerDisable bool `json:"exp_trigger_disable"`
	Triggers          []struct {
		Exp      string `json:"exp"`
		Severity int    `json:"severity"`
	} `json:"triggers"`
	NodataTrigger struct {
		Enable   bool `json:"enable"`
		Severity int  `json:"severity"`
	} `json:"nodata_trigger"`
	AnomalyTrigger struct {
		Enable   bool `json:"enable"`
		Severity int  `json:"severity"`
	} `json:"anomaly_trigger"`
}

func validateAlertRuleSeverity(field string, severity int) error {
	if severity < SeverityEmergency || severity > SeverityNotice {
		return fmt.Errorf("%s(%d) invalid: severity must be 1, 2 or 3", field, severity)
	}
	return nil
}

// ValidateRuleConfig checks the rule_config fields every alert event depends
// on: severities must be 1, 2 or 3 and the query/trigger expressions must not
// be blank. The top-level AlertRule.Severity and rule_config.severity are
// legacy shadow fields and may legitimately be zero when a rule uses queries
// or triggers. Non-zero values in those fields must still be valid severities.
func (ar *AlertRule) ValidateRuleConfig() error {
	if ar.Severity != 0 {
		if err := validateAlertRuleSeverity("severity", ar.Severity); err != nil {
			return err
		}
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(ar.RuleConfig), &object); err != nil {
		// RuleConfig is intentionally polymorphic. Other validators own its
		// overall shape; this validation only applies to JSON objects.
		return nil
	}

	var config alertRuleConfigForVerify
	if err := json.Unmarshal([]byte(ar.RuleConfig), &config); err != nil {
		return fmt.Errorf("invalid rule_config: %v", err)
	}

	// Host rules are keyed by prod, not cate: Verify defaults an empty cate to
	// prometheus and the engine dispatches host rules by prod (GetRuleType), so
	// classifying by cate alone would validate a host rule as prom v1.
	isHost := ar.Prod == HOST || ar.Cate == HOST
	isPromV1 := !isHost && (ar.Cate == PROMETHEUS || ar.Cate == LOKI) && config.Version != "v2"
	legacyConfigSeverityActive := isPromV1 && len(config.Queries) == 0
	if config.Severity != nil && (legacyConfigSeverityActive || *config.Severity != 0) {
		if err := validateAlertRuleSeverity("rule_config.severity", *config.Severity); err != nil {
			return err
		}
	}

	// Skip the per-query checks when the anomaly trigger is enabled: legacy
	// anomaly rules (n9e-plus, prod=anomaly) are migrated into queries that
	// are algorithm inputs without prom_ql/severity — their event severity
	// lives in anomaly_trigger and is validated below.
	if isPromV1 && len(config.Queries) > 0 && !config.AnomalyTrigger.Enable {
		for i := range config.Queries {
			if strings.TrimSpace(config.Queries[i].PromQl) == "" {
				return fmt.Errorf("rule_config.queries[%d].prom_ql is blank", i)
			}
			if err := validateAlertRuleSeverity(fmt.Sprintf("rule_config.queries[%d].severity", i), config.Queries[i].Severity); err != nil {
				return err
			}
		}
	} else if !isPromV1 {
		// Host rules describe triggers with type/duration/percent instead of
		// an expression, so the exp requirement only applies to other cates
		// with threshold conditions enabled.
		expRequired := !isHost && !config.ExpTriggerDisable
		for i := range config.Triggers {
			if expRequired && strings.TrimSpace(config.Triggers[i].Exp) == "" {
				return fmt.Errorf("rule_config.triggers[%d].exp is blank", i)
			}
			if err := validateAlertRuleSeverity(fmt.Sprintf("rule_config.triggers[%d].severity", i), config.Triggers[i].Severity); err != nil {
				return err
			}
		}
	}

	if config.NodataTrigger.Enable {
		if err := validateAlertRuleSeverity("rule_config.nodata_trigger.severity", config.NodataTrigger.Severity); err != nil {
			return err
		}
	}

	if config.AnomalyTrigger.Enable {
		if err := validateAlertRuleSeverity("rule_config.anomaly_trigger.severity", config.AnomalyTrigger.Severity); err != nil {
			return err
		}
	}

	return nil
}

// hhmmPattern 严格匹配 24 小时制 HH:MM。不接受 8:00 这类缺前导零的写法：生效时段在运行时是把
// 当前时间格式化成 HH:MM 后按字符串直接比大小的，位数不齐会让时段判断出错。
var hhmmPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// hhmmEndPattern 在 HH:MM 之外额外放行结束时间 24:00：同样是字符串比大小，触发时刻最大只到 23:59，
// 恒小于 24:00，所以 02:00-24:00 表示生效到当日结束，是 mute.go/dispatch.go 支持的既有写法。
var hhmmEndPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$|^24:00$`)

// ValidateEffectiveTimes 校验生效时段的起止时间格式。前端用 moment 格式化时间，moment 拿到脏数据
// 会格式化出 「Invalid date」 这类字符串并原样提交，存进 DB 后按空格切分会让时段数组长度错乱。
// 取值口径与上面的段数校验一致：复数字段为空时回退到已废弃的单数字段。
func (ar *AlertRule) ValidateEffectiveTimes() error {
	stimes := ar.EnableStimesJSON
	if len(stimes) == 0 && ar.EnableStimeJSON != "" {
		stimes = []string{ar.EnableStimeJSON}
	}

	etimes := ar.EnableEtimesJSON
	if len(etimes) == 0 && ar.EnableEtimeJSON != "" {
		etimes = []string{ar.EnableEtimeJSON}
	}

	for i := range stimes {
		if !hhmmPattern.MatchString(stimes[i]) {
			return fmt.Errorf("invalid effective time span %d: start time(%s) must be in HH:MM format", i+1, stimes[i])
		}
	}

	for i := range etimes {
		if !hhmmEndPattern.MatchString(etimes[i]) {
			return fmt.Errorf("invalid effective time span %d: end time(%s) must be in HH:MM format", i+1, etimes[i])
		}
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

// relabelConfigForVerify 用宽松类型接收 event_relabel_config，只为把非法 label name
// 定位到具体第几条配置：直接解成 pconf.RelabelConfig 的话，它的 SourceLabels 是
// model.LabelNames，遇到非法 label name 会中途中止，拿不到出错的下标。
type relabelConfigForVerify struct {
	SourceLabels []string `json:"source_labels"`
}

// validateEventRelabelConfig 保证 rule_config.event_relabel_config 能被读取端解开。
// 读取端用的是 pconf.RelabelConfig，source_labels 里的非法 label name（空串、数字开头等）
// 会让它的 UnmarshalJSON 失败：center 侧表现为 event_relabel_config 被静默改坏，edge 侧表现为
// 整批告警规则同步失败、进程退出。字段类型不合法（如 modulus 传字符串）虽不会打挂
// edge，但读取端同样整段丢弃，表现为"保存成功但 relabel 永远不生效"。两类都拦在写入口。
func (ar *AlertRule) validateEventRelabelConfig() error {
	if ar.RuleConfig == "" {
		return nil
	}

	// 用 RawMessage 接住 relabel 段：rule_config 的整体结构由各 cate 自己定义，
	// 这里只关心 relabel 部分，其余部分解不出来就不越权报错
	var ruleConfig struct {
		EventRelabelConfig json.RawMessage `json:"event_relabel_config"`
	}
	if err := json.Unmarshal([]byte(ar.RuleConfig), &ruleConfig); err != nil {
		return nil
	}

	if len(ruleConfig.EventRelabelConfig) == 0 {
		return nil
	}

	// 宽松类型解得开时，优先给出带下标的报错
	var looseConfigs []*relabelConfigForVerify
	if err := json.Unmarshal(ruleConfig.EventRelabelConfig, &looseConfigs); err == nil {
		for i, cfg := range looseConfigs {
			if cfg == nil {
				continue
			}

			// 只校验 source_labels：它在读取端是 model.LabelNames，非法 label name 会让
			// 反序列化失败。target_label 在读取端是普通 string，不参与反序列化校验，
			// 且运行期 lowercase/uppercase/hashmod 等分支会原样写出（含点号的标签正是
			// relabel.go 里 REPLACE_DOT 机制要支持的），所以这里不能收得比读取端更严。
			for _, sourceLabel := range cfg.SourceLabels {
				if !model.LabelName(sourceLabel).IsValid() {
					return fmt.Errorf("event_relabel_config[%d]: %q is not a valid label name in source_labels", i, sourceLabel)
				}
			}
		}
	}

	// 最后按读取端的真实类型解一遍，判定标准与 DB2FE 完全一致
	var configs []*pconf.RelabelConfig
	if err := json.Unmarshal(ruleConfig.EventRelabelConfig, &configs); err != nil {
		return fmt.Errorf("invalid event_relabel_config: %v", err)
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

// Upsert: 同 group 内若存在同名规则则覆盖（保留原 id/create_at/create_by，下游引用不破），否则插入。
// 用于 force=true 的批量导入场景。调用方传入的 ar 不要预先调用 FE2DB —— 本方法内部处理。
func (ar *AlertRule) Upsert(ctx *ctx.Context) error {
	var existing AlertRule
	err := DB(ctx).Where("group_id = ? AND name = ?", ar.GroupId, ar.Name).Take(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := ar.FE2DB(); err != nil {
			return err
		}
		return ar.Add(ctx) // 复用 Add 的 Verify + 同名兜底（防 SELECT 与 INSERT 之间的并发插入）
	}

	return existing.Update(ctx, *ar) // Update 内部会 FE2DB、Verify，并保留 existing 的 id/create_at/create_by
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
		severityValue, ok := value.(float64)
		if !ok || severityValue != float64(int(severityValue)) {
			return fmt.Errorf("severity(%v) invalid: severity must be 1, 2 or 3", value)
		}
		severity := int(severityValue)
		if err := validateAlertRuleSeverity("severity", severity); err != nil {
			return err
		}
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

	if column == "notify_rule_ids" {
		updates := map[string]interface{}{
			"notify_version":  1,
			"notify_channels": "",
			"notify_groups":   "",
			"notify_rule_ids": value,
		}
		return DB(ctx).Model(ar).Updates(updates).Error
	}

	if column == "notify_groups" || column == "notify_channels" {
		updates := map[string]interface{}{
			column:            value,
			"notify_version":  0,
			"notify_rule_ids": []int64{},
		}
		return DB(ctx).Model(ar).Updates(updates).Error
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

	for i := range ar.AppendTagsJSON {
		// 后面要把多个标签拼接在一起，所以每个标签里不能有空格
		ar.AppendTagsJSON[i] = strings.ReplaceAll(ar.AppendTagsJSON[i], " ", "")
	}

	if len(ar.AppendTagsJSON) > 0 {
		ar.AppendTags = strings.Join(ar.AppendTagsJSON, " ")
	}

	algoParamsByte, err := json.Marshal(ar.AlgoParamsJson)
	if err != nil {
		return fmt.Errorf("marshal algo_params err:%v", err)
	}
	ar.AlgoParams = string(algoParamsByte)

	// 老的规则，是 PromQl 和 Severity 字段，新版的规则，使用 RuleConfig 字段
	if ar.RuleConfigJson == nil || len(ar.PromQl) > 0 {
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
		// 写侧顺手洗掉前端回传的 null，新落库的数据不再带 null（读侧 DB2FE 仍会兜底存量）
		ar.RuleConfigJson = normalizeRuleConfigNulls(ar.RuleConfigJson)
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

	if ar.NotifyRuleIds == nil {
		ar.NotifyRuleIds = make([]int64, 0)
	}

	ar.NotifyChannelsJSON = strings.Fields(ar.NotifyChannels)
	ar.NotifyGroupsJSON = strings.Fields(ar.NotifyGroups)
	ar.CallbacksJSON = strings.Fields(ar.Callbacks)
	ar.AppendTagsJSON = strings.Fields(ar.AppendTags)
	json.Unmarshal([]byte(ar.AlgoParams), &ar.AlgoParamsJson)
	json.Unmarshal([]byte(ar.RuleConfig), &ar.RuleConfigJson)
	json.Unmarshal([]byte(ar.Annotations), &ar.AnnotationsJSON)
	json.Unmarshal([]byte(ar.ExtraConfig), &ar.ExtraConfigJSON)
	// 存量 rule_config 里的 null 数组（如 var_config.param_val）统一归成 []
	ar.RuleConfigJson = normalizeRuleConfigNulls(ar.RuleConfigJson)

	// 解析 RuleConfig 字段
	// 空 rule_config 在老库里是存量数据（该列 text not null 无默认值），不是异常，直接跳过，
	// 否则每次列表接口都会为这些行刷一条 Warning。
	if ar.RuleConfig != "" {
		var ruleConfig struct {
			EventRelabelConfig []*pconf.RelabelConfig `json:"event_relabel_config"`
		}
		if err := json.Unmarshal([]byte(ar.RuleConfig), &ruleConfig); err != nil {
			// 这里的错误不能吞：SourceLabels 是 model.LabelNames，遇到非法 label name 会
			// 中途中止，留下一个被改坏的结构体（已扩容但未赋值的元素变成空串，报错位置之后的
			// 字段全丢）。把这份数据发给 edge，edge 反序列化整批规则都会失败并退出进程，
			// 所以宁可整段置空，也不能把伪造出来的值传下去。
			logger.Warningf("alert rule(id=%d name=%s) decode event_relabel_config failed, dropped: %v",
				ar.Id, ar.Name, err)
			ar.EventRelabelConfig = nil
		} else {
			ar.EventRelabelConfig = ruleConfig.EventRelabelConfig
		}
	}

	// 兼容旧逻辑填充 cron_pattern
	if ar.CronPattern == "" && ar.PromEvalInterval != 0 {
		ar.CronPattern = fmt.Sprintf("@every %ds", ar.PromEvalInterval)
	}

	err := ar.FillDatasourceQueries()
	if err != nil {
		return err
	}

	ar.FillSeverities()

	// 数组 / map 字段对外统一返回 [] / {}，不返回 null：
	// serializer:json 列为空、gorm:"-" 的派生字段没填、annotations 列为空串时这些字段都是 nil。
	// Go 侧消费者（edge 同步、引擎）对 nil 和空切片的处理完全一致，只影响 JSON 形状。
	if ar.DatasourceQueries == nil {
		ar.DatasourceQueries = []DatasourceQuery{}
	}
	if ar.EventRelabelConfig == nil {
		ar.EventRelabelConfig = []*pconf.RelabelConfig{}
	}
	if ar.NotifyGroupsObj == nil {
		ar.NotifyGroupsObj = []UserGroup{}
	}
	// pipeline_configs 与 severities 故意不归一：这两个字段前端都是用真值兜底的，[] 是真值、null 才是假值，归一会让兜底失效。
	// pipeline_configs：编辑页 `pipeline_configs ?? [{enable:true}]` 靠 null 出默认工作流行，[] 会让工作流区空白且无法添加。
	// severities：列表页筛选是 `(item.severities && ...) || !item.severities`，[] 会让推不出严重度的规则整条从列表消失
	// （rule_config 为空串、或非 prom 规则 triggers 为空时 FillSeverities 一个都 append 不出来）。该字段 gorm:"-" 且只给前端用。
	if ar.AnnotationsJSON == nil {
		ar.AnnotationsJSON = map[string]string{}
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

	return len(lst) > 0, nil
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

// AlertRuleGetsLegacyNotifyByBGIds 查老式通知配置（notify_version=0）的告警规则。
// 用于迁移审计场景：把 notify_version 过滤下推到 SQL，避免全表拉回再内存过滤。
// includeDisabled=false 时同时过滤 disabled=0。
func AlertRuleGetsLegacyNotifyByBGIds(ctx *ctx.Context, bgids []int64, includeDisabled bool) ([]AlertRule, error) {
	session := DB(ctx).Where("notify_version = ?", 0)
	if len(bgids) > 0 {
		session = session.Where("group_id in (?)", bgids)
	}
	if !includeDisabled {
		session = session.Where("disabled = ?", 0)
	}

	var lst []AlertRule
	err := session.Order("name").Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].DB2FE()
		}
	}

	return lst, err
}

// rawAlertRule 与 json.RawMessage 一样延迟解码，额外实现 String()，
// 避免 poster 里 %+v 的 debug 日志把整批规则打成字节十进制数组。
type rawAlertRule []byte

func (r *rawAlertRule) UnmarshalJSON(b []byte) error {
	*r = append((*r)[:0], b...)
	return nil
}

func (r rawAlertRule) String() string {
	return string(r)
}

func AlertRuleGetsAll(ctx *ctx.Context) ([]*AlertRule, error) {
	if !ctx.IsCenter {
		// 逐条反序列化，而不是一次解成 []*AlertRule：单条规则里的脏数据（例如
		// event_relabel_config 里的非法 label name）会让整个响应解码失败，进而
		// 导致边缘机房一条规则都同步不到、启动阶段直接 exit。坏规则跳过并告警，
		// 其余规则照常生效。
		raws, err := poster.GetByUrls[[]rawAlertRule](ctx, "/v1/n9e/alert-rules?disabled=0")
		if err != nil {
			return nil, err
		}

		lst := make([]*AlertRule, 0, len(raws))
		for i := 0; i < len(raws); i++ {
			var ar AlertRule
			if err := json.Unmarshal(raws[i], &ar); err != nil {
				logger.Errorf("failed to decode alert rule, skipped: %v, raw: %s", err, string(raws[i]))
				continue
			}
			ar.FE2DB()
			lst = append(lst, &ar)
		}

		if skipped := len(raws) - len(lst); skipped > 0 {
			logger.Errorf("%d of %d alert rules skipped due to decode failure", skipped, len(raws))
		}

		// 一条都没解出来，说明不是个别脏数据而是响应整体不可用（如 center 与 edge 版本不一致）。
		// 此时必须返回 error：调用方据此保留旧缓存并继续重试，否则规则缓存会被清空、
		// 同步水位又被刷成最新，告警全停且不再重试。
		if len(raws) > 0 && len(lst) == 0 {
			return nil, fmt.Errorf("all %d alert rules failed to decode", len(raws))
		}

		return lst, nil
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

func (ar *AlertRule) IsTdengineRule() bool {
	return ar.Cate == TDENGINE
}

func (ar *AlertRule) IsHostRule() bool {
	return ar.Prod == HOST
}

func (ar *AlertRule) IsInnerRule() bool {
	return ar.Cate == TDENGINE ||
		ar.Cate == IOTDB ||
		ar.Cate == CLICKHOUSE ||
		ar.Cate == ELASTICSEARCH ||
		ar.Prod == LOKI || ar.Cate == LOKI ||
		ar.Cate == MYSQL ||
		ar.Cate == POSTGRESQL ||
		ar.Cate == DORIS ||
		ar.Cate == OPENSEARCH ||
		ar.Cate == VICTORIALOGS
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

// HostAlertRuleTargetsSig 计算 host 告警规则 -> target 映射的输入签名。
// 该映射由三部分输入共同决定，任一变化都会改变结果，签名随之变化：
//  1. host 告警规则（增删/改/启停，都会改变 count 或 max(update_at)）
//  2. 机器与业务组的归属关系 target_busi_group
//  3. 机器自身的 tags/host_tags 等字段（变更时 router_heartbeat 会 bump target.update_at）
//
// 供 memsto 在每轮同步前做变更检测：签名未变即可跳过整轮规则查询，
// 避免每个周期对所有 host 规则各发一条全表扫描的过滤 SQL。仅 center 直连 DB 时调用。
func HostAlertRuleTargetsSig(ctx *ctx.Context) (string, error) {
	var ruleStat []*Statistics
	err := DB(ctx).Model(&AlertRule{}).
		Select("count(*) as total", "max(update_at) as last_updated").
		Where("prod = ?", HOST).Find(&ruleStat).Error
	if err != nil {
		return "", err
	}

	bgStat, err := StatisticsGet(ctx, &TargetBusiGroup{})
	if err != nil {
		return "", err
	}

	tgStat, err := StatisticsGet(ctx, &Target{})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%d:%d|%d:%d|%d:%d",
		ruleStat[0].Total, ruleStat[0].LastUpdated,
		bgStat.Total, bgStat.LastUpdated,
		tgStat.Total, tgStat.LastUpdated), nil
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

// 复制告警策略，需要提供操作者名称和新的业务组ID
func (ar *AlertRule) Clone(operatorName string, newBgid int64) *AlertRule {
	newAr := ar

	newAr.Id = 0
	newAr.GroupId = newBgid
	newAr.Name = ar.Name
	newAr.UpdateBy = operatorName
	newAr.UpdateAt = time.Now().Unix()
	newAr.CreateBy = operatorName
	newAr.CreateAt = time.Now().Unix()

	return newAr
}
