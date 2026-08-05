package evallog

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func initForTest(t *testing.T, mutate func(*Config)) Config {
	t.Helper()
	cfg := Config{Dir: t.TempDir()}
	cfg.normalize()
	if mutate != nil {
		mutate(&cfg)
	}
	if err := Init(cfg, Hooks{}); err != nil {
		t.Fatalf("init error: %v", err)
	}
	t.Cleanup(Shutdown)
	return cfg
}

// queryRecs 只取记录列表，供不关心截断信息的用例使用。
func queryRecs(cfg Config, ruleId, datasourceId int64, fromMs, toMs, beforeMs int64, limit int) ([]EvalRecord, error) {
	res, err := queryRecords(cfg, ruleId, datasourceId, fromMs, toMs, beforeMs, limit)
	return res.Records, err
}

func mkRecord(ruleId int64, ts time.Time, nSeries int) *EvalRecord {
	r := NewRecord(ruleId, 1, ts.UnixMilli())
	q := QueryRecord{Ref: "A", Query: "up == 0", SeriesTotal: nSeries}
	for i := 0; i < nSeries; i++ {
		q.Series = append(q.Series, SeriesSample{
			Labels: map[string]string{"instance": fmt.Sprintf("host-%d", i)},
			Points: [][2]float64{{float64(ts.Unix()), 1}},
		})
	}
	r.AddQuery(q)
	return r
}

func TestNilRecordSafe(t *testing.T) {
	// 未启用时 NewRecord 返回 nil，所有方法应为 no-op 不 panic
	Shutdown()
	r := NewRecord(1, 1, time.Now().UnixMilli())
	if r != nil {
		t.Fatal("expect nil record when not enabled")
	}
	r.AddQuery(QueryRecord{})
	r.AddAnomaly(AnomalyBrief{})
	r.AddEvent(EventTrail{Hash: "h", Stage: StageFired})
	r.SetError(fmt.Errorf("x"))
	r.Finish(1)
	Push(r)
}

func TestAddEventCountersAndCap(t *testing.T) {
	initForTest(t, func(c *Config) {
		c.MaxSeriesPerQuery = 3
	})

	r := NewRecord(1, 1, time.Now().UnixMilli())
	stages := []string{
		StageDropByPipeline,
		StageMuted, StageMutedNotifyOnly, StageMutedByHook,
		StagePending, StageInhibited,
		StageFired, StageStalled, StageNotifyMuted,
		StageRecovered, StagePushQueueFailed,
	}
	for i, s := range stages {
		r.AddEvent(EventTrail{Hash: fmt.Sprintf("h%d", i), Stage: s, Tags: "a=b", Detail: "d"})
	}

	// 计数按 stage 归类，且不受明细截断影响
	if r.DropByPipeline != 1 {
		t.Fatalf("drop_by_pipeline: %d", r.DropByPipeline)
	}
	if r.Muted != 3 {
		t.Fatalf("muted should count 3 mute stages, got %d", r.Muted)
	}
	if r.Pending != 1 || r.Inhibited != 1 {
		t.Fatalf("pending %d inhibited %d", r.Pending, r.Inhibited)
	}
	if r.Fired != 3 {
		t.Fatalf("fired should count fired/stalled/notify_muted, got %d", r.Fired)
	}
	// recovered / push_queue_failed 不进漏斗计数
	if len(r.Events) != 3 || !r.Truncated {
		t.Fatalf("expect 3 trails kept and truncated flag, got %d trails truncated=%v", len(r.Events), r.Truncated)
	}
}

func TestEventTrailPersistedAndDegraded(t *testing.T) {
	cfg := initForTest(t, func(c *Config) {
		c.MaxRecordBytes = 2048
	})

	now := time.Now()
	// 大查询结果 + 事件轨迹：先裁 Series，轨迹骨架应保留
	r := mkRecord(21, now, 50)
	r.AddEvent(EventTrail{Hash: "abc123", Tags: "instance=web-1", Severity: 2, Stage: StagePending, Detail: "for=300s elapsed=120s"})
	Push(r)
	Shutdown()

	recs, err := queryRecs(cfg, 21, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("query: %v n=%d", err, len(recs))
	}
	got := recs[0]
	if len(got.Queries[0].Series) != 0 {
		t.Fatal("expect series stripped first")
	}
	if len(got.Events) != 1 {
		t.Fatalf("expect event trail kept, got %d", len(got.Events))
	}
	e := got.Events[0]
	if e.Hash != "abc123" || e.Stage != StagePending {
		t.Fatalf("trail skeleton lost: %+v", e)
	}
	// 本例裁完 Series 后已在上限内，明细不应被清空
	if e.Detail != "for=300s elapsed=120s" || e.Tags != "instance=web-1" {
		t.Fatalf("detail should survive when within limit: %+v", e)
	}
	if got.Pending != 1 {
		t.Fatalf("counter should be persisted: %d", got.Pending)
	}
}

func TestEventTrailSkeletonWhenStillOverLimit(t *testing.T) {
	cfg := initForTest(t, func(c *Config) {
		c.MaxRecordBytes = 512 // 极小上限：裁完 Series 仍超限
	})

	now := time.Now()
	r := NewRecord(22, 1, now.UnixMilli())
	for i := 0; i < 20; i++ {
		r.AddEvent(EventTrail{
			Hash:     fmt.Sprintf("hash-%d", i),
			Tags:     strings.Repeat("k=v,,", 20),
			Severity: 2,
			Stage:    StageFired,
			Detail:   strings.Repeat("reason ", 20),
		})
	}
	Push(r)
	Shutdown()

	recs, err := queryRecs(cfg, 22, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("query: %v n=%d", err, len(recs))
	}
	got := recs[0]
	if len(got.Events) != 20 {
		t.Fatalf("expect all trails kept as skeleton, got %d", len(got.Events))
	}
	for _, e := range got.Events {
		if e.Hash == "" || e.Stage != StageFired {
			t.Fatalf("skeleton must keep hash+stage: %+v", e)
		}
		if e.Tags != "" || e.Detail != "" {
			t.Fatalf("expect tags/detail stripped: %+v", e)
		}
	}
	if !got.Truncated || got.Fired != 20 {
		t.Fatalf("truncated=%v fired=%d", got.Truncated, got.Fired)
	}
}

func TestWriteQueryRoundtrip(t *testing.T) {
	cfg := initForTest(t, nil)

	now := time.Now().Truncate(time.Minute)
	for i := 0; i < 10; i++ {
		r := mkRecord(100, now.Add(time.Duration(i)*time.Second), 2)
		r.AddAnomaly(AnomalyBrief{Key: "k", Value: 1, Severity: 2})
		r.AddEvent(EventTrail{Hash: "h", Tags: "k=v", Severity: 2, Stage: StageFired, Detail: "fired, first_trigger_time: 1"})
		Push(r)
	}
	Shutdown() // 刷盘

	recs, err := queryRecs(cfg, 100, 1, now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), 0, 0)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(recs) != 10 {
		t.Fatalf("expect 10 records, got %d", len(recs))
	}
	// 倒序
	for i := 1; i < len(recs); i++ {
		if recs[i].Ts > recs[i-1].Ts {
			t.Fatalf("records not desc: %d > %d", recs[i].Ts, recs[i-1].Ts)
		}
	}
	if recs[0].Fired != 1 || recs[0].AnomalyTotal != 1 {
		t.Fatalf("funnel/anomaly not persisted: %+v", recs[0])
	}
	if len(recs[0].Queries) != 1 || recs[0].Queries[0].SeriesTotal != 2 {
		t.Fatalf("query record not persisted: %+v", recs[0].Queries)
	}

	// limit + before 游标
	page1, _ := queryRecs(cfg, 100, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 3)
	if len(page1) != 3 {
		t.Fatalf("expect 3, got %d", len(page1))
	}
	page2, _ := queryRecs(cfg, 100, 1, 0, now.Add(time.Hour).UnixMilli(), page1[2].Ts, 3)
	if len(page2) != 3 || page2[0].Ts >= page1[2].Ts {
		t.Fatalf("cursor page wrong: %+v", page2)
	}

	// 其他规则查不到
	other, _ := queryRecs(cfg, 200, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
	if len(other) != 0 {
		t.Fatalf("expect empty for other rule, got %d", len(other))
	}
}

func TestHourRollAndGzip(t *testing.T) {
	cfg := initForTest(t, nil)

	old := time.Now().Add(-2 * time.Hour)
	Push(mkRecord(7, old, 1))
	Push(mkRecord(7, time.Now(), 1)) // 跨小时触发滚动
	Shutdown()                       // 等待压缩完成

	oldPath := filepath.Join(cfg.Dir, "7_1", old.Format(dateLayout), old.Format(hourLayout))
	if _, err := os.Stat(oldPath + ".jsonl.gz"); err != nil {
		t.Fatalf("expect gz file: %v", err)
	}
	if _, err := os.Stat(oldPath + ".jsonl"); !os.IsNotExist(err) {
		t.Fatal("expect plain file removed after compress")
	}

	// 跨小时查询要能同时读到 gz 与当前文件
	recs, err := queryRecs(cfg, 7, 1, old.Add(-time.Minute).UnixMilli(), time.Now().UnixMilli(), 0, 0)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expect 2 records across hours, got %d", len(recs))
	}
}

func TestSeriesAndPointsCap(t *testing.T) {
	cfg := initForTest(t, func(c *Config) {
		c.MaxSeriesPerQuery = 3
		c.MaxPointsPerSeries = 2
	})

	r := NewRecord(1, 1, time.Now().UnixMilli())
	q := QueryRecord{Ref: "A", SeriesTotal: 5}
	for i := 0; i < 5; i++ {
		// 底层数组刻意留出远大于 len 的容量，模拟真实的 range 查询结果（千级点数）。
		// 用 [][2]float64{...} 字面量的话 cap==len，从尾部重新切片后 cap 恰好等于闸值，
		// 「是否还牵着整块原数组」这件事就被掩盖了，下面那条 cap 断言会永远通过。
		pts := make([][2]float64, 0, 1024)
		for j := 1; j <= 4; j++ {
			pts = append(pts, [2]float64{float64(j), float64(j)})
		}
		q.Series = append(q.Series, SeriesSample{
			Labels: map[string]string{"i": fmt.Sprintf("%d", i)},
			Points: pts,
		})
	}
	r.AddQuery(q)

	if len(r.Queries[0].Series) != 3 {
		t.Fatalf("expect 3 series, got %d", len(r.Queries[0].Series))
	}
	pts := r.Queries[0].Series[0].Points
	if len(pts) != 2 || pts[0][0] != 3 || pts[1][0] != 4 {
		t.Fatalf("expect last 2 points kept, got %v", pts)
	}
	// 截断必须换成新分配，不能是从尾部重新切片：切片仍指向原来那块分配，Go 以整块为单位
	// 回收，记录会一直压着整条曲线的全部点——闸1b 就只限住了序列化长度、限不住堆。
	if cap(pts) > cfg.MaxPointsPerSeries {
		t.Fatalf("trimmed points still reference the original %d-cap array; copy instead of resliceing", cap(pts))
	}
	if !r.Truncated {
		t.Fatal("expect truncated flag")
	}

	// 异常点摘要同样受限
	for i := 0; i < 5; i++ {
		r.AddAnomaly(AnomalyBrief{Key: fmt.Sprintf("%d", i)})
	}
	if len(r.Anomalies) != 3 || r.AnomalyTotal != 5 {
		t.Fatalf("anomaly cap wrong: kept %d total %d", len(r.Anomalies), r.AnomalyTotal)
	}
}

func TestMaxRecordBytesStrip(t *testing.T) {
	cfg := initForTest(t, func(c *Config) {
		c.MaxRecordBytes = 1024
	})

	now := time.Now()
	r := mkRecord(9, now, 50) // 序列化后远超 1KB
	Push(r)
	Shutdown()

	recs, err := queryRecs(cfg, 9, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("query: %v, n=%d", err, len(recs))
	}
	if len(recs[0].Queries[0].Series) != 0 {
		t.Fatal("expect series stripped by MaxRecordBytes")
	}
	if !recs[0].Truncated || recs[0].Queries[0].SeriesTotal != 50 {
		t.Fatalf("expect truncated with series_total kept: %+v", recs[0].Queries[0])
	}
}

func TestDailyBudgetDegrade(t *testing.T) {
	cfg := initForTest(t, func(c *Config) {
		c.PerRuleDailyMB = 1
	})

	// 单条约 60KB（100 series × 500B labels），写 30 条超 1MB 预算
	now := time.Now().Truncate(time.Minute)
	for i := 0; i < 30; i++ {
		r := NewRecord(11, 1, now.Add(time.Duration(i)*time.Second).UnixMilli())
		q := QueryRecord{Ref: "A", SeriesTotal: 100}
		for j := 0; j < 100; j++ {
			q.Series = append(q.Series, SeriesSample{
				Labels: map[string]string{"pad": strings.Repeat("x", 500), "i": fmt.Sprintf("%d", j)},
				Points: [][2]float64{{1, 1}},
			})
		}
		r.AddQuery(q)
		Push(r)
	}
	Shutdown()

	recs, err := queryRecs(cfg, 11, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
	if err != nil || len(recs) != 30 {
		t.Fatalf("query: %v, n=%d", err, len(recs))
	}
	// recs 为倒序：最新的应已降级为摘要，最旧的应保留完整 Series
	if len(recs[0].Queries[0].Series) != 0 || !recs[0].Truncated {
		t.Fatalf("expect newest degraded to summary: series=%d", len(recs[0].Queries[0].Series))
	}
	if len(recs[len(recs)-1].Queries[0].Series) == 0 {
		t.Fatal("expect oldest keeps full series")
	}
}

func TestCleanerRetention(t *testing.T) {
	cfg := Config{Dir: t.TempDir()}
	cfg.normalize()
	cfg.RetentionHours = 48

	mk := func(day time.Time, hour string) string {
		dir := filepath.Join(cfg.Dir, "1_1", day.Format(dateLayout))
		os.MkdirAll(dir, 0755)
		p := filepath.Join(dir, hour+".jsonl.gz")
		os.WriteFile(p, []byte("x"), 0644)
		return p
	}

	oldFile := mk(time.Now().AddDate(0, 0, -5), "10")
	newFile := mk(time.Now(), time.Now().Format(hourLayout))

	cleanOnce(cfg)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("expect expired file removed")
	}
	if _, err := os.Stat(filepath.Dir(oldFile)); !os.IsNotExist(err) {
		t.Fatal("expect expired date dir removed")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatal("expect fresh file kept")
	}
}

// 回归：同一小时被二次滚动压缩时，compressFile 曾用 rename 覆盖已有 .gz，
// 把整小时记录冲成只剩迟到的那一两条。触发路径是跨整点的慢评估（Ts 取评估开始时刻、
// Push 在评估结束时执行）。
func TestCompressAppendsInsteadOfOverwritingGz(t *testing.T) {
	cfg := initForTest(t, nil)

	// 上一小时的一批记录，先滚动压缩成 .gz
	past := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 5; i++ {
		Push(mkRecord(101, past.Add(time.Duration(i)*time.Second), 1))
	}
	Push(mkRecord(101, time.Now(), 1)) // 跨小时，触发 finalize + gzip
	waitGzip(t, cfg, 101, past)

	// 迟到的同小时记录：会重建同名 .jsonl，再次压缩时不能覆盖已有 .gz
	Push(mkRecord(101, past.Add(9*time.Second), 1))
	Shutdown()
	if err := compressFile(hourPath(cfg, 101, past) + ".jsonl"); err != nil && !os.IsNotExist(err) {
		t.Fatalf("compress late file: %v", err)
	}

	recs, err := queryRecs(cfg, 101, 1, past.Add(-time.Hour).UnixMilli(), past.Add(time.Hour).UnixMilli(), 0, 0)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(recs) != 6 {
		t.Fatalf("expect 6 records preserved across two compressions, got %d", len(recs))
	}
}

func hourPath(cfg Config, ruleId int64, at time.Time) string {
	return filepath.Join(cfg.Dir, fmt.Sprintf("%d_1", ruleId), at.Format(dateLayout), at.Format(hourLayout))
}

func waitGzip(t *testing.T, cfg Config, ruleId int64, at time.Time) {
	t.Helper()
	waitFile(t, hourPath(cfg, ruleId, at)+".jsonl.gz")
}

func waitFile(t *testing.T, path string) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", path)
}

// 回归：采样含 NaN/±Inf 时 json.Marshal 报错，整条记录（连同漏斗计数与事件轨迹）被丢弃。
// prom 的比率表达式分母为 0 就会产生 NaN，正是最该留下现场的周期。
func TestNonFiniteValuesDoNotDropRecord(t *testing.T) {
	cfg := initForTest(t, nil)

	now := time.Now()
	r := NewRecord(31, 1, now.UnixMilli())
	r.AddQuery(QueryRecord{
		Ref: "A", Query: "a/b", SeriesTotal: 2,
		Series: []SeriesSample{
			{Labels: map[string]string{"i": "x"}, Points: [][2]float64{{float64(now.Unix()), math.NaN()}}},
			{Labels: map[string]string{"i": "y"}, Points: [][2]float64{{float64(now.Unix()), math.Inf(-1)}}},
		},
	})
	r.AddAnomaly(AnomalyBrief{Key: "x", Value: math.Inf(1), Severity: 2})
	r.AddEvent(EventTrail{Hash: "h1", Stage: StageFired})
	Push(r)
	Shutdown()

	recs, err := queryRecs(cfg, 31, 1, now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), 0, 0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("record must survive non-finite values: err=%v n=%d", err, len(recs))
	}
	got := recs[0]
	if got.Fired != 1 || got.AnomalyTotal != 1 {
		t.Fatalf("counters lost: fired=%d anomaly=%d", got.Fired, got.AnomalyTotal)
	}
	for _, s := range got.Queries[0].Series {
		if v := s.Points[0][1]; math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("non-finite value leaked into storage: %v", v)
		}
	}
	if !containsString(got.Queries[0].Warnings, nonFiniteWarning) {
		t.Fatalf("expect warning about non-finite values, got %v", got.Queries[0].Warnings)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// 回归：查询下界没有钳到保留期时，from=0 会让小时循环一路倒退到 1970 年。
func TestClampFromToRetention(t *testing.T) {
	cfg := Config{RetentionHours: 24}
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	floor := now.Add(-24 * time.Hour).UnixMilli()

	if got := clampFromToRetention(cfg, 0, now); got != floor {
		t.Fatalf("from=0 should be clamped to retention floor %d, got %d", floor, got)
	}
	inRange := now.Add(-time.Hour).UnixMilli()
	if got := clampFromToRetention(cfg, inRange, now); got != inRange {
		t.Fatalf("in-range from must be untouched, got %d", got)
	}
}

// 回归：只钳 from 不钳 to 时，to=公元 9999 年会让小时循环跑约 7000 万轮
// （每轮 2 次 os.Open，实测约 4 分钟纯 syscall），与 from 侧那个已修的慢速 DoS 完全对称。
func TestClampToToNow(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	ceil := now.Add(futureSkewAllowance).UnixMilli()

	// 远未来的 to 被钳到 now + 余量
	if got := clampToToNow(time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli(), now); got != ceil {
		t.Fatalf("far-future to should be clamped to %d, got %d", ceil, got)
	}
	// 余量之内的 to 原样保留（覆盖浏览器时钟快几分钟这类现实偏差）
	inSkew := now.Add(30 * time.Minute).UnixMilli()
	if got := clampToToNow(inSkew, now); got != inSkew {
		t.Fatalf("to within the skew allowance must be untouched, got %d", got)
	}
	// 过去的 to 原样保留
	past := now.Add(-3 * time.Hour).UnixMilli()
	if got := clampToToNow(past, now); got != past {
		t.Fatalf("past to must be untouched, got %d", got)
	}
}

// 端到端：to 传成公元 9999 年时既要立刻返回，也不能因为钳制而漏掉区间内的记录。
// 未钳制时这个用例会跑上百秒，超时本身就是失败信号。
func TestQueryWithFarFutureToStaysBounded(t *testing.T) {
	cfg := initForTest(t, nil)

	now := time.Now()
	Push(mkRecord(66, now.Add(-2*time.Minute), 1))
	Shutdown()

	farFuture := time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	start := time.Now()
	recs, err := queryRecs(cfg, 66, 1, now.Add(-time.Hour).UnixMilli(), farFuture, 0, 0)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("clamping to must not drop in-range records, got %d", len(recs))
	}
	// 钳制后轮数被 RetentionHours（默认 192）封顶，实测亚毫秒级；
	// 这里留足两个数量级的余量，只用来拦住「完全没钳」的情况
	if elapsed > 5*time.Second {
		t.Fatalf("query took %v, to is probably not clamped", elapsed)
	}

	// 整个窗口都在未来：返回空而不是报错
	recs, err = queryRecs(cfg, 66, 1, now.Add(24*time.Hour).UnixMilli(), farFuture, 0, 0)
	if err != nil {
		t.Fatalf("a fully-future window should return empty, not error: %v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("expect no records for a fully-future window, got %d", len(recs))
	}

	// 入参本身倒挂仍应保持原有报错语义
	if _, err = queryRecs(cfg, 66, 1, now.UnixMilli(), now.Add(-time.Hour).UnixMilli(), 0, 0); err == nil {
		t.Fatal("expect error when from > to on input")
	}
}

// 回归：读侧曾用 time.Truncate(time.Hour)（按绝对时长取整），写侧按本地展现形式分桶，
// 在非整点偏移的时区里对不上，每小时前半段的记录永远定位不到文件。
func TestTruncHourMatchesLocalHourBucket(t *testing.T) {
	for _, zone := range []string{"Asia/Shanghai", "Asia/Kolkata", "Asia/Tehran", "Australia/Adelaide", "America/St_Johns"} {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Skipf("tzdata unavailable for %s: %v", zone, err)
		}
		for _, minute := range []int{0, 10, 29, 30, 45, 59} {
			at := time.Date(2026, 8, 4, 22, minute, 0, 0, loc)
			h := truncHour(at)
			if h.Format(dateLayout) != at.Format(dateLayout) || h.Format(hourLayout) != at.Format(hourLayout) {
				t.Fatalf("%s %s: truncHour gave %s/%s, want %s/%s", zone, at.Format(time.RFC3339),
					h.Format(dateLayout), h.Format(hourLayout), at.Format(dateLayout), at.Format(hourLayout))
			}
		}
	}
}

// 回归：日预算曾挂在 appender 上，appender 因空闲/淘汰被关闭后计数清零，
// 评估间隔超过 appenderIdleTimeout 的规则永远触发不了降级。
func TestDailyBudgetSurvivesAppenderClose(t *testing.T) {
	// 用裸 Writer 而不是 initForTest：appenders / budgets 按设计只在消费 goroutine 内访问，
	// 起了 loop() 再从测试 goroutine 改这两张表，就违反了本测试要守的那条不变量
	w := &Writer{
		cfg:       Config{PerRuleDailyMB: 1},
		appenders: make(map[string]*appender),
		budgets:   make(map[string]*ruleBudget),
	}
	now := time.Now()
	date := now.Format(dateLayout)
	key := "77" // 预算按 rule_id 计，appender 才是 {rule_id}_{ds_id}

	b := w.budgetFor(key, date, now)
	b.bytes = 900 * 1024

	// 模拟 housekeep 因空闲关闭句柄：appender 没了，预算必须还在
	delete(w.appenders, "77_1")

	again := w.budgetFor(key, date, now)
	if again.bytes != 900*1024 {
		t.Fatalf("budget must survive appender close, got %d", again.bytes)
	}
	// 跨日才清零
	if next := w.budgetFor(key, "2999-01-01", now); next.bytes != 0 {
		t.Fatalf("budget must reset on new date, got %d", next.bytes)
	}
}

// 回归：日预算的 key 曾复用 appender 的 {rule_id}_{ds_id}，于是一条按 cate 匹配到 N 个
// 数据源的规则实际拿到 N × PerRuleDailyMB 的额度，闸2 先于 MaxDiskGB 失效；而 MaxDiskGB
// 的兜底是从最旧小时全局删起，一条胖规则会把其他规则的历史记录一起挤掉。
func TestDailyBudgetSharedAcrossDatasources(t *testing.T) {
	cfg := initForTest(t, func(c *Config) {
		c.PerRuleDailyMB = 1
	})

	// 单条约 60KB（100 series × 500B labels）。同一规则的两个数据源交替写，
	// 合计超过 1MB 后**两个数据源**都应降级为摘要
	now := time.Now().Truncate(time.Minute)
	for i := 0; i < 30; i++ {
		dsId := int64(1 + i%2)
		r := NewRecord(88, dsId, now.Add(time.Duration(i)*time.Second).UnixMilli())
		q := QueryRecord{Ref: "A", SeriesTotal: 100}
		for j := 0; j < 100; j++ {
			q.Series = append(q.Series, SeriesSample{
				Labels: map[string]string{"pad": strings.Repeat("x", 500), "i": fmt.Sprintf("%d", j)},
				Points: [][2]float64{{1, 1}},
			})
		}
		r.AddQuery(q)
		Push(r)
	}
	Shutdown()

	for _, dsId := range []int64{1, 2} {
		recs, err := queryRecs(cfg, 88, dsId, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
		if err != nil || len(recs) == 0 {
			t.Fatalf("query ds %d: %v, n=%d", dsId, err, len(recs))
		}
		// recs 倒序：最新一条必然在预算用尽之后，两个数据源都应已降级
		if len(recs[0].Queries[0].Series) != 0 || !recs[0].Truncated {
			t.Fatalf("ds %d: expect newest degraded to summary once the rule's shared budget is used up, series=%d",
				dsId, len(recs[0].Queries[0].Series))
		}
	}
}

// 回归：清理曾把 Dir 下任意 {任意名}/{YYYY-MM-DD}/ 目录整棵删掉，
// Dir 误配成已有目录时会静默删除第三方数据。
func TestCleanerOnlyTouchesOwnLayout(t *testing.T) {
	cfg := Config{Dir: t.TempDir()}
	cfg.normalize()
	cfg.RetentionHours = 48

	oldDay := time.Now().AddDate(0, 0, -5).Format(dateLayout)

	// 非本包写出的目录结构：名字不是 {rule_id}_{ds_id}
	foreignDir := filepath.Join(cfg.Dir, "someone-elses-logs", oldDay)
	os.MkdirAll(foreignDir, 0755)
	foreignFile := filepath.Join(foreignDir, "important.log")
	os.WriteFile(foreignFile, []byte("keep me"), 0644)

	// 本包写出的过期数据，应当被删
	ownDir := filepath.Join(cfg.Dir, "5_2", oldDay)
	os.MkdirAll(ownDir, 0755)
	ownFile := filepath.Join(ownDir, "10.jsonl.gz")
	os.WriteFile(ownFile, []byte("x"), 0644)

	cleanOnce(cfg)

	if _, err := os.Stat(foreignFile); err != nil {
		t.Fatalf("foreign directory must not be touched: %v", err)
	}
	if _, err := os.Stat(ownFile); !os.IsNotExist(err) {
		t.Fatal("expect own expired hour file removed")
	}
	// 说明：{rule_id}_{ds_id}/{date}/ 整棵目录是本包自己的布局，过期后连同内容一起
	// RemoveAll 是预期行为；这里守的是「不越出这个布局」。
}

func TestCleanerPathPatterns(t *testing.T) {
	for _, name := range []string{"1_1", "12_0", "999999_37"} {
		if !ruleDirPattern.MatchString(name) {
			t.Fatalf("%q should be recognized as a rule dir", name)
		}
	}
	for _, name := range []string{"logs", "1_", "_1", "1_1_1", "a_1", "1_1.bak", ".."} {
		if ruleDirPattern.MatchString(name) {
			t.Fatalf("%q must not be treated as a rule dir", name)
		}
	}
	for _, name := range []string{"00.jsonl", "23.jsonl.gz"} {
		if !hourFilePattern.MatchString(name) {
			t.Fatalf("%q should be recognized as an hour file", name)
		}
	}
	for _, name := range []string{"notes.txt", "1.jsonl", "100.jsonl", "23.jsonl.gz.tmp", "23.gz"} {
		if hourFilePattern.MatchString(name) {
			t.Fatalf("%q must not be treated as an hour file", name)
		}
	}
}

// 回归：writer 启动时的 sweepLeftovers 曾只按 .jsonl 后缀遍历整棵子树，
// 命中的文件会被压成 .gz 并删除原文件，Dir 误配成已有目录时会毁掉第三方数据。
func TestSweepLeftoversOnlyTouchesOwnLayout(t *testing.T) {
	dir := t.TempDir()

	// 非本包布局的 .jsonl：一层、两层、四层，以及规则目录名不合法的情况
	foreign := []string{
		filepath.Join(dir, "app.jsonl"),
		filepath.Join(dir, "someone-elses-logs", "events.jsonl"),
		filepath.Join(dir, "someone-elses-logs", "2020-01-02", "10.jsonl"),
		filepath.Join(dir, "1_1", "2020-01-02", "nested", "10.jsonl"),
		filepath.Join(dir, "1_1", "not-a-date", "10.jsonl"),
	}
	for _, p := range foreign {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("keep me\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 本包布局的历史小时文件：应当被压缩掉
	past := time.Now().Add(-2 * time.Hour)
	own := filepath.Join(dir, "3_1", past.Format(dateLayout), past.Format(hourLayout)+".jsonl")
	if err := os.MkdirAll(filepath.Dir(own), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(own, []byte("{\"ts\":1}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := Config{Dir: dir}
	cfg.normalize()
	// 直接用 NewWriter 而不是 Init：Init 会同时拉起 cleanLoop，而按设计
	// {rule_id}_{ds_id}/{date}/ 整棵目录归本包所有、过期即 RemoveAll，
	// 会把放在那底下的对照文件一起删掉，混淆本测试要守的「sweep 不越界」。
	w, err := NewWriter(cfg, nil)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	// sweepLeftovers 在自己的 goroutine 里跑，直接 Close 会因 gzClosed 让它提前中止；
	// 先等它把本包的遗留文件压完，再关闭并检查第三方文件
	waitFile(t, own+".gz")
	w.Close()

	for _, p := range foreign {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("foreign file must not be touched: %s: %v", p, err)
		}
		if _, err := os.Stat(p + ".gz"); !os.IsNotExist(err) {
			t.Fatalf("foreign file must not be compressed: %s.gz exists", p)
		}
	}
	if _, err := os.Stat(own + ".gz"); err != nil {
		t.Fatalf("own leftover hour file should be compressed: %v", err)
	}
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatal("own leftover plain file should be removed after compress")
	}
}

func TestOwnHourFile(t *testing.T) {
	root := "/var/lib/evallog"
	if h, ok := ownHourFile(root, filepath.Join(root, "12_3", "2026-08-04", "07.jsonl")); !ok {
		t.Fatal("valid layout should be recognized")
	} else if h.Hour() != 7 || h.Day() != 4 {
		t.Fatalf("unexpected hour: %v", h)
	}
	if _, ok := ownHourFile(root, filepath.Join(root, "12_3", "2026-08-04", "07.jsonl.gz")); !ok {
		t.Fatal("gz variant should be recognized")
	}
	for _, p := range []string{
		filepath.Join(root, "app.jsonl"),
		filepath.Join(root, "logs", "2026-08-04", "07.jsonl"),
		filepath.Join(root, "12_3", "2026-08-04", "sub", "07.jsonl"),
		filepath.Join(root, "12_3", "not-a-date", "07.jsonl"),
		filepath.Join(root, "12_3", "2026-08-04", "7.jsonl"),
		filepath.Join(root, "12_3", "2026-08-04", "07.jsonl.gz.tmp"),
	} {
		if _, ok := ownHourFile(root, p); ok {
			t.Fatalf("%q must not be treated as an own hour file", p)
		}
	}
}

// 回归：压缩进行中重开同名 .jsonl，会让新 appender 持有一个随即被 compressFile
// os.Remove 掉的 inode，之后写入静默丢失。getAppender 现在先等压缩结束。
func TestGetAppenderWaitsForInflightCompression(t *testing.T) {
	cfg := Config{Dir: t.TempDir()}
	cfg.normalize()
	w, err := NewWriter(cfg, nil)
	if err != nil {
		t.Fatalf("new writer: %v", err)
	}
	defer w.Close()

	path := filepath.Join(cfg.Dir, "1_1", "2026-08-04", "07.jsonl")

	// 手工登记一个"压缩中"的路径，模拟 gzLoop 正在处理
	done := make(chan struct{})
	w.gzMu.Lock()
	w.gzInflight[path] = done
	w.gzMu.Unlock()

	returned := make(chan struct{})
	go func() {
		w.waitGzDone(path)
		close(returned)
	}()

	select {
	case <-returned:
		t.Fatal("waitGzDone must block while the path is being compressed")
	case <-time.After(50 * time.Millisecond):
	}

	w.finishGz(path)
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		t.Fatal("waitGzDone must return once compression finishes")
	}

	// 未登记的路径不得阻塞
	w.waitGzDone(filepath.Join(cfg.Dir, "9_9", "2026-08-04", "07.jsonl"))
}

// 回归：gzip 队列打满时 enqueueGz 会放弃压缩，此时若仍留着 inflight 登记，
// getAppender 里的 waitGzDone 会永远阻塞消费 goroutine。
func TestEnqueueGzReleasesWaiterWhenQueueFull(t *testing.T) {
	cfg := Config{Dir: t.TempDir()}
	cfg.normalize()
	w := &Writer{
		cfg:        cfg,
		gzCh:       make(chan string), // 无缓冲且无人消费 → 必然走 default 分支
		gzInflight: make(map[string]chan struct{}),
	}

	w.enqueueGz("/tmp/whatever/07.jsonl")

	w.gzMu.Lock()
	_, busy := w.gzInflight["/tmp/whatever/07.jsonl"]
	w.gzMu.Unlock()
	if busy {
		t.Fatal("dropped compress task must not stay registered as inflight")
	}

	waited := make(chan struct{})
	go func() {
		w.waitGzDone("/tmp/whatever/07.jsonl")
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("waitGzDone must not block after the task was dropped")
	}
}

// 回归：磁盘写失败必须走熔断——每个窗口至多一条进入/续期日志、每条丢失都计入 dropHook、
// Push 直接拒绝让调用方回退 info 摘要（与队列满同语义）、磁盘恢复后自动收口清零。
// 修复前：磁盘满时每条记录一条 Warning（日志与 evallog 常在同一块盘，写放大雪上加霜），
// 且 write 失败路径不计 dropHook，eval_log_drop_total 对这类丢失是盲的。
func TestWriteFailureCircuitBreaker(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based failure injection does not work as root")
	}
	old := writeFailCooldown
	writeFailCooldown = 50 * time.Millisecond
	defer func() { writeFailCooldown = old }()

	var drops int32
	cfg := Config{Dir: t.TempDir()}
	cfg.normalize()
	// 裸 Writer 同步调用 write()，避开消费 goroutine 的时序；字段访问约定同 loop()
	w := &Writer{
		cfg:        cfg,
		ch:         make(chan *EvalRecord, 4),
		dropHook:   func() { atomic.AddInt32(&drops, 1) },
		appenders:  make(map[string]*appender),
		budgets:    make(map[string]*ruleBudget),
		gzCh:       make(chan string, 8),
		gzInflight: make(map[string]chan struct{}),
	}
	// 不走 NewRecord：它依赖包级单例已 Init，而本用例刻意用裸 Writer
	rec := func(ruleId int64, ts time.Time) *EvalRecord {
		return &EvalRecord{Ts: ts.UnixMilli(), RuleId: ruleId, DatasourceId: 1}
	}
	now := time.Now()

	// 基线：正常写入不触发熔断
	w.write(rec(1, now))
	if w.degraded(time.Now()) {
		t.Fatal("healthy write must not degrade")
	}

	// 注入打开失败：根目录改只读，新规则目录的 MkdirAll 必然失败
	if err := os.Chmod(cfg.Dir, 0555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(cfg.Dir, 0755)

	w.write(rec(2, now))
	if !w.degraded(time.Now()) {
		t.Fatal("open failure must enter degradation")
	}
	if got := atomic.LoadInt32(&drops); got != 1 {
		t.Fatalf("the triggering record must be counted as dropped, got %d", got)
	}

	// 熔断期内：Push 直接拒绝（调用方据此回退 info 摘要），write 入口短路，均计数、不碰磁盘
	if w.Push(rec(3, now)) {
		t.Fatal("Push must report not-queued during degradation")
	}
	w.write(rec(4, now))
	if got := atomic.LoadInt32(&drops); got != 3 {
		t.Fatalf("every dropped record must hit dropHook, got %d", got)
	}
	if got := atomic.LoadInt64(&w.degradedDrops); got != 3 {
		t.Fatalf("degradedDrops = %d, want 3", got)
	}

	// 修复磁盘 + 熔断到期：下一条写入成功即恢复并清零统计
	if err := os.Chmod(cfg.Dir, 0755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * writeFailCooldown)
	w.write(rec(5, now))
	if w.degraded(time.Now()) || atomic.LoadInt64(&w.degradedUntilNs) != 0 {
		t.Fatal("a successful write after the cooldown must clear degradation")
	}
	if got := atomic.LoadInt64(&w.degradedDrops); got != 0 {
		t.Fatalf("drop counter must reset on recovery, got %d", got)
	}

	// 恢复后的记录要真的落盘可查
	w.closeAll(false)
	recs, err := queryRecs(cfg, 5, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("post-recovery record must be persisted: err=%v n=%d", err, len(recs))
	}
}

// 回归：MaxDiskGB 兜底曾把当前小时文件排除在 total 之外，而写入几乎全部集中在当前小时，
// 于是目录实际早已超预算时 pruneToBudget 仍可能一条都不删。
func TestCleanerCountsCurrentHourInTotal(t *testing.T) {
	cfg := Config{Dir: t.TempDir()}
	cfg.normalize()
	cfg.RetentionHours = 48
	cfg.MaxDiskGB = 1 // budget = 1GB

	now := time.Now()
	// 用稀疏文件而不是真写字节：cleanOnce 只读 FileInfo.Size()（逻辑大小），
	// 而这个用例要造出 GB 级的目录占用，真写会拖慢整个包的测试并可能撑爆临时盘
	write := func(at time.Time, size int64) string {
		dir := filepath.Join(cfg.Dir, "1_1", at.Format(dateLayout))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, at.Format(hourLayout)+".jsonl.gz")
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(size); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// 旧文件 0.4GB + 当前小时 0.7GB = 1.1GB > 1GB 预算。
	// 漏算当前小时时 total 只有 0.4GB，旧文件会被判定为"没超预算"而保留。
	const gb int64 = 1024 * 1024 * 1024
	oldFile := write(now.Add(-3*time.Hour), 4*gb/10)
	curFile := write(now, 7*gb/10)

	cleanOnce(cfg)

	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("expect the old hour file pruned once the current hour is counted in total")
	}
	if _, err := os.Stat(curFile); err != nil {
		t.Fatalf("current hour file must never be pruned: %v", err)
	}
}

// 回归：读取端曾把整个小时文件反序列化后才应用 limit。现在 limit 下推到扫描环节，
// 但跨小时的倒序、翻页语义必须保持不变。
func TestQueryLimitPushdownAcrossHours(t *testing.T) {
	cfg := initForTest(t, nil)

	now := time.Now()
	// 三个小时各 5 条
	for hoursAgo := 0; hoursAgo < 3; hoursAgo++ {
		base := now.Add(-time.Duration(hoursAgo) * time.Hour)
		for i := 0; i < 5; i++ {
			Push(mkRecord(55, base.Add(time.Duration(i)*time.Second), 1))
		}
	}
	Shutdown()

	from := now.Add(-4 * time.Hour).UnixMilli()
	to := now.Add(time.Hour).UnixMilli()

	all, err := queryRecs(cfg, 55, 1, from, to, 0, 0)
	if err != nil || len(all) != 15 {
		t.Fatalf("expect 15 records: err=%v n=%d", err, len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Ts > all[i-1].Ts {
			t.Fatalf("records not desc at %d: %d > %d", i, all[i].Ts, all[i-1].Ts)
		}
	}

	// limit 小于单个小时的条数：只能拿到最新小时里最新的 3 条
	top3, err := queryRecs(cfg, 55, 1, from, to, 0, 3)
	if err != nil || len(top3) != 3 {
		t.Fatalf("expect 3 records: err=%v n=%d", err, len(top3))
	}
	for i := range top3 {
		if top3[i].Ts != all[i].Ts {
			t.Fatalf("limit must keep the newest records: got %d want %d", top3[i].Ts, all[i].Ts)
		}
	}

	// limit 跨越小时边界：第 7 条落在第二个小时里
	page, err := queryRecs(cfg, 55, 1, from, to, 0, 7)
	if err != nil || len(page) != 7 {
		t.Fatalf("expect 7 records: err=%v n=%d", err, len(page))
	}
	for i := range page {
		if page[i].Ts != all[i].Ts {
			t.Fatalf("cross-hour limit mismatch at %d: got %d want %d", i, page[i].Ts, all[i].Ts)
		}
	}

	// 游标翻页仍从上一页末尾继续
	next, err := queryRecs(cfg, 55, 1, from, to, page[6].Ts, 4)
	if err != nil || len(next) != 4 {
		t.Fatalf("expect 4 records: err=%v n=%d", err, len(next))
	}
	for i := range next {
		if next[i].Ts != all[7+i].Ts {
			t.Fatalf("cursor page mismatch at %d: got %d want %d", i, next[i].Ts, all[7+i].Ts)
		}
	}
}

// pushTs 往环里塞一条只有 ts 的行，pad 用于把行撑到指定字节数以测试字节预算。
func pushTs(r *lineRing, ts int64, pad int) {
	rec := EvalRecord{Ts: ts}
	if pad > 0 {
		rec.Error = strings.Repeat("x", pad)
	}
	line, err := json.Marshal(&rec)
	if err != nil {
		panic(err)
	}
	r.push(line)
}

func tsOf(recs []EvalRecord) []int64 {
	out := make([]int64, 0, len(recs))
	for _, r := range recs {
		out = append(out, r.Ts)
	}
	return out
}

func TestLineRing(t *testing.T) {
	r := newLineRing(3)
	r.reset(3, 1<<20)
	for i := 1; i <= 5; i++ {
		pushTs(r, int64(i), 0)
	}
	// 保留最后 3 条（3/4/5），倒序输出
	if got := tsOf(r.appendDesc(nil)); !reflect.DeepEqual(got, []int64{5, 4, 3}) {
		t.Fatalf("count eviction wrong: %v", got)
	}
	if r.dropped {
		t.Fatal("count eviction is normal limit semantics, must not raise the byte-budget flag")
	}

	// 未填满时同样按倒序输出
	r.reset(10, 1<<20)
	pushTs(r, 1, 0)
	pushTs(r, 2, 0)
	if got := tsOf(r.appendDesc(nil)); !reflect.DeepEqual(got, []int64{2, 1}) {
		t.Fatalf("partial ring wrong: %v", got)
	}

	// 一条都没 push 过
	r.reset(5, 1<<20)
	if got := r.appendDesc(nil); len(got) != 0 {
		t.Fatalf("empty ring should yield nothing, got %+v", got)
	}

	// 恰好填满、以及绕圈多轮后的顺序
	r.reset(2, 1<<20)
	pushTs(r, 1, 0)
	pushTs(r, 2, 0)
	if got := tsOf(r.appendDesc(nil)); !reflect.DeepEqual(got, []int64{2, 1}) {
		t.Fatalf("exactly-full ring wrong: %v", got)
	}
	for i := 3; i <= 8; i++ {
		pushTs(r, int64(i), 0)
	}
	if got := tsOf(r.appendDesc(nil)); !reflect.DeepEqual(got, []int64{8, 7}) {
		t.Fatalf("wrapped ring wrong: %v", got)
	}
}

// 字节预算：条数还没满就先撞上字节上限时，淘汰的必须是更旧的那几条，并把 dropped 立起来。
func TestLineRingByteBudget(t *testing.T) {
	r := newLineRing(100)
	// 单条约 1KB，预算 3KB：条数闸（100）用不上，只有字节闸生效
	r.reset(100, 3*1024)
	for i := 1; i <= 10; i++ {
		pushTs(r, int64(i), 1000)
	}
	got := tsOf(r.appendDesc(nil))
	if len(got) == 0 || len(got) >= 10 {
		t.Fatalf("byte budget should keep a middle number of records, got %d", len(got))
	}
	if !r.dropped {
		t.Fatal("byte-budget eviction must be flagged, otherwise a capacity cap looks like missing data")
	}
	if r.bytes > 3*1024 {
		t.Fatalf("ring holds %d bytes, over the %d budget", r.bytes, 3*1024)
	}
	// 留下的必须是最新的那一段且连续倒序
	for i, ts := range got {
		if want := int64(10 - i); ts != want {
			t.Fatalf("expect newest records kept, at %d got %d want %d", i, ts, want)
		}
	}

	// 单条就超预算：宁可超一条，也不能返回空——空列表会被读成"当时没有记录"
	r.reset(100, 16)
	pushTs(r, 42, 500)
	if got := tsOf(r.appendDesc(nil)); !reflect.DeepEqual(got, []int64{42}) {
		t.Fatalf("an oversized single record must still be returned, got %v", got)
	}
}

// 字节预算必须约束**实际驻留**，而不只是在册字节。
//
// TestLineRingByteBudget 只断言 r.bytes，看不到被淘汰槽位仍压着的底层数组：字节闸主导时
// size 长期停在远低于 maxLines 的水位，新元素落在 start+size 处逐格前移，若淘汰只挪 start，
// start 会走遍全部 maxLines 个槽位、每个各留一份大数组。修复前本用例实测 296.9MB / 32MB
// 预算（9.3 倍），修复后驻留与在册条数同阶。
func TestLineRingRetainedCapBounded(t *testing.T) {
	const (
		capacity = 2000
		lineSize = 145 * 1024
		budget   = int64(32 * 1024 * 1024)
	)
	r := newLineRing(capacity)
	r.reset(capacity, budget)

	line := []byte(strings.Repeat("x", lineSize))
	for i := 0; i < 5*capacity; i++ {
		r.push(line)
	}

	var retained int64
	holding := 0
	for i := range r.buf {
		retained += int64(cap(r.buf[i]))
		if cap(r.buf[i]) > 0 {
			holding++
		}
	}
	// 留一倍余量给 append 的扩容策略；真正要挡住的是「驻留随 maxLines 线性增长」
	if retained > 2*budget {
		t.Fatalf("ring retains %d bytes of backing arrays, over twice the %d byte budget: "+
			"evicted slots are not being handed to the next write position", retained, budget)
	}
	if holding > r.size+1 {
		t.Fatalf("%d slots still hold a backing array but only %d records are live", holding, r.size)
	}
}

// 环形缓冲的三个不变量：条数不超上限、bytes 与在环元素的实际长度始终一致、
// 输出严格倒序。bytes 有三处加减（字节淘汰、条数覆盖、追加），只有直接查不变量才能确认
// 它们互相对得上——记错了会让 queryRecords 的预算扣减跟着错。
func TestLineRingInvariants(t *testing.T) {
	const capacity = 8
	r := newLineRing(capacity)

	check := func(step string) {
		t.Helper()
		if r.size > r.maxLines {
			t.Fatalf("%s: size %d exceeds maxLines %d", step, r.size, r.maxLines)
		}
		var want int64
		var prev int64 = -1
		for i := 0; i < r.size; i++ {
			line := r.buf[(r.start+i)%r.maxLines]
			want += int64(len(line))
			ts, ok := probeTs(line)
			if !ok {
				t.Fatalf("%s: unreadable line in ring", step)
			}
			if ts <= prev {
				t.Fatalf("%s: ring not in ascending order: %d after %d", step, ts, prev)
			}
			prev = ts
		}
		if r.bytes != want {
			t.Fatalf("%s: bytes accounting drifted: tracked %d, actual %d", step, r.bytes, want)
		}
	}

	// 伪随机但确定的一串 (maxLines, maxBytes, pad) 组合，覆盖字节闸与条数闸交替生效
	seed := 1
	next := func(mod int) int {
		seed = (seed*1103515245 + 12345) & 0x7fffffff
		return seed % mod
	}
	var ts int64
	for round := 0; round < 200; round++ {
		r.reset(1+next(capacity), int64(64+next(4096)))
		check("after reset")
		for i := 0; i < 20; i++ {
			ts++
			pushTs(r, ts, next(600))
			check(fmt.Sprintf("round %d push %d", round, i))
		}
		r.appendDesc(nil)
	}

	// 单条超预算：允许超出 maxBytes（否则只能返回空），但仍要保持账目一致
	r.reset(4, 32)
	ts++
	pushTs(r, ts, 500)
	check("single oversized line")
	if r.size != 1 {
		t.Fatalf("an oversized single line must be kept, size=%d", r.size)
	}
	if r.bytes <= r.maxBytes {
		t.Fatal("this case is meant to exercise bytes > maxBytes; adjust the padding")
	}
}

// probeTs 的前缀快路径必须与通用 JSON 解析等价，快路径不匹配时要能退回去。
func TestProbeTs(t *testing.T) {
	line, err := json.Marshal(&EvalRecord{Ts: 1754400000123, RuleId: 9})
	if err != nil {
		t.Fatal(err)
	}
	if ts, ok := fastProbeTs(line); !ok || ts != 1754400000123 {
		t.Fatalf("fast path failed on our own output: ts=%d ok=%v line=%s", ts, ok, line)
	}

	cases := []struct {
		name string
		line string
		want int64
		ok   bool
	}{
		{"字段顺序变了退回通用解析", `{"rule_id":9,"ts":123}`, 123, true},
		{"ts 为负退回通用解析", `{"ts":-5,"rule_id":9}`, -5, true},
		{"只有 ts 一个字段", `{"ts":123}`, 123, true},
		// 浮点 ts 本包写不出来，通用解析也解不进 int64，判为坏行跳过即可
		{"ts 是浮点", `{"ts":123.0}`, 0, false},
		{"半行", `{"ts":12`, 0, false},
		{"整行不是 JSON", `not json at all`, 0, false},
		{"超长数字不静默溢出", `{"ts":123456789012345678901234,"a":1}`, 0, false},
	}
	for _, c := range cases {
		got, ok := probeTs([]byte(c.line))
		if ok != c.ok || (ok && got != c.want) {
			t.Fatalf("%s: probeTs(%s) = (%d,%v), want (%d,%v)", c.name, c.line, got, ok, c.want, c.ok)
		}
	}
}

// 字节预算端到端：一次查询能拉进内存的记录必须按**字节**封顶，而不是只按条数。
// 单条记录满载可达 176KB，只按条数封顶的话，前端默认的一页 1000 条就是百 MB 级堆分配。
func TestQueryByteBudgetTruncates(t *testing.T) {
	const budget = 64 * 1024
	cfg := initForTest(t, func(c *Config) { c.MaxQueryBytes = budget })

	// 同一个小时文件内写 50 条较胖的记录（各约 2KB），总量明显超出预算
	base := truncHour(time.Now()).Add(5 * time.Minute)
	const total = 50
	for i := 0; i < total; i++ {
		Push(mkRecord(88, base.Add(time.Duration(i)*time.Millisecond), 30))
	}
	Shutdown()

	from := base.Add(-time.Hour).UnixMilli()
	to := base.Add(time.Hour).UnixMilli()

	res, err := queryRecords(cfg, 88, 1, from, to, 0, 100)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(res.Records) == 0 {
		t.Fatal("byte budget must not swallow the whole result")
	}
	if len(res.Records) >= total {
		t.Fatalf("expect the byte budget to cut the result, got all %d records", len(res.Records))
	}
	// 容量上限绝不能长得像"这段时间就这么多记录"：必须带标记和可读原因
	if !res.Truncated || res.Note == "" {
		t.Fatalf("truncation must be reported: truncated=%v note=%q", res.Truncated, res.Note)
	}
	// 留下的必须是最新的一段（排障先看最近发生了什么）
	newest := base.Add(time.Duration(total-1) * time.Millisecond).UnixMilli()
	if res.Records[0].Ts != newest {
		t.Fatalf("expect newest record first: got %d want %d", res.Records[0].Ts, newest)
	}
	for i := 1; i < len(res.Records); i++ {
		if res.Records[i].Ts >= res.Records[i-1].Ts {
			t.Fatalf("records not desc at %d", i)
		}
	}

	// 跨小时：预算在最新的小时就被吃光时，更老的小时文件应当连开都不开，且照样带截断标记
	cfg2 := initForTest(t, func(c *Config) { c.MaxQueryBytes = budget })
	newestHour := truncHour(time.Now()).Add(5 * time.Minute)
	olderHour := newestHour.Add(-time.Hour)
	for i := 0; i < total; i++ {
		Push(mkRecord(87, newestHour.Add(time.Duration(i)*time.Millisecond), 30))
		Push(mkRecord(87, olderHour.Add(time.Duration(i)*time.Millisecond), 30))
	}
	Shutdown()

	res3, err := queryRecords(cfg2, 87, 1, olderHour.Add(-time.Hour).UnixMilli(), newestHour.Add(time.Hour).UnixMilli(), 0, 1000)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if !res3.Truncated {
		t.Fatal("cross-hour budget exhaustion must be reported too")
	}
	for _, r := range res3.Records {
		if r.Ts < newestHour.UnixMilli() {
			t.Fatalf("budget should be spent on the newest hour first, got a record from %v", msTime(r.Ts))
		}
	}

	// 预算够用时不该有任何截断标记
	full := initForTest(t, nil)
	base2 := truncHour(time.Now()).Add(5 * time.Minute)
	for i := 0; i < total; i++ {
		Push(mkRecord(89, base2.Add(time.Duration(i)*time.Millisecond), 30))
	}
	Shutdown()
	res2, err := queryRecords(full, 89, 1, from, to, 0, 100)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(res2.Records) != total || res2.Truncated {
		t.Fatalf("default budget must not truncate: n=%d truncated=%v", len(res2.Records), res2.Truncated)
	}
}

// 回归：解码次数应正比于**返回条数**，而不是区间内的记录总数。
//
// 原实现对每条命中记录都做整条 json.Unmarshal（含 labels map 分配），再靠环形缓冲丢掉
// 绝大多数——一个被写满的小时文件就是几万次白白的结构体分配，GC 压力直接落到评估延迟上。
// 用分配次数来卡：真退回"每条都解码"的话，这里会差一个数量级。
func TestQueryDecodesOnlyReturnedRecords(t *testing.T) {
	cfg := initForTest(t, nil)

	base := truncHour(time.Now()).Add(5 * time.Minute)
	const total = 3000
	for i := 0; i < total; i++ {
		Push(mkRecord(90, base.Add(time.Duration(i)*time.Millisecond), 3))
	}
	Shutdown()

	from := base.Add(-time.Hour).UnixMilli()
	to := base.Add(time.Hour).UnixMilli()

	const limit = 10
	allocs := testing.AllocsPerRun(1, func() {
		res, err := queryRecords(cfg, 90, 1, from, to, 0, limit)
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
		if len(res.Records) != limit {
			t.Fatalf("expect %d records, got %d", limit, len(res.Records))
		}
	})

	t.Logf("scanned %d in-range records, returned %d, allocated %.0f objects", total, limit, allocs)
	// 每条命中记录都解码的话，光 3000 条 × (记录 + queries + series + labels map) 就远超这个数；
	// 只解返回的 10 条时实测两位数量级以下，留足余量只用来拦住退化
	if allocs > total {
		t.Fatalf("query allocated %.0f objects for %d in-range records (limit %d); "+
			"looks like every matched record is being decoded again", allocs, total, limit)
	}
}

// 并发闸：查询排不上队时必须给出可区分的 ErrBusy，而不是空列表。
func TestQueryConcurrencyGate(t *testing.T) {
	// 缩短排队等待，否则本用例要空等 queryAcquireTimeout
	old := queryAcquireTimeout
	queryAcquireTimeout = 50 * time.Millisecond
	defer func() { queryAcquireTimeout = old }()

	var rejected int32
	cfg := Config{Dir: t.TempDir(), MaxConcurrentQueries: 1}
	if err := Init(cfg, Hooks{OnQueryReject: func() { atomic.AddInt32(&rejected, 1) }}); err != nil {
		t.Fatalf("init error: %v", err)
	}
	t.Cleanup(Shutdown)

	now := time.Now()
	Push(mkRecord(91, now, 1))

	// 闸没占满时正常返回
	if _, err := QueryRecords(91, 1, now.Add(-time.Hour).UnixMilli(), now.UnixMilli(), 0, 10); err != nil {
		t.Fatalf("query should succeed when the gate is free: %v", err)
	}

	// 手工占满唯一的槽位
	mu.RLock()
	sem := querySem
	mu.RUnlock()
	sem <- struct{}{}

	start := time.Now()
	_, err := QueryRecords(91, 1, now.Add(-time.Hour).UnixMilli(), now.UnixMilli(), 0, 10)
	if err != ErrBusy {
		t.Fatalf("expect ErrBusy when the gate is full, got %v", err)
	}
	if elapsed := time.Since(start); elapsed < queryAcquireTimeout {
		t.Fatalf("should have queued for a while before giving up, gave up after %v", elapsed)
	}
	if atomic.LoadInt32(&rejected) != 1 {
		t.Fatalf("reject hook should fire exactly once, got %d", rejected)
	}

	// 槽位释放后立刻恢复
	<-sem
	if _, err := QueryRecords(91, 1, now.Add(-time.Hour).UnixMilli(), now.UnixMilli(), 0, 10); err != nil {
		t.Fatalf("query should recover once the slot is released: %v", err)
	}
}

// 并发数不超过闸值时一律不该被拒——center 的本机扇出正是按 QueryConcurrency() 对齐的，
// 这个约定一旦破了，合并部署下一次普通的页面请求就会看到半屏"引擎忙"。
func TestQueryGateAdmitsUpToItsSize(t *testing.T) {
	old := queryAcquireTimeout
	queryAcquireTimeout = 50 * time.Millisecond
	defer func() { queryAcquireTimeout = old }()

	const gate = 4
	initForTest(t, func(c *Config) { c.MaxConcurrentQueries = gate })

	if got := QueryConcurrency(); got != gate {
		t.Fatalf("QueryConcurrency() = %d, want %d; center sizes its local fanout by this", got, gate)
	}

	now := time.Now()
	Push(mkRecord(92, now, 1))

	mu.RLock()
	sem := querySem
	mu.RUnlock()
	if cap(sem) != gate {
		t.Fatalf("gate cap = %d, want %d", cap(sem), gate)
	}

	// 占住 gate-1 个槽位，最后一路查询仍必须畅通
	for i := 0; i < gate-1; i++ {
		sem <- struct{}{}
	}
	defer func() {
		for i := 0; i < gate-1; i++ {
			<-sem
		}
	}()

	if _, err := QueryRecords(92, 1, now.Add(-time.Hour).UnixMilli(), now.UnixMilli(), 0, 10); err != nil {
		t.Fatalf("query at exactly the gate size must not be rejected: %v", err)
	}
}

// 预算兜底按小时桶两遍执行：分界之前的小时整桶删、分界小时只删到腾够为止、
// 当前小时永不动；同一小时跨规则的文件属于同一个桶。
func TestPruneToBudget(t *testing.T) {
	cfg := Config{Dir: t.TempDir()}
	cfg.normalize()

	now := truncHour(time.Now())
	mk := func(rule string, hoursAgo int, size int64) string {
		at := now.Add(-time.Duration(hoursAgo) * time.Hour)
		dir := filepath.Join(cfg.Dir, rule, at.Format(dateLayout))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, at.Format(hourLayout)+".jsonl.gz")
		if err := os.WriteFile(p, make([]byte, size), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	// 桶（从旧到新）：h-5 = 200（跨两个规则）、h-4 = 100、h-3 = 200（跨两个规则）；
	// 当前小时 100 只计 total 不进候选。total = 600。
	h5a := mk("1_1", 5, 100)
	h5b := mk("2_1", 5, 100)
	h4 := mk("1_1", 4, 100)
	h3a := mk("1_1", 3, 100)
	h3b := mk("2_1", 3, 100)
	cur := mk("1_1", 0, 100)

	total, buckets := sweepExpiredAndMeasure(cfg)
	if total != 600 {
		t.Fatalf("total = %d, want 600 (current hour must be counted)", total)
	}
	if len(buckets) != 3 {
		t.Fatalf("buckets = %d, want 3 (current hour must not be a candidate)", len(buckets))
	}
	if got := buckets[now.Add(-5*time.Hour).Unix()]; got != 200 {
		t.Fatalf("same hour across rules must share one bucket, got %d", got)
	}

	// 预算 250 → 超出 350：h-5 整桶(200) + h-4 整桶(100) + h-3 桶里腾 50（即一个文件）
	pruneToBudget(cfg, buckets, total, 250)

	for _, p := range []string{h5a, h5b, h4} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("hours before the boundary must be fully pruned: %s survived", p)
		}
	}
	h3Left := 0
	for _, p := range []string{h3a, h3b} {
		if _, err := os.Stat(p); err == nil {
			h3Left++
		}
	}
	if h3Left != 1 {
		t.Fatalf("boundary hour must be pruned only until enough is freed, %d of 2 files left", h3Left)
	}
	if _, err := os.Stat(cur); err != nil {
		t.Fatalf("current hour file must never be pruned: %v", err)
	}
}
