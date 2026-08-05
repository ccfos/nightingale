// Package evallog 将告警规则每个评估周期"查到了什么、判定出了什么、后续环节丢弃了什么"
// 以结构化记录落到引擎本地磁盘，支持按 规则+时间范围 查询原始现场，用于排查
// "TSDB 里有数据但当时没告警"一类问题。
//
// 文件布局（小时粒度，整点滚动后 gzip）：
//
//	{Dir}/{rule_id}_{datasource_id}/{2006-01-02}/{15}.jsonl(.gz)
//
// 查询按文件名直接定位小时文件，保留清理按天目录删除，均不依赖索引。
package evallog

import (
	"math"
	"unicode/utf8"
)

// EvalRecord 一条评估周期记录。字段语义与告警排障漏斗对应：
// Queries 回答"当时查到了什么"，Anomalies 回答"判定出了什么"，
// Fired/Muted/DropByPipeline/Pending/Inhibited 回答"事件后来去哪了"，
// Events 则把上述漏斗细化到每个事件（按 hash）的逐个裁决点。
type EvalRecord struct {
	Ts           int64  `json:"ts"` // 评估开始时间，毫秒
	RuleId       int64  `json:"rule_id"`
	DatasourceId int64  `json:"datasource_id"`
	DurationMs   int64  `json:"duration_ms"`
	Error        string `json:"error,omitempty"` // 周期级错误，如规则配置解析失败

	Queries []QueryRecord `json:"queries,omitempty"`

	Anomalies    []AnomalyBrief `json:"anomalies,omitempty"`
	AnomalyTotal int            `json:"anomaly_total"`
	RecoverTotal int            `json:"recover_total"`

	Fired          int `json:"fired"`
	Muted          int `json:"muted"` // 含 notify-only mute 与 mute hook
	DropByPipeline int `json:"drop_by_pipeline"`
	Pending        int `json:"pending"` // for 持续时长未满足
	Inhibited      int `json:"inhibited"`

	Events []EventTrail `json:"events,omitempty"`

	Truncated bool `json:"truncated,omitempty"` // 任一容量闸生效
}

// 事件裁决阶段。同一事件同一周期可能经过多个阶段（如先 muted_notify_only 再 fired），
// 按发生顺序追加，构成该事件本周期的处理轨迹。
const (
	StageDropByPipeline  = "drop_by_pipeline"  // 被事件流水线丢弃
	StageMuted           = "muted"             // 命中屏蔽规则，事件不再产生
	StageMutedNotifyOnly = "muted_notify_only" // 仅屏蔽通知，事件继续流转
	StageMutedByHook     = "muted_by_hook"     // 被 mute hook 拦截
	StagePending         = "pending"           // for 持续时长未满足
	StageInhibited       = "inhibited"         // 被更高级别事件抑制
	StageFired           = "fired"             // 触发并入队（含首次/重复通知）
	StageStalled         = "stalled"           // 已在告警中，本轮不重复通知
	StageNotifyMuted     = "notify_muted"      // 屏蔽通知期内的落库快照
	StageRecovered       = "recovered"         // 恢复并入队
	StagePushQueueFailed = "push_queue_failed" // 事件队列已满，入队失败
)

// EventTrail 一个事件在一个裁决点的结论。
type EventTrail struct {
	Hash     string `json:"hash"`               // process.Hash(ruleId, datasourceId, point)，与 alert_cur_event.hash 一致
	Tags     string `json:"tags,omitempty"`     // 事件标签，容量降级时会被清空
	Severity int    `json:"severity,omitempty"` //
	Stage    string `json:"stage"`
	Detail   string `json:"detail,omitempty"` // 该阶段的裁决说明，容量降级时会被清空
}

// QueryRecord 单个查询的执行现场。
type QueryRecord struct {
	Ref         string         `json:"ref"`   // 查询别名 A/B…；host 规则为 trigger type
	Query       string         `json:"query"` // 变量填充后的最终 promql / 非 prom 数据源的序列化查询
	DurationMs  int64          `json:"duration_ms"`
	Error       string         `json:"error,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
	SeriesTotal int            `json:"series_total"` // 截断前的真实总数，0 表示当时确实没查到
	Series      []SeriesSample `json:"series,omitempty"`
	VarQuery    bool           `json:"var_query,omitempty"` // 变量填充查询：Series 为聚合后的异常点采样
}

// SeriesSample 一条曲线的原始采样。
type SeriesSample struct {
	Labels map[string]string `json:"labels"`
	Points [][2]float64      `json:"points"` // [ts(秒), value]，instant 查询只有一个点
}

// AnomalyBrief 本周期判定产生的异常点/恢复点摘要。
type AnomalyBrief struct {
	Key         string  `json:"key"`
	Value       float64 `json:"value"`
	Severity    int     `json:"severity"`
	TriggerType string  `json:"trigger_type,omitempty"` // normal/nodata
	Recover     bool    `json:"recover,omitempty"`
}

// NewRecord 返回一条新记录；evallog 未启用时返回 nil，
// 后续所有方法对 nil 接收者均为 no-op，调用方无需判空。
func NewRecord(ruleId, datasourceId int64, tsMs int64) *EvalRecord {
	if !Enabled() {
		return nil
	}
	return &EvalRecord{
		Ts:           tsMs,
		RuleId:       ruleId,
		DatasourceId: datasourceId,
	}
}

// nonFiniteWarning 采样含 NaN/±Inf 时追加到 QueryRecord.Warnings 的提示。
const nonFiniteWarning = "query result contains non-finite values (NaN/Inf), recorded as 0"

// sanitizePoints 把 NaN/±Inf 归一为 0，返回是否发生过替换。
// encoding/json 对非有限浮点直接返回错误，一旦 Marshal 失败整条记录（含漏斗计数与
// 事件轨迹）都会丢掉；而查询结果出现 NaN（如分母为 0 的比率表达式）恰恰是最需要
// 留下现场的周期，所以在入库时就地兜底，而不是让序列化失败。
func sanitizePoints(points [][2]float64) bool {
	var replaced bool
	for i := range points {
		for j := range points[i] {
			if v := points[i][j]; math.IsNaN(v) || math.IsInf(v, 0) {
				points[i][j] = 0
				replaced = true
			}
		}
	}
	return replaced
}

// truncateRunes 按 rune 边界截断，避免把多字节字符切成半个导致 JSON 里出现替换字符。
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "...(truncated)"
}

// AddQuery 追加一个查询现场，按容量闸截断 Series/Points。
func (r *EvalRecord) AddQuery(q QueryRecord) {
	if r == nil {
		return
	}
	cfg := currentConfig()
	if cfg == nil {
		return
	}
	var nonFinite bool
	for i := range q.Series {
		if sanitizePoints(q.Series[i].Points) {
			nonFinite = true
		}
	}
	if nonFinite {
		q.Warnings = append(q.Warnings, nonFiniteWarning)
	}
	if len(q.Series) > cfg.MaxSeriesPerQuery {
		q.Series = q.Series[:cfg.MaxSeriesPerQuery]
		r.Truncated = true
	}
	for i := range q.Series {
		if len(q.Series[i].Points) > cfg.MaxPointsPerSeries {
			// 保留最新的点。必须**拷贝**而不是从尾部重新切片：切片仍指向原来那块分配，
			// Go 以整块为单位回收，于是记录会一直压着整条曲线的全部点，闸1b 就只限住了
			// 序列化长度、限不住堆。采集侧（EvalLogSamplesFrom*）已按 PointsCap 提前截断，
			// 这里是对其他调用方的兜底，正常路径不会命中。
			tail := q.Series[i].Points[len(q.Series[i].Points)-cfg.MaxPointsPerSeries:]
			trimmed := make([][2]float64, cfg.MaxPointsPerSeries)
			copy(trimmed, tail)
			q.Series[i].Points = trimmed
			r.Truncated = true
		}
	}
	r.Queries = append(r.Queries, q)
}

// AddAnomaly 追加一个异常/恢复点摘要，总量受 MaxSeriesPerQuery 约束。
func (r *EvalRecord) AddAnomaly(a AnomalyBrief) {
	if r == nil {
		return
	}
	if math.IsNaN(a.Value) || math.IsInf(a.Value, 0) {
		// 同 sanitizePoints：非有限值会让整条记录序列化失败
		a.Value = 0
	}
	if a.Recover {
		r.RecoverTotal++
	} else {
		r.AnomalyTotal++
	}
	cfg := currentConfig()
	if cfg == nil || len(r.Anomalies) >= cfg.MaxSeriesPerQuery {
		r.Truncated = r.Truncated || cfg != nil
		return
	}
	r.Anomalies = append(r.Anomalies, a)
}

// SetError 记录周期级错误。
func (r *EvalRecord) SetError(err error) {
	if r == nil || err == nil {
		return
	}
	r.Error = err.Error()
}

// Finish 收口周期耗时。
func (r *EvalRecord) Finish(durationMs int64) {
	if r == nil {
		return
	}
	r.DurationMs = durationMs
}

// AddEvent 记录一个事件的裁决轨迹，同时累加对应的漏斗计数。
// 由 Processor 各环节调用，nil-safe；评估与事件处理在同一 goroutine 内串行执行，无需加锁。
// 轨迹条数受 MaxSeriesPerQuery 约束，但计数不受影响（截断只丢明细，不丢统计）。
func (r *EvalRecord) AddEvent(t EventTrail) {
	if r == nil {
		return
	}

	switch t.Stage {
	case StageDropByPipeline:
		r.DropByPipeline++
	case StageMuted, StageMutedNotifyOnly, StageMutedByHook:
		r.Muted++
	case StagePending:
		r.Pending++
	case StageInhibited:
		r.Inhibited++
	case StageFired, StageStalled, StageNotifyMuted:
		// 与漏斗语义一致：进入 firing 状态即计数，含被重复间隔/次数限制 stall 的周期
		r.Fired++
	}

	cfg := currentConfig()
	if cfg == nil || len(r.Events) >= cfg.MaxSeriesPerQuery {
		r.Truncated = r.Truncated || cfg != nil
		return
	}
	r.Events = append(r.Events, t)
}

// stripSeries 去掉全部原始曲线数据，仅保留摘要（容量闸兜底动作）。
func (r *EvalRecord) stripSeries() {
	for i := range r.Queries {
		r.Queries[i].Series = nil
	}
	r.Truncated = true
}

// stripEventDetails 清空事件轨迹中的可变长字段，仅保留 hash + stage 骨架。
// 用于 stripSeries 之后仍超出单条记录上限的极端场景：轨迹回答"事件为什么没发出去"，
// 比原始曲线更接近排障结论，因此最后才降级、且只降级到骨架而非整体丢弃。
func (r *EvalRecord) stripEventDetails() {
	for i := range r.Events {
		r.Events[i].Tags = ""
		r.Events[i].Detail = ""
	}
	r.Truncated = true
}

// maxSkeletonStringBytes shrinkToSkeleton 中单个可变长字符串保留的字节数。
const maxSkeletonStringBytes = 512

// shrinkToSkeleton 前两级降级后仍超出 MaxRecordBytes 时的最后一级：清空异常点明细，
// 把每个查询压到 ref + 截断后的查询串 + 统计量。计数与事件骨架保留，它们最接近排障结论。
//
// 存在的意义是让 MaxRecordBytes 真的成为硬上限：stripSeries / stripEventDetails 都不动
// Query / Error / Warnings / Anomalies.Key 这些可变长字段，光靠它们无法收敛。而写出超过
// 读取端 4MB 扫描上限的行，会让该小时文件从这一行起的所有记录被静默丢弃。
func (r *EvalRecord) shrinkToSkeleton() {
	r.Anomalies = nil
	for i := range r.Queries {
		r.Queries[i].Series = nil
		r.Queries[i].Warnings = nil
		r.Queries[i].Query = truncateRunes(r.Queries[i].Query, maxSkeletonStringBytes)
		r.Queries[i].Error = truncateRunes(r.Queries[i].Error, maxSkeletonStringBytes)
	}
	r.Error = truncateRunes(r.Error, maxSkeletonStringBytes)
	r.Truncated = true
}
