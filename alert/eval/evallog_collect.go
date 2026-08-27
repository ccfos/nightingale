package eval

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/evallog"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

// EvalLogSamplesFromValue 将 prom 查询结果转为 evallog 采样，按 SeriesCap 提前截断避免大结果集的无谓分配。
// 返回 (采样, 真实总数)。
func EvalLogSamplesFromValue(value model.Value) ([]evallog.SeriesSample, int) {
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
			skip := pointsSkip(len(items[i].Values))
			points := make([][2]float64, 0, len(items[i].Values)-skip)
			for _, v := range items[i].Values[skip:] {
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

// pointsSkip 返回按 PointsCap 只保留最新点时应跳过的前缀长度（n 为原始点数）。
//
// 必须在**复制之前**就截断：先复制全量再让 AddQuery 从尾部重新切片，切片仍指向那块完整
// 分配，Go 以整块为单位回收，闸1b 就只限住了序列化长度、限不住堆。非 prom 数据源的
// range 查询单曲线点数可达千级，100 条曲线即多驻留十几 MB，且记录在写入队列里排队期间
// 一直不释放。cap<=0（evallog 未启用）时不截断——调用方此时根本不会走到这里。
func pointsSkip(n int) int {
	pcap := evallog.PointsCap()
	if pcap <= 0 || n <= pcap {
		return 0
	}
	return n - pcap
}

// EvalLogSamplesFromDataResps 将非 prom 数据源查询结果转为 evallog 采样。
func EvalLogSamplesFromDataResps(series []models.DataResp) ([]evallog.SeriesSample, int) {
	cap := evallog.SeriesCap()
	if cap <= 0 {
		return nil, 0
	}

	var samples []evallog.SeriesSample
	for i := 0; i < len(series) && i < cap; i++ {
		// 同 EvalLogSamplesFromValue：先按 PointsCap 截断再复制，否则整条曲线的点会被
		// 记录一直引用（详见 pointsSkip）
		skip := pointsSkip(len(series[i].Values))
		points := make([][2]float64, 0, len(series[i].Values)-skip)
		for _, v := range series[i].Values[skip:] {
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

// EvalLogSamplesFromPoints 将异常点转为 evallog 采样（变量填充查询的聚合现场）。
func EvalLogSamplesFromPoints(points []models.AnomalyPoint) ([]evallog.SeriesSample, int) {
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

// EvalLogAnomalies 将本周期判定结果写入记录。
func EvalLogAnomalies(rec *evallog.EvalRecord, points []models.AnomalyPoint, recover bool) {
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

// LogEvalRecordFallback 记录未能进入落盘队列时，把本轮现场摘要回退打到 info 日志。
//
// evallog 启用后，eval 里原本标注「此条日志很重要，是告警判断的现场值」的 info 日志
// 被降级成了 debug。但「evallog 启用」不等于「记录一定落盘」：写入队列满、或磁盘写失败
// 进入熔断期时，Push 都会直接丢弃。两者判定条件相同、失效条件不同，磁盘慢或队列打满
// 这类最需要现场的场景下会出现记录没落盘、日志也不打了的两头落空，这个回退就是为了堵住它。
//
// 刻意只打摘要而不是整条记录：会走到这里通常就是队列被打满，此时批量往日志里灌
// 几百 KB 的原始曲线，只会把「写不动」变成「写日志也写不动」。
func LogEvalRecordFallback(rec *evallog.EvalRecord) {
	if rec == nil {
		return
	}

	var qs strings.Builder
	for i := range rec.Queries {
		if i > 0 {
			qs.WriteString("; ")
		}
		q := &rec.Queries[i]
		fmt.Fprintf(&qs, "%s series=%d cost=%dms", q.Ref, q.SeriesTotal, q.DurationMs)
		if q.Error != "" {
			fmt.Fprintf(&qs, " error=%s", truncateForLog(q.Error))
		}
		fmt.Fprintf(&qs, " query=%s", truncateForLog(q.Query))
	}

	logger.Infof("alert_eval_%d datasource_%d evallog record dropped, scene summary: cost=%dms error=%s "+
		"anomaly=%d recover=%d fired=%d pending=%d muted=%d dropped=%d inhibited=%d queries[%s]",
		rec.RuleId, rec.DatasourceId, rec.DurationMs, truncateForLog(rec.Error),
		rec.AnomalyTotal, rec.RecoverTotal, rec.Fired, rec.Pending, rec.Muted, rec.DropByPipeline, rec.Inhibited,
		qs.String())
}

// truncateForLog 回退日志里单个字段的长度上限，避免超长 promql / 错误串刷爆日志。
func truncateForLog(s string) string {
	const max = 256
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "...(truncated)"
}

// EvalLogQueryString 将非 prom 数据源的查询对象序列化为字符串。
func EvalLogQueryString(query interface{}) string {
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
