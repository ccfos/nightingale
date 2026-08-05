package migrate

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// MySQL / PostgreSQL 分支各自依赖真实数据库（在线 DDL 语法、pg_index 目录），
// 这里覆盖通用分支与「重复执行不报错」这一所有方言都必须成立的性质
func TestMigrateNotificationRecordIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

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
