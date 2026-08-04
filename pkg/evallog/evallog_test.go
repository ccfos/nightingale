package evallog

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
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
	if err := Init(cfg, nil); err != nil {
		t.Fatalf("init error: %v", err)
	}
	t.Cleanup(Shutdown)
	return cfg
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

	recs, err := queryRecords(cfg, 21, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
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

	recs, err := queryRecords(cfg, 22, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
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

	recs, err := queryRecords(cfg, 100, 1, now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), 0, 0)
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
	page1, _ := queryRecords(cfg, 100, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 3)
	if len(page1) != 3 {
		t.Fatalf("expect 3, got %d", len(page1))
	}
	page2, _ := queryRecords(cfg, 100, 1, 0, now.Add(time.Hour).UnixMilli(), page1[2].Ts, 3)
	if len(page2) != 3 || page2[0].Ts >= page1[2].Ts {
		t.Fatalf("cursor page wrong: %+v", page2)
	}

	// 其他规则查不到
	other, _ := queryRecords(cfg, 200, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
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
	recs, err := queryRecords(cfg, 7, 1, old.Add(-time.Minute).UnixMilli(), time.Now().UnixMilli(), 0, 0)
	if err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(recs) != 2 {
		t.Fatalf("expect 2 records across hours, got %d", len(recs))
	}
}

func TestSeriesAndPointsCap(t *testing.T) {
	initForTest(t, func(c *Config) {
		c.MaxSeriesPerQuery = 3
		c.MaxPointsPerSeries = 2
	})

	r := NewRecord(1, 1, time.Now().UnixMilli())
	q := QueryRecord{Ref: "A", SeriesTotal: 5}
	for i := 0; i < 5; i++ {
		q.Series = append(q.Series, SeriesSample{
			Labels: map[string]string{"i": fmt.Sprintf("%d", i)},
			Points: [][2]float64{{1, 1}, {2, 2}, {3, 3}, {4, 4}},
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

	recs, err := queryRecords(cfg, 9, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
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

	recs, err := queryRecords(cfg, 11, 1, 0, now.Add(time.Hour).UnixMilli(), 0, 0)
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

	recs, err := queryRecords(cfg, 101, 1, past.Add(-time.Hour).UnixMilli(), past.Add(time.Hour).UnixMilli(), 0, 0)
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

	recs, err := queryRecords(cfg, 31, 1, now.Add(-time.Hour).UnixMilli(), now.Add(time.Hour).UnixMilli(), 0, 0)
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
	key := "77_1"

	b := w.budgetFor(key, date, now)
	b.bytes = 900 * 1024

	// 模拟 housekeep 因空闲关闭句柄：appender 没了，预算必须还在
	delete(w.appenders, key)

	again := w.budgetFor(key, date, now)
	if again.bytes != 900*1024 {
		t.Fatalf("budget must survive appender close, got %d", again.bytes)
	}
	// 跨日才清零
	if next := w.budgetFor(key, "2999-01-01", now); next.bytes != 0 {
		t.Fatalf("budget must reset on new date, got %d", next.bytes)
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

	all, err := queryRecords(cfg, 55, 1, from, to, 0, 0)
	if err != nil || len(all) != 15 {
		t.Fatalf("expect 15 records: err=%v n=%d", err, len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].Ts > all[i-1].Ts {
			t.Fatalf("records not desc at %d: %d > %d", i, all[i].Ts, all[i-1].Ts)
		}
	}

	// limit 小于单个小时的条数：只能拿到最新小时里最新的 3 条
	top3, err := queryRecords(cfg, 55, 1, from, to, 0, 3)
	if err != nil || len(top3) != 3 {
		t.Fatalf("expect 3 records: err=%v n=%d", err, len(top3))
	}
	for i := range top3 {
		if top3[i].Ts != all[i].Ts {
			t.Fatalf("limit must keep the newest records: got %d want %d", top3[i].Ts, all[i].Ts)
		}
	}

	// limit 跨越小时边界：第 7 条落在第二个小时里
	page, err := queryRecords(cfg, 55, 1, from, to, 0, 7)
	if err != nil || len(page) != 7 {
		t.Fatalf("expect 7 records: err=%v n=%d", err, len(page))
	}
	for i := range page {
		if page[i].Ts != all[i].Ts {
			t.Fatalf("cross-hour limit mismatch at %d: got %d want %d", i, page[i].Ts, all[i].Ts)
		}
	}

	// 游标翻页仍从上一页末尾继续
	next, err := queryRecords(cfg, 55, 1, from, to, page[6].Ts, 4)
	if err != nil || len(next) != 4 {
		t.Fatalf("expect 4 records: err=%v n=%d", err, len(next))
	}
	for i := range next {
		if next[i].Ts != all[7+i].Ts {
			t.Fatalf("cursor page mismatch at %d: got %d want %d", i, next[i].Ts, all[7+i].Ts)
		}
	}
}

func TestRecordRing(t *testing.T) {
	r := newRecordRing(3)
	for i := 1; i <= 5; i++ {
		r.push(EvalRecord{Ts: int64(i)})
	}
	got := r.appendDesc(nil)
	if len(got) != 3 {
		t.Fatalf("expect 3 kept, got %d", len(got))
	}
	// 保留最后 3 条（3/4/5），倒序输出
	for i, want := range []int64{5, 4, 3} {
		if got[i].Ts != want {
			t.Fatalf("at %d: got %d want %d", i, got[i].Ts, want)
		}
	}

	// 未填满时同样按倒序输出
	r2 := newRecordRing(10)
	r2.push(EvalRecord{Ts: 1})
	r2.push(EvalRecord{Ts: 2})
	got2 := r2.appendDesc(nil)
	if len(got2) != 2 || got2[0].Ts != 2 || got2[1].Ts != 1 {
		t.Fatalf("unexpected: %+v", got2)
	}

	// 一条都没 push 过：惰性分配下底层数组还是 nil，不能在此处取模 panic
	if got3 := newRecordRing(5).appendDesc(nil); len(got3) != 0 {
		t.Fatalf("empty ring should yield nothing, got %+v", got3)
	}

	// 恰好填满、以及绕圈多轮后的顺序
	r4 := newRecordRing(2)
	for i := 1; i <= 2; i++ {
		r4.push(EvalRecord{Ts: int64(i)})
	}
	if got := r4.appendDesc(nil); len(got) != 2 || got[0].Ts != 2 || got[1].Ts != 1 {
		t.Fatalf("exactly-full ring wrong: %+v", got)
	}
	for i := 3; i <= 8; i++ {
		r4.push(EvalRecord{Ts: int64(i)})
	}
	if got := r4.appendDesc(nil); len(got) != 2 || got[0].Ts != 8 || got[1].Ts != 7 {
		t.Fatalf("wrapped ring wrong: %+v", got)
	}
}

func TestPruneToBudget(t *testing.T) {
	dir := t.TempDir()
	var files []hourFile
	var total int64
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, fmt.Sprintf("%d.jsonl.gz", i))
		os.WriteFile(p, make([]byte, 100), 0644)
		files = append(files, hourFile{path: p, hour: time.Now().Add(time.Duration(i) * time.Hour), size: 100})
		total += 100
	}

	pruneToBudget(files, total, 250)

	// 最旧的 3 个（0/1/2）应被删，留下 2 个
	for i := 0; i < 3; i++ {
		if _, err := os.Stat(files[i].path); !os.IsNotExist(err) {
			t.Fatalf("expect %s removed", files[i].path)
		}
	}
	for i := 3; i < 5; i++ {
		if _, err := os.Stat(files[i].path); err != nil {
			t.Fatalf("expect %s kept", files[i].path)
		}
	}
}
