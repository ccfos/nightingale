package memsto

import (
	"context"
	"reflect"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestTargetsOfAlertRuleSyncRetriesAfterPartialFailure 覆盖：某条 host 规则的 target
// 查询失败时，不能记下签名。否则签名不变就一直跳过重算，这条规则的机器会长期缺失
// （center 自身以及经 /v1/n9e/targets-of-alert-rule 取数的 edge 都会漏告警）。
func TestTargetsOfAlertRuleSyncRetriesAfterPartialFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AlertRule{}); err != nil {
		t.Fatalf("migrate alert_rule: %v", err)
	}
	// 故意缺少 tags / host_tags 列：按 tags 过滤的规则查询会失败，而签名只依赖
	// count / update_at，仍能正常计算——模拟一次只影响单条规则查询的瞬时错误。
	if err := db.Exec(`CREATE TABLE target (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_id INTEGER,
		ident TEXT,
		note TEXT,
		update_at INTEGER,
		host_ip TEXT,
		agent_version TEXT,
		engine_name TEXT,
		os TEXT
	)`).Error; err != nil {
		t.Fatalf("create target table: %v", err)
	}
	if err := db.Exec(`CREATE TABLE target_busi_group (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target_ident TEXT,
		group_id INTEGER,
		update_at INTEGER
	)`).Error; err != nil {
		t.Fatalf("create target_busi_group table: %v", err)
	}
	if err := db.Exec(`INSERT INTO target(ident, update_at, engine_name) VALUES ('host-1', 1, 'edge-a')`).Error; err != nil {
		t.Fatalf("insert target: %v", err)
	}
	for _, r := range []*models.AlertRule{
		{Id: 1, Name: "all-hosts", Prod: models.HOST, RuleConfig: `{"queries":[]}`, UpdateAt: 1},
		{Id: 2, Name: "by-tag", Prod: models.HOST, RuleConfig: `{"queries":[{"key":"tags","op":"==","values":["env=prod"]}]}`, UpdateAt: 1},
	} {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("insert rule: %v", err)
		}
	}

	tc := &TargetsOfAlertRuleCacheType{
		ctx:     ctx.NewContext(context.Background(), db, true),
		targets: make(map[string]map[int64][]string),
	}

	if err := tc.syncTargets(); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if tc.lastSig != "" {
		t.Fatalf("lastSig = %q after partial failure, want empty so next round recomputes", tc.lastSig)
	}
	// 残缺结果仍然先用上：成功的规则不受影响
	if lst, has := tc.Get("edge-a", 1); !has || !reflect.DeepEqual(lst, []string{"host-1"}) {
		t.Fatalf("Get(edge-a, 1) = %v, %v, want [host-1], true", lst, has)
	}

	// 错误消失（补上列），输入签名不变，下一轮仍必须重算并补齐规则 2
	for _, col := range []string{"tags", "host_tags"} {
		if err := db.Exec("ALTER TABLE target ADD COLUMN " + col + " TEXT").Error; err != nil {
			t.Fatalf("add column %s: %v", col, err)
		}
	}
	if err := db.Exec(`UPDATE target SET tags = 'env=prod ', host_tags = '[]'`).Error; err != nil {
		t.Fatalf("update target tags: %v", err)
	}

	if err := tc.syncTargets(); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if lst, has := tc.Get("edge-a", 2); !has || !reflect.DeepEqual(lst, []string{"host-1"}) {
		t.Fatalf("Get(edge-a, 2) = %v, %v, want [host-1], true", lst, has)
	}
	if tc.lastSig == "" {
		t.Fatal("lastSig still empty after a complete sync")
	}
}
