package models

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Binding with tags regressed to writing through a second connection while SQLite held the transaction lock.
func TestTargetBindBgidsPersistsTagsRegression(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "targets.db") + "?_pragma=busy_timeout(200)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(2)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&Target{}, &TargetBusiGroup{}))
	c := ctx.NewContext(context.Background(), db, true)
	require.NoError(t, db.Create(&Target{Ident: "host-bind", Tags: "env=prod "}).Error)

	err = TargetBindBgids(c, []string{"host-bind"}, []int64{7}, []string{"team=ops", "region=west"})

	require.NoError(t, err)
	groupIDs, err := TargetGroupIdsGetByIdent(c, "host-bind")
	require.NoError(t, err)
	require.Equal(t, []int64{7}, groupIDs)
	target, err := TargetGetByIdent(c, "host-bind")
	require.NoError(t, err)
	require.NotNil(t, target)
	require.ElementsMatch(t, []string{"env=prod", "team=ops", "region=west"}, strings.Fields(target.Tags))
}
