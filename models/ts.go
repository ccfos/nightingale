package models

import "github.com/prometheus/common/model"

type DataResp struct {
	Ref    string       `json:"ref"`
	Metric model.Metric `json:"metric"`
	Labels string       `json:"-"`
	Values [][]float64  `json:"values"`
}

func (d *DataResp) Last() (float64, float64, bool) {
	if len(d.Values) == 0 {
		return 0, 0, false
	}

	lastValue := d.Values[len(d.Values)-1]
	if len(lastValue) != 2 {
		return 0, 0, false
	}
	return lastValue[0], lastValue[1], true
}

func (d *DataResp) MetricName() string {
	metric := d.Metric["__name__"]
	return string(metric)
}

type RelationKey struct {
	LeftKey  string `json:"left_key"`
	RightKey string `json:"right_key"`
	OP       string `json:"op"`
}

type QueryParam struct {
	Cate         string        `json:"cate"`
	DatasourceId int64         `json:"datasource_id"`
	Querys       []interface{} `json:"query"`
}

type Series struct {
	SeriesStore map[uint64]DataResp            `josn:"store"`
	SeriesIndex map[string]map[uint64]struct{} `json:"index"`
}
