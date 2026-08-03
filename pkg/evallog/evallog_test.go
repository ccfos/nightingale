package evallog

import (
	"fmt"
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
