package models

import (
	"context"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestConfigExternalEqQuotesReservedIdentifierForMySQL(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:9910)/gorm?charset=utf8&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
	})
	if err != nil {
		t.Fatalf("open dry-run MySQL database: %v", err)
	}

	stmt := db.Model(&Configs{}).
		Where("ckey = ?", JWT_SIGNING_KEY).
		Where(configExternalEq(0)).
		Pluck("cval", &[]string{}).
		Statement
	if stmt.Error != nil {
		t.Fatalf("build config query: %v", stmt.Error)
	}

	if got := stmt.SQL.String(); !strings.Contains(got, "WHERE ckey = ? AND `external` = ?") {
		t.Fatalf("expected external to be quoted in MySQL query, got %q", got)
	}
}

// 保留字校验按「名字是否变化」生效：新建和改名都要拦，存量同名记录仍可编辑。
// 早先按 id == 0 判断，等于更新路径完全没有校验，先建 my_event 再改名成 event 就能绕过。
func TestUserVariableReservedKeyCheckedOnRename(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Configs{}); err != nil {
		t.Fatalf("automigrate configs: %v", err)
	}
	c := ctx.NewContext(context.Background(), db, true)

	if err := ConfigsUserVariableInsert(c, Configs{Ckey: "event", Cval: "x"}); err == nil {
		t.Error("creating a variable named event should be rejected")
	}

	if err := ConfigsUserVariableInsert(c, Configs{Ckey: "my_event", Cval: "x"}); err != nil {
		t.Fatalf("insert my_event: %v", err)
	}
	var mine Configs
	if err := DB(c).Where("ckey = ?", "my_event").First(&mine).Error; err != nil {
		t.Fatalf("load my_event: %v", err)
	}

	err = ConfigsUserVariableUpdate(c, Configs{Id: mine.Id, Ckey: "event", Cval: "x", External: ConfigExternal})
	if err == nil || !strings.Contains(err.Error(), "reserved words") {
		t.Errorf("renaming my_event to event should be rejected, got: %v", err)
	}
	var after Configs
	DB(c).Where("id = ?", mine.Id).First(&after)
	if after.Ckey != "my_event" {
		t.Errorf("ckey was mutated to %q", after.Ckey)
	}

	// 改值不改名，不该被保留字校验挡住
	if err := ConfigsUserVariableUpdate(c, Configs{Id: mine.Id, Ckey: "my_event", Cval: "y", External: ConfigExternal}); err != nil {
		t.Errorf("updating cval only should pass: %v", err)
	}

	// 存量脏数据：库里早就有一条叫 params 的变量，编辑其它字段仍要放行
	legacy := Configs{Ckey: "params", Cval: "old", External: ConfigExternal}
	if err := DB(c).Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy record: %v", err)
	}
	if err := ConfigsUserVariableUpdate(c, Configs{Id: legacy.Id, Ckey: "params", Cval: "new", External: ConfigExternal}); err != nil {
		t.Errorf("legacy record with a reserved name should stay editable: %v", err)
	}
}
