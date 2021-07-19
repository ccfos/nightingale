package vos

import (
	"fmt"
	"math"
	"time"
)

type JsonFloat float64

func (v JsonFloat) MarshalJSON() ([]byte, error) {
	f := float64(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return []byte("null"), nil
	} else {
		return []byte(fmt.Sprintf("%f", f)), nil
	}
}

type Point struct {
	Timestamp int64     `json:"t"`
	Value     JsonFloat `json:"v"`
}

func NewPoint(ts int64, val float64) *Point {
	return &Point{Timestamp: ts, Value: JsonFloat(val)}
}

type DataQueryParam struct {
	Params []DataQueryParamOne `json:"params"`
	Limit  int                 `json:"limit"`
	Start  int64               `json:"start"`
	End    int64               `json:"end"`
}

type DataQueryInstanceParam struct {
	PromeQl string `json:"prome_ql"`
}

type DataQueryParamOne struct {
	PromeQl          string     `json:"prome_ql"`
	Idents           []string   `json:"idents"`
	ClasspathId      int64      `json:"classpath_id"`
	ClasspathPrefix  int        `json:"classpath_prefix"`
	Metric           string     `json:"metric"`
	TagPairs         []*TagPair `json:"tags"`
	DownSamplingFunc string     `json:"down_sampling_func"`
	Aggr             AggrConf   `json:"aggr"`
	Comparisons      []int64    `json:"comparisons"` //环比多少时间
}

type AggrConf struct {
	GroupKey []string `json:"group_key"`                          //聚合维度
	Func     string   `json:"func" description:"sum,avg,max,min"` //聚合计算
}

type DataQueryResp struct {
	Ident      string   `json:"ident"`
	Metric     string   `json:"metric"`
	Tags       string   `json:"tags"`
	Values     []*Point `json:"values"`
	Resolution int64    `json:"resolution"`
	PNum       int      `json:"pNum"`
}

type DataQueryInstanceResp struct {
	Metric map[string]interface{} `json:"metric"`
	Value  []float64              `json:"value"`
}

type DataQL struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	QL    string `json:"ql"`
	Step  int64  `json:"step"`
}

type TagKeyQueryParam struct {
	Idents         []string   `json:"idents"`
	TagKey         string     `json:"tagkey"`
	TagPairs       []*TagPair `json:"tags"`
	Metric         string     `json:"metric"`
	Start          int64      `json:"start" description:"inclusive"`
	End            int64      `json:"end" description:"exclusive"`
	StartInclusive time.Time  `json:"-"`
	EndExclusive   time.Time  `json:"-"`
}

func (p *TagKeyQueryParam) Validate() (err error) {
	p.StartInclusive, p.EndExclusive, err = timeRangeValidate(p.Start, p.End)
	return
}

type TagKeyQueryResp struct {
	Keys []string `json:"keys"`
}

type TagValueQueryParam struct {
	TagKey         string    `json:"tagkey"`
	TagValue       string    `json:"value"`
	Metric         string    `json:"metric"`
	Idents         []string  `json:"idents"`
	Tags           []string  `json:"tags"`
	Start          int64     `json:"start" description:"inclusive"`
	End            int64     `json:"end" description:"exclusive"`
	StartInclusive time.Time `json:"-"`
	EndExclusive   time.Time `json:"-"`
}

func (p *TagValueQueryParam) Validate() (err error) {
	p.StartInclusive, p.EndExclusive, err = timeRangeValidate(p.Start, p.End)
	return
}

type PromQlCheckResp struct {
	ParseError string `json:"parse_error"`
	QlCorrect  bool   `json:"ql_correct"`
}

type TagValueQueryResp struct {
	Values []string `json:"values"`
}

type TagPairQueryParamOne struct {
	Idents []string `json:"idents"`
	Metric string   `json:"metric"`
}

type TagPairQueryParam struct {
	Params         []TagPairQueryParamOne `json:"params"`
	TagPairs       []*TagPair             `json:"tags"`
	Start          int64                  `json:"start" description:"inclusive"`
	End            int64                  `json:"end" description:"exclusive"`
	StartInclusive time.Time              `json:"-"`
	EndExclusive   time.Time              `json:"-"`
	Limit          int                    `json:"limit"`
}

type CommonTagQueryParam struct {
	Params         []TagPairQueryParamOne `json:"params"`
	TagPairs       []*TagPair             `json:"tags"`
	TagKey         string                 `json:"tag_key"`   // 查询目标key，或者模糊查询
	TagValue       string                 `json:"tag_value"` // 根据标签key查询value，或者模糊查询
	Start          int64                  `json:"start" description:"inclusive"`
	End            int64                  `json:"end" description:"exclusive"`
	StartInclusive time.Time              `json:"-"`
	EndExclusive   time.Time              `json:"-"`
	Limit          int                    `json:"limit"`
}

func (p *CommonTagQueryParam) Validate() (err error) {
	p.StartInclusive, p.EndExclusive, err = timeRangeValidate(p.Start, p.End)
	return
}

type TagPairQueryResp struct {
	Idents   []string `json:"idents"`
	Metric   string   `json:"metric"`
	TagPairs []string `json:"tags"`
}

type MetricQueryParam struct {
	Idents         []string   `json:"idents"`
	Metric         string     `json:"metric"`
	TagPairs       []*TagPair `json:"tags"`
	Start          int64      `json:"start" description:"inclusive"`
	End            int64      `json:"end" description:"exclusive"`
	StartInclusive time.Time  `json:"-"`
	EndExclusive   time.Time  `json:"-"`
	Limit          int        `json:"limit"`
}

func (p *MetricQueryParam) Validate() (err error) {
	p.StartInclusive, p.EndExclusive, err = timeRangeValidate(p.Start, p.End)
	return
}

type MetricQueryResp struct {
	Metrics []string `json:"metrics"`
}

type MetricDesQueryResp struct {
	Metrics []MetricsWithDescription `json:"metrics"`
}

type MetricsWithDescription struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TagPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type IndexQueryParam struct {
	Metric         string     `json:"metric"`
	Idents         []string   `json:"idents"`
	Include        []*TagPair `json:"include"`
	Exclude        []*TagPair `json:"exclude"`
	Start          int64      `json:"start" description:"inclusive"`
	End            int64      `json:"end" description:"exclusive"`
	StartInclusive time.Time  `json:"-"`
	EndExclusive   time.Time  `json:"-"`
}

func (p *IndexQueryParam) Validate() (err error) {
	p.StartInclusive, p.EndExclusive, err = timeRangeValidate(p.Start, p.End)
	return
}

type IndexQueryResp struct {
	Metric string            `json:"metric"`
	Ident  string            `json:"ident"`
	Tags   map[string]string `json:"tags"`
}

func timeRangeValidate(start, end int64) (startInclusive, endExclusive time.Time, err error) {
	if end == 0 {
		endExclusive = time.Now()
	} else {
		endExclusive = time.Unix(end, 0)
	}

	if start == 0 {
		startInclusive = endExclusive.Add(-time.Hour * 25)
	} else {
		startInclusive = time.Unix(start, 0)
	}

	if startInclusive.After(endExclusive) {
		err = fmt.Errorf("start is after end")
	}

	return
}
