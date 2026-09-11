package migrate

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openNotifyRecordDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// MySQL / PostgreSQL 分支各自依赖真实数据库（在线 DDL 语法、pg_index 目录），
// 这里覆盖通用分支与「重复执行不报错」这一所有方言都必须成立的性质
func TestMigrateNotificationRecordIndexes(t *testing.T) {
	db := openNotifyRecordDB(t)

	if err := db.Exec(`CREATE TABLE notification_record (
		id integer primary key autoincrement, notify_rule_id integer not null default 0,
		event_id integer not null, sub_id integer, channel varchar(255) not null,
		status integer, target varchar(1024) not null, details varchar(2048) default '',
		created_at integer not null)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	migrateNotificationRecordIndexes(db)

	for _, idx := range notificationRecordIndexes {
		if !db.Migrator().HasIndex("notification_record", idx.name) {
			t.Fatalf("index %s not created", idx.name)
		}
	}

	// 每次启动都会跑一遍，必须幂等
	migrateNotificationRecordIndexes(db)
	for _, idx := range notificationRecordIndexes {
		if !db.Migrator().HasIndex("notification_record", idx.name) {
			t.Fatalf("index %s lost after second run", idx.name)
		}
	}
}

// 建索引跑在 MigrateTables 的异步 goroutine 里，可能抢在主协程给 notification_record
// 补 notify_rule_id 之前执行（老库、或用旧版 docker/sqlite.sql 初始化出来的库）。
// 此时缺列的索引应被跳过而不是报错退出，且不牵连只依赖 created_at 的那个索引；
// 列补齐后的下一次启动要能把它补建出来
func TestMigrateNotificationRecordIndexesBeforeColumnMigrated(t *testing.T) {
	db := openNotifyRecordDB(t)

	// notify rule 特性之前的表结构：没有 notify_rule_id 列
	if err := db.Exec(`CREATE TABLE notification_record (
		id integer primary key autoincrement,
		event_id integer not null, sub_id integer, channel varchar(255) not null,
		status integer, target varchar(1024) not null, details varchar(2048) default '',
		created_at integer not null)`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	migrateNotificationRecordIndexes(db)

	if db.Migrator().HasIndex("notification_record", "idx_nr_rule_created_evt") {
		t.Fatal("idx_nr_rule_created_evt should be skipped while notify_rule_id is missing")
	}
	if !db.Migrator().HasIndex("notification_record", "idx_nr_created_at") {
		t.Fatal("idx_nr_created_at only needs created_at, it should still be created")
	}

	// 主协程的 AutoMigrate 把列补上之后，下次启动补建
	if err := db.Exec("ALTER TABLE notification_record ADD COLUMN notify_rule_id integer not null default 0").Error; err != nil {
		t.Fatalf("add column: %v", err)
	}

	migrateNotificationRecordIndexes(db)
	for _, idx := range notificationRecordIndexes {
		if !db.Migrator().HasIndex("notification_record", idx.name) {
			t.Fatalf("index %s not created after notify_rule_id was migrated", idx.name)
		}
	}
}
