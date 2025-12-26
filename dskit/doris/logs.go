package doris

import (
	"context"
	"sort"
)

// 日志相关的操作
const (
	TimeseriesAggregationTimestamp = "__ts__"
)

// QueryLogs 查询日志
// TODO: 待测试, MAP/ARRAY/STRUCT/JSON 等类型能否处理
func (d *Doris) QueryLogs(ctx context.Context, query *QueryParam) ([]map[string]interface{}, error) {
	// 等同于 Query()
	return d.Query(ctx, query, true)
}

// QueryHistogram 本质是查询时序数据, 取第一组, SQL由上层封装, 不再做复杂的解析和截断
func (d *Doris) QueryHistogram(ctx context.Context, query *QueryParam) ([][]float64, error) {
	values, err := d.QueryTimeseries(ctx, query)
	if err != nil {
		return [][]float64{}, nil
	}
	if len(values) > 0 && len(values[0].Values) > 0 {
		items := values[0].Values
		sort.Slice(items, func(i, j int) bool {
			if len(items[i]) > 0 && len(items[j]) > 0 {
				return items[i][0] < items[j][0]
			}
			return false
		})
		return items, nil
	}
	return [][]float64{}, nil
}
