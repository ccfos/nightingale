package dataobj

import (
	"time"
)

type AggrCalc struct {
	Id               int64     `xorm:"id pk autoincr" json:"id"`
	Nid              int64     `xorm:"nid" json:"nid"`
	Category         int       `xorm:"category" json:"category"`
	NewMetric        string    `xorm:"new_metric" json:"new_metric"`
	NewStep          int       `xorm:"new_step" json:"new_step"`
	GroupByString    string    `xorm:"groupby" json:"-"`
	RawMetricsString string    `xorm:"raw_metrics" json:"-"`
	GlobalOperator   string    `xorm:"global_operator"json:"global_operator"` //指标聚合方式
	Expression       string    `xorm:"expression" json:"expression"`
	RPN              string    `xorm:"rpn" json:"rpn"`
	Status           int       `xorm:"status" json:"-"`
	Quota            int       `xorm:"quota" json:"quota"`
	Comment          string    `xorm:"comment" json:"comment"`
	Creator          string    `xorm:"creator" json:"creator"`
	Created          time.Time `xorm:"created" json:"created"`
	LastUpdator      string    `xorm:"last_updator" json:"last_updator"`
	LastUpdated      time.Time `xorm:"<-" json:"last_updated"`

	RawMetrics []*RawMetric `xorm:"-" json:"raw_metrics"`
	GroupBy    []string     `xorm:"-" json:"groupby"`
}

type RawMetric struct {
	Nid       int64             `json:"nid"`
	ExclNid   []int64           `json:"excl_nid"`
	Endpoints []string          `json:"endpoints"`
	Nids      []string          `json:"nids"`
	VarID     string            `json:"var_id"`
	Name      string            `json:"name"`
	Opt       string            `json:"opt"`
	Filters   []*AggrTagsFilter `json:"filters"`
}

type AggrTagsFilter struct {
	TagK string   `json:"tagk"`
	Opt  string   `json:"opt"`
	TagV []string `json:"tagv"`
}

type RawMetricAggrCalc struct {
	Sid            int64             `json:"sid"`
	Nid            int64             `json:"nid"`
	NewMetric      string            `json:"newMetric"`
	NewStep        int               `json:"newStep"`
	GroupBy        []string          `json:"groupBy"`
	GlobalOperator string            `json:"globalOperator"`
	InnerOperator  string            `json:"innerOperator"`
	VarID          string            `json:"varID"`
	VarNum         int               `json:"varNum"`
	SourceNid      int64             `json:"source_nid"`
	SourceMetric   string            `json:"sourceMetric"`
	RPN            string            `json:"RPN"`
	TagFilters     []*AggrTagsFilter `json:"tagFilters"`
	Lateness       int               `json:"lateness"`
}

type AggrList struct {
	Data []*CentralAggrV2Point `json:"data"`
}

type CentralAggrV2Point struct {
	Timestamp int64           `json:"t"`
	Value     float64         `json:"v"`
	Strategys []*AggrCalcStra `json:"s"`
	Hash      uint64          `json:"h"`
}

type AggrCalcStra struct {
	SID            int64  `json:"sid"`
	NID            string `json:"nid"`
	ResultStep     int    `json:"step"`
	RawStep        int    `json:"rawStep"`
	GroupKey       string `json:"key"`
	GlobalOperator string `json:"global"`
	InnerOperator  string `json:"inner"`
	VarID          string `json:"varID"`
	VarNum         int    `json:"varNum"`
	RPN            string `json:"RPN"`
	Lateness       int    `json:"lateness"`
}

type AggrOut struct {
	Index   string `json:"index"`
	Operate int    `json:"operate"`
	Data    struct {
		Nid       string      `json:"nid"`
		Step      int64       `json:"step"`
		GroupTag  string      `json:"groupTag"`
		Value     interface{} `json:"value"`
		Sid       int64       `json:"sid"`
		Timestamp int64       `json:"timestamp"`
	} `json:"data"`
	Type string `json:"type"`
}
