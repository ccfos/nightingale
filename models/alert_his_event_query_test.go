package models_test

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

func newHisEventCtx(t *testing.T) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.AutoMigrate(&models.AlertHisEvent{}); err != nil {
		t.Fatalf("migrate alert_his_event: %v", err)
	}

	return ctx.NewContext(context.Background(), db, true)
}

func seedHisEvents(t *testing.T, c *ctx.Context) {
	t.Helper()
	events := []models.AlertHisEvent{
		{Id: 1, GroupId: 1, Severity: 2, IsRecovered: 0, RuleId: 11, RuleName: "cpu high", Tags: "host=a", Hash: "h-cpu-a", TriggerTime: 100, LastEvalTime: 100},
		{Id: 2, GroupId: 1, Severity: 1, IsRecovered: 0, RuleId: 12, RuleName: "mem high", Tags: "host=b", Hash: "h-mem-b", TriggerTime: 150, LastEvalTime: 200},
		{Id: 3, GroupId: 1, Severity: 1, IsRecovered: 1, RuleId: 12, RuleName: "mem high", Tags: "host=b", Hash: "h-mem-b", TriggerTime: 90, LastEvalTime: 200},
		{Id: 4, GroupId: 2, Severity: 3, IsRecovered: 0, RuleId: 13, RuleName: "disk full", Tags: "host=c", Hash: "h-disk-c", TriggerTime: 300, LastEvalTime: 300},
		{Id: 5, GroupId: 2, Severity: 2, IsRecovered: 1, RuleId: 13, RuleName: "disk full", Tags: "host=c", Hash: "h-disk-c2", TriggerTime: 240, LastEvalTime: 250},
		{Id: 6, GroupId: 1, Severity: 2, IsRecovered: 0, RuleId: 11, RuleName: "cpu high", Tags: "host=a", Hash: "h-cpu-a2", TriggerTime: 400, LastEvalTime: 400},
	}
	for i := range events {
		if err := events[i].Add(c); err != nil {
			t.Fatalf("seed event %d: %v", events[i].Id, err)
		}
	}
}

func eventIdsOf(lst []models.AlertHisEvent) []int64 {
	ids := make([]int64, 0, len(lst))
	for i := range lst {
		ids = append(ids, lst[i].Id)
	}
	return ids
}

func assertIds(t *testing.T, got []int64, want ...int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}

func TestAlertHisEventGetsOrder(t *testing.T) {
	c := newHisEventCtx(t)
	seedHisEvents(t, c)

	// 按 last_eval_time desc, id desc 排序；id 2/3 的 last_eval_time 相同，id 大者在前
	lst, err := models.AlertHisEventGets(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 10, 0, nil)
	if err != nil {
		t.Fatalf("gets: %v", err)
	}
	assertIds(t, eventIdsOf(lst), 6, 4, 5, 3, 2, 1)
}

func TestAlertHisEventTotalAndFilters(t *testing.T) {
	c := newHisEventCtx(t)
	seedHisEvents(t, c)

	total, err := models.AlertHisEventTotal(c, nil, []int64{1}, 0, 1000, -1, -1, nil, nil, 0, "", nil)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 4 {
		t.Fatalf("total by group 1 = %d, want 4", total)
	}

	total, err = models.AlertHisEventTotal(c, nil, nil, 0, 1000, 2, 0, nil, nil, 0, "", nil)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 2 {
		t.Fatalf("total severity=2 not recovered = %d, want 2", total)
	}

	total, err = models.AlertHisEventTotal(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "cpu", nil)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 2 {
		t.Fatalf("total query=cpu = %d, want 2", total)
	}

	// 时间窗过滤用 last_eval_time：id 3 trigger_time=90 但 last_eval_time=200，不应命中
	total, err = models.AlertHisEventTotal(c, nil, nil, 0, 150, -1, -1, nil, nil, 0, "", nil)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 1 {
		t.Fatalf("total in [0,150] = %d, want 1", total)
	}
}

func TestAlertHisEventGetsByCursor(t *testing.T) {
	c := newHisEventCtx(t)
	seedHisEvents(t, c)

	// 游标翻页结果应与 offset 翻页完全一致
	page1, err := models.AlertHisEventGets(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 2, 0, nil)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	assertIds(t, eventIdsOf(page1), 6, 4)

	last := page1[len(page1)-1]
	page2, err := models.AlertHisEventGetsByCursor(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", last.LastEvalTime, last.Id, 2, nil)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	assertIds(t, eventIdsOf(page2), 5, 3)

	offsetPage2, err := models.AlertHisEventGets(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 2, 2, nil)
	if err != nil {
		t.Fatalf("offset page2: %v", err)
	}
	assertIds(t, eventIdsOf(offsetPage2), 5, 3)

	// last_eval_time 相同时游标按 id 严格递减，不重不漏
	last = page2[len(page2)-1]
	page3, err := models.AlertHisEventGetsByCursor(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", last.LastEvalTime, last.Id, 2, nil)
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	assertIds(t, eventIdsOf(page3), 2, 1)

	// 无游标等价于第一页
	first, err := models.AlertHisEventGetsByCursor(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 0, 0, 2, nil)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	assertIds(t, eventIdsOf(first), 6, 4)
}

func TestAlertHisEventHashScope(t *testing.T) {
	c := newHisEventCtx(t)
	seedHisEvents(t, c)

	hashScope := models.AlertHisEventHashScope("h-mem-b")

	// 列表与计数是两条独立路径，都要验证，否则按 hash 筛选后翻页的 total 可能不对
	total, err := models.AlertHisEventTotal(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", nil, hashScope)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 2 {
		t.Fatalf("total by hash = %d, want 2", total)
	}

	lst, err := models.AlertHisEventGets(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 10, 0, nil, hashScope)
	if err != nil {
		t.Fatalf("gets: %v", err)
	}
	assertIds(t, eventIdsOf(lst), 3, 2)

	// 精确匹配：前缀相同的 hash 不应命中
	lst, err = models.AlertHisEventGets(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 10, 0, nil,
		models.AlertHisEventHashScope("h-mem"))
	if err != nil {
		t.Fatalf("gets by prefix hash: %v", err)
	}
	if len(lst) != 0 {
		t.Fatalf("gets by prefix hash = %v, want empty", eventIdsOf(lst))
	}

	// hash 不存在时返回空列表且不报错
	total, err = models.AlertHisEventTotal(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", nil,
		models.AlertHisEventHashScope("h-not-exist"))
	if err != nil {
		t.Fatalf("total by absent hash: %v", err)
	}
	if total != 0 {
		t.Fatalf("total by absent hash = %d, want 0", total)
	}

	// 与其他筛选条件叠加：hash 命中 2 条，其中未恢复的只有 id 2
	lst, err = models.AlertHisEventGets(c, nil, []int64{1}, 0, 1000, 1, 0, nil, nil, 0, "mem", 10, 0, nil, hashScope)
	if err != nil {
		t.Fatalf("gets with filters: %v", err)
	}
	assertIds(t, eventIdsOf(lst), 2)

	// 游标翻页同样要带上 scope，否则深翻页会返回没被 hash 过滤的行
	page1, err := models.AlertHisEventGetsByCursor(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 0, 0, 1, nil, hashScope)
	if err != nil {
		t.Fatalf("cursor page1: %v", err)
	}
	assertIds(t, eventIdsOf(page1), 3)

	last := page1[len(page1)-1]
	page2, err := models.AlertHisEventGetsByCursor(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", last.LastEvalTime, last.Id, 1, nil, hashScope)
	if err != nil {
		t.Fatalf("cursor page2: %v", err)
	}
	assertIds(t, eventIdsOf(page2), 2)

	// 时间窗仍然生效：两条的 last_eval_time 都是 200，窗口 [0,150] 内一条都不该命中
	total, err = models.AlertHisEventTotal(c, nil, nil, 0, 150, -1, -1, nil, nil, 0, "", nil, hashScope)
	if err != nil {
		t.Fatalf("total in window: %v", err)
	}
	if total != 0 {
		t.Fatalf("total by hash in [0,150] = %d, want 0", total)
	}
}
