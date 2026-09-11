package models_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// sqlRecorder 记录 gorm 实际发出的 SQL，用于断言子查询是否真的下推到了同一条语句里
type sqlRecorder struct {
	gormlogger.Interface
	stmts []string
}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.stmts = append(r.stmts, sql)
}

func (r *sqlRecorder) reset() { r.stmts = nil }

func (r *sqlRecorder) lastSelectOn(table string) string {
	for i := len(r.stmts) - 1; i >= 0; i-- {
		if strings.HasPrefix(r.stmts[i], "SELECT") && strings.Contains(r.stmts[i], table) {
			return r.stmts[i]
		}
	}
	return ""
}

func newNotifyRecordCtx(t *testing.T) (*ctx.Context, *sqlRecorder) {
	t.Helper()
	recorder := &sqlRecorder{Interface: gormlogger.Discard}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: recorder})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	if err := db.AutoMigrate(&models.AlertHisEvent{}, &models.NotificationRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return ctx.NewContext(context.Background(), db, true), recorder
}

// 事件 1/2/3 由规则 61 在时间窗内通知过，事件 4 属于另一条规则，事件 5 从未被通知过
func seedNotifiedEvents(t *testing.T, c *ctx.Context) {
	t.Helper()
	events := []models.AlertHisEvent{
		{Id: 1, GroupId: 1, Severity: 1, IsRecovered: 0, RuleId: 11, RuleName: "cpu high", Tags: "host=a", TriggerTime: 100, LastEvalTime: 100},
		{Id: 2, GroupId: 1, Severity: 2, IsRecovered: 0, RuleId: 12, RuleName: "mem high", Tags: "host=b", TriggerTime: 120, LastEvalTime: 120},
		{Id: 3, GroupId: 1, Severity: 1, IsRecovered: 1, RuleId: 13, RuleName: "disk full", Tags: "host=c", TriggerTime: 140, LastEvalTime: 140},
		{Id: 4, GroupId: 1, Severity: 1, IsRecovered: 0, RuleId: 14, RuleName: "net down", Tags: "host=d", TriggerTime: 160, LastEvalTime: 160},
		{Id: 5, GroupId: 1, Severity: 1, IsRecovered: 0, RuleId: 15, RuleName: "proc gone", Tags: "host=e", TriggerTime: 180, LastEvalTime: 180},
	}
	for i := range events {
		if err := events[i].Add(c); err != nil {
			t.Fatalf("seed event %d: %v", events[i].Id, err)
		}
	}

	// 同一事件多条通知记录，用来验证去重：三个事件只应产生三行结果。
	// 显式指定 Id：模型上 id 声明为 bigint，SQLite 下这样的主键不会成为 rowid 别名、
	// 自增不生效（MySQL/PostgreSQL 无此问题），与 seedHisEvents 的写法保持一致
	records := []models.NotificationRecord{
		{Id: 1, NotifyRuleID: 61, EventId: 1, Channel: "dingtalk", Target: "t1", CreatedAt: 100},
		{Id: 2, NotifyRuleID: 61, EventId: 1, Channel: "email", Target: "t2", CreatedAt: 110},
		{Id: 3, NotifyRuleID: 61, EventId: 2, Channel: "dingtalk", Target: "t1", CreatedAt: 120},
		{Id: 4, NotifyRuleID: 61, EventId: 3, Channel: "dingtalk", Target: "t1", CreatedAt: 140},
		{Id: 5, NotifyRuleID: 61, EventId: 3, Channel: "email", Target: "t2", CreatedAt: 150},
		{Id: 6, NotifyRuleID: 62, EventId: 4, Channel: "dingtalk", Target: "t1", CreatedAt: 160},
		// 规则 61 但落在时间窗之外，不应被选中
		{Id: 7, NotifyRuleID: 61, EventId: 5, Channel: "dingtalk", Target: "t1", CreatedAt: 900},
	}
	for i := range records {
		if err := records[i].Add(c); err != nil {
			t.Fatalf("seed record %d: %v", i, err)
		}
	}
}

// 子查询必须下推进同一条 SQL，且 LIMIT/OFFSET 落在外层事件表上而不是子查询里
func TestNotifiedEventIdsScopePushdown(t *testing.T) {
	c, recorder := newNotifyRecordCtx(t)
	seedNotifiedEvents(t, c)

	scope := models.NotifiedEventIdsScope(c, 61, 0, 200)
	recorder.reset()

	_, err := models.AlertHisEventGets(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 2, 0, nil, scope)
	if err != nil {
		t.Fatalf("gets: %v", err)
	}

	sql := recorder.lastSelectOn("alert_his_event")
	if sql == "" {
		t.Fatal("no select on alert_his_event recorded")
	}

	if !strings.Contains(sql, "notification_record") {
		t.Fatalf("notification_record subquery not inlined, sql = %s", sql)
	}
	if !strings.Contains(sql, "notify_rule_id") || !strings.Contains(sql, "event_id") {
		t.Fatalf("subquery lost its filter columns, sql = %s", sql)
	}
	// 旧实现靠 GROUP BY event_id + ORDER BY MAX(created_at) 去重排序，逼出临时表和文件排序
	if strings.Contains(strings.ToUpper(sql), "GROUP BY") {
		t.Fatalf("subquery should rely on IN semantics rather than GROUP BY, sql = %s", sql)
	}

	// LIMIT 只出现一次，且位置在子查询之后 —— 即作用于外层事件表而非通知记录表
	if n := strings.Count(strings.ToUpper(sql), "LIMIT"); n != 1 {
		t.Fatalf("expect exactly one LIMIT on the outer query, got %d, sql = %s", n, sql)
	}
	if strings.Index(strings.ToUpper(sql), "LIMIT") < strings.Index(sql, "notification_record") {
		t.Fatalf("LIMIT applied inside the subquery, sql = %s", sql)
	}
}

// total 是过滤后的真实总数，翻页不重不漏；旧实现下 total 只统计当前页那批 id
func TestNotifyRuleEventsPagination(t *testing.T) {
	c, _ := newNotifyRecordCtx(t)
	seedNotifiedEvents(t, c)

	scope := models.NotifiedEventIdsScope(c, 61, 0, 200)

	total, err := models.AlertHisEventTotal(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", nil, scope)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	// 事件 1/2/3：事件 4 属别的规则，事件 5 的通知记录在时间窗外；重复通知记录不重复计数
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}

	page1, err := models.AlertHisEventGets(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 2, 0, nil, scope)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	assertIds(t, eventIdsOf(page1), 3, 2)

	page2, err := models.AlertHisEventGets(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "", 2, 2, nil, scope)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	assertIds(t, eventIdsOf(page2), 1)

	// 游标翻页与 offset 翻页结果一致
	last := page1[len(page1)-1]
	cursorPage2, err := models.AlertHisEventGetsByCursor(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "",
		last.LastEvalTime, last.Id, 2, nil, scope)
	if err != nil {
		t.Fatalf("cursor page2: %v", err)
	}
	assertIds(t, eventIdsOf(cursorPage2), 1)
}

// 叠加事件侧过滤条件时，total 和每页数据必须来自同一套条件
func TestNotifyRuleEventsWithFilters(t *testing.T) {
	c, _ := newNotifyRecordCtx(t)
	seedNotifiedEvents(t, c)

	scope := models.NotifiedEventIdsScope(c, 61, 0, 200)

	// 通知过的 1/2/3 中，severity=1 的是事件 1 和 3
	total, err := models.AlertHisEventTotal(c, nil, nil, 0, 1000, 1, -1, nil, nil, 0, "", nil, scope)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 2 {
		t.Fatalf("total severity=1 = %d, want 2", total)
	}

	lst, err := models.AlertHisEventGets(c, nil, nil, 0, 1000, 1, -1, nil, nil, 0, "", 30, 0, nil, scope)
	if err != nil {
		t.Fatalf("gets: %v", err)
	}
	assertIds(t, eventIdsOf(lst), 3, 1)

	// 关键字过滤同样只在通知过的事件里生效：事件 4 的 net down 未被本规则通知
	total, err = models.AlertHisEventTotal(c, nil, nil, 0, 1000, -1, -1, nil, nil, 0, "net", nil, scope)
	if err != nil {
		t.Fatalf("total: %v", err)
	}
	if total != 0 {
		t.Fatalf("total query=net = %d, want 0", total)
	}
}

func TestNotificationRecordDeleteBefore(t *testing.T) {
	c, _ := newNotifyRecordCtx(t)
	seedNotifiedEvents(t, c)

	// 时间窗内共 6 条记录，分批删除时每批不超过 batchSize
	deleted, err := models.NotificationRecordDeleteBefore(c, 200, 4)
	if err != nil {
		t.Fatalf("delete batch1: %v", err)
	}
	if deleted != 4 {
		t.Fatalf("batch1 deleted = %d, want 4", deleted)
	}

	deleted, err = models.NotificationRecordDeleteBefore(c, 200, 4)
	if err != nil {
		t.Fatalf("delete batch2: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("batch2 deleted = %d, want 2", deleted)
	}

	// 删空后返回 0，调用方据此结束循环
	deleted, err = models.NotificationRecordDeleteBefore(c, 200, 4)
	if err != nil {
		t.Fatalf("delete batch3: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("batch3 deleted = %d, want 0", deleted)
	}

	// created_at=900 的记录晚于保留边界，必须留下
	left, err := models.NotificationRecordsGet(c, "1=1")
	if err != nil {
		t.Fatalf("get left: %v", err)
	}
	if len(left) != 1 || left[0].CreatedAt != 900 {
		t.Fatalf("left records = %+v, want the only one created_at=900", left)
	}
}
