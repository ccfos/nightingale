package models

import (
	"strings"
	"testing"

	"gorm.io/driver/mysql"
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
