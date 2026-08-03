package eval

import (
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/evallog"

	"github.com/prometheus/common/model"
)

// evalLogSamplesFromValue 将 prom 查询结果转为 evallog 采样，按 SeriesCap 提前截断避免大结果集的无谓分配。
// 返回 (采样, 真实总数)。
func evalLogSamplesFromValue(value model.Value) ([]evallog.SeriesSample, int) {
	cap := evallog.SeriesCap()
	if cap <= 0 || value == nil {
		return nil, 0
	}

	var samples []evallog.SeriesSample
	total := 0
	switch value.Type() {
	case model.ValVector:
		items, ok := value.(model.Vector)
		if !ok {
			return nil, 0
		}
		total = len(items)
		for i := 0; i < len(items) && i < cap; i++ {
			samples = append(samples, evallog.SeriesSample{
				Labels: metricToMap(items[i].Metric),
				Points: [][2]float64{{float64(items[i].Timestamp.Unix()), float64(items[i].Value)}},
			})
		}
	case model.ValMatrix:
		items, ok := value.(model.Matrix)
		if !ok {
			return nil, 0
		}
		total = len(items)
		for i := 0; i < len(items) && i < cap; i++ {
			var points [][2]float64
			for _, v := range items[i].Values {
				points = append(points, [2]float64{float64(v.Timestamp.Unix()), float64(v.Value)})
			}
			samples = append(samples, evallog.SeriesSample{
				Labels: metricToMap(items[i].Metric),
				Points: points,
			})
		}
	}
	return samples, total
}

// evalLogSamplesFromDataResps 将非 prom 数据源查询结果转为 evallog 采样。
func evalLogSamplesFromDataResps(series []models.DataResp) ([]evallog.SeriesSample, int) {
	cap := evallog.SeriesCap()
	if cap <= 0 {
		return nil, 0
	}

	var samples []evallog.SeriesSample
	for i := 0; i < len(series) && i < cap; i++ {
		var points [][2]float64
		for _, v := range series[i].Values {
			if len(v) >= 2 {
				points = append(points, [2]float64{v[0], v[1]})
			}
		}
		samples = append(samples, evallog.SeriesSample{
			Labels: metricToMap(series[i].Metric),
			Points: points,
		})
	}
	return samples, len(series)
}

// evalLogSamplesFromPoints 将异常点转为 evallog 采样（变量填充查询的聚合现场）。
func evalLogSamplesFromPoints(points []models.AnomalyPoint) ([]evallog.SeriesSample, int) {
	cap := evallog.SeriesCap()
	if cap <= 0 {
		return nil, 0
	}

	var samples []evallog.SeriesSample
	for i := 0; i < len(points) && i < cap; i++ {
		samples = append(samples, evallog.SeriesSample{
			Labels: metricToMap(points[i].Labels),
			Points: [][2]float64{{float64(points[i].Timestamp), points[i].Value}},
		})
	}
	return samples, len(points)
}

// evalLogAnomalies 将本周期判定结果写入记录。
func evalLogAnomalies(rec *evallog.EvalRecord, points []models.AnomalyPoint, recover bool) {
	if rec == nil {
		return
	}
	for i := range points {
		rec.AddAnomaly(evallog.AnomalyBrief{
			Key:         points[i].Key,
			Value:       points[i].Value,
			Severity:    points[i].Severity,
			TriggerType: string(points[i].TriggerType),
			Recover:     recover,
		})
	}
}

// evalLogQueryString 将非 prom 数据源的查询对象序列化为字符串。
func evalLogQueryString(query interface{}) string {
	b, err := json.Marshal(query)
	if err != nil {
		return fmt.Sprintf("%+v", query)
	}
	return string(b)
}

func metricToMap(m model.Metric) map[string]string {
	res := make(map[string]string, len(m))
	for k, v := range m {
		res[string(k)] = string(v)
	}
	return res
}
