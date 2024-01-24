package models

import (
	"math"

	"github.com/prometheus/common/model"
)

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

func Convert2DataResp(value model.Value, ref ...string) (resp []DataResp) {
	if value == nil {
		return
	}

	switch value.Type() {
	case model.ValMatrix:
		matrix, ok := value.(model.Matrix)
		if !ok {
			return
		}

		for _, stream := range matrix {
			if len(stream.Values) == 0 {
				continue // 如果一个系列没有值，我们跳过而不是返回
			}

			// 填充 DataResp 结构
			var pairs []model.SamplePair
			for _, sample := range stream.Values {
				if !math.IsNaN(float64(sample.Value)) { // 只有当值不是 NaN 时才添加
					pairs = append(pairs, sample)
				}
			}

			// 如果一个系列所有的样本都是 NaN，我们跳过这个系列
			if len(pairs) == 0 {
				continue
			}

			// 我们可以选择添加额外的逻辑来填充 Ref 和 Labels，如果需要
			item := DataResp{
				Metric: stream.Metric,
				// Labels: ..., // 这需要额外的上下文来确定如何填充
				// Ref: ..., // 这需要额外的上下文来确定如何填充
			}

			if len(ref) > 0 {
				item.Ref = ref[0]
			}

			for _, sample := range pairs {
				item.Values = append(item.Values, []float64{float64(sample.Timestamp.Unix()), float64(sample.Value)})
			}

			resp = append(resp, item)
		}
	default:
		return
	}

	return
}
