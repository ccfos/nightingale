package models

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func targetBehaviorContext(t *testing.T, tables ...interface{}) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tables...))
	return ctx.NewContext(context.Background(), db, true)
}

func TestTargetGets_ComposesFiltersWithoutDuplicatingMultiGroupTargets(t *testing.T) {
	c := targetBehaviorContext(t, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]Target{
		{Id: 1, Ident: "node-a", HostIp: "192.0.2.1", Tags: "env=prod ", OS: "linux"},
		{Id: 2, Ident: "node-b", HostIp: "192.0.2.2", Tags: "env=test ", OS: "linux"},
		{Id: 3, Ident: "ungrouped", HostIp: "192.0.2.3", OS: "windows"},
	}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "node-a", GroupId: 7}, {TargetIdent: "node-a", GroupId: 8}, {TargetIdent: "node-b", GroupId: 8},
	}).Error)
	cases := []struct {
		name    string
		options []BuildTargetWhereOption
		want    []string
	}{
		{name: "all", want: []string{"node-a", "node-b", "ungrouped"}},
		{
			name:    "ungrouped",
			options: []BuildTargetWhereOption{BuildTargetWhereWithBgids([]int64{0})},
			want:    []string{"ungrouped"},
		},
		{
			name: "multiple groups", options: []BuildTargetWhereOption{BuildTargetWhereWithBgids([]int64{7, 8})},
			want: []string{"node-a", "node-b"},
		},
		{
			name: "group or ungrouped", options: []BuildTargetWhereOption{BuildTargetWhereWithBgids([]int64{0, 7})},
			want: []string{"node-a", "ungrouped"},
		},
		{
			name: "hosts excluding ident",
			options: []BuildTargetWhereOption{
				BuildTargetWhereWithHosts([]string{"192.0.2.1", "node-b"}), BuildTargetWhereExcludeIdents([]string{"node-b"}),
			},
			want: []string{"node-a"},
		},
		{
			name: "idents excluding one",
			options: []BuildTargetWhereOption{
				BuildTargetWhereWithIdents([]string{"node-a", "node-b"}), BuildTargetWhereExcludeIdents([]string{"node-b"}),
			},
			want: []string{"node-a"},
		},
		{
			name: "positive and negative search", options: []BuildTargetWhereOption{BuildTargetWhereWithQuery("linux -test")},
			want: []string{"node-a"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			targets, err := TargetGets(c, 10, 0, "ident", false, tc.options...)
			require.NoError(t, err)
			total, err := TargetTotal(c, tc.options...)

			require.NoError(t, err)
			names := make([]string, 0, len(targets))
			for _, target := range targets {
				names = append(names, target.Ident)
			}
			require.Equal(t, tc.want, names)
			require.Equal(t, int64(len(tc.want)), total)
		})
	}
}

func TestTargetGets_PaginatesInRequestedOrder(t *testing.T) {
	c := targetBehaviorContext(t, &Target{})
	require.NoError(t, c.DB.Create(&[]Target{{Ident: "a"}, {Ident: "b"}, {Ident: "c"}}).Error)

	targets, err := TargetGets(c, 1, 1, "ident", true)

	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, "b", targets[0].Ident)
}

func TestTargetsGetIdentsByIdentsAndHostIps_DeduplicatesAndReportsMissingInputs(t *testing.T) {
	c := targetBehaviorContext(t, &Target{})
	require.NoError(t, c.DB.Create(&[]Target{
		{Id: 1, Ident: "node-a", HostIp: "192.0.2.1"}, {Id: 2, Ident: "node-b", HostIp: "192.0.2.1"},
	}).Error)

	missing, idents, err := TargetsGetIdentsByIdentsAndHostIps(c,
		[]string{"node-a", "absent"}, []string{"192.0.2.1", "192.0.2.99"})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"node-a", "node-b"}, idents)
	require.Equal(t, map[string]string{"absent": "Ident not found", "192.0.2.99": "HostIp not found"}, missing)
}

func TestTargetsGetIdsByIdentsAndHostIps_DeduplicatesAndReportsMissingInputs(t *testing.T) {
	c := targetBehaviorContext(t, &Target{})
	require.NoError(t, c.DB.Create(&[]Target{
		{Id: 1, Ident: "node-a", HostIp: "192.0.2.1"}, {Id: 2, Ident: "node-b", HostIp: "192.0.2.1"},
	}).Error)

	missing, ids, err := TargetsGetIdsByIdentsAndHostIps(c,
		[]string{"node-a", "absent"}, []string{"192.0.2.1", "192.0.2.99"})

	require.NoError(t, err)
	require.ElementsMatch(t, []int64{1, 2}, ids)
	require.Equal(t, map[string]string{"absent": "Ident not found", "192.0.2.99": "HostIp not found"}, missing)
}

func TestTargetGetTags_CombinesDistinctTagsWithBusinessGroupFirst(t *testing.T) {
	c := targetBehaviorContext(t, &Target{})
	require.NoError(t, c.DB.Create(&[]Target{
		{Ident: "node-a", Tags: "env=prod busigroup=ops ", HostTags: []string{"os=linux", "env=prod"}},
		{Ident: "node-b", Tags: "env=prod zone=west ", HostTags: []string{"os=linux"}},
	}).Error)

	tags, err := TargetGetTags(c, nil, false, "busigroup")

	require.NoError(t, err)
	require.Equal(t, []string{"busigroup=ops", "env=prod", "os=linux", "zone=west"}, tags)
	tags, err = TargetGetTags(c, []string{"node-a"}, true, "")
	require.NoError(t, err)
	require.Equal(t, []string{"busigroup=ops", "env=prod"}, tags)
	tags, err = TargetGetTags(c, []string{"missing"}, false, "")
	require.NoError(t, err)
	require.Empty(t, tags)
}

func TestTargetFillGroup_CombinesCacheAndDatabaseMemberships(t *testing.T) {
	c := targetBehaviorContext(t, &BusiGroup{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&BusiGroup{Id: 8, Name: "from database"}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "node-a", GroupId: 7}, {TargetIdent: "node-a", GroupId: 8}, {TargetIdent: "node-a", GroupId: 99},
	}).Error)
	target := Target{Ident: "node-a"}
	cache := map[int64]*BusiGroup{7: {Id: 7, Name: "from cache"}}

	err := target.FillGroup(c, cache)

	require.NoError(t, err)
	require.ElementsMatch(t, []int64{7, 8, 99}, target.GroupIds)
	require.Len(t, target.GroupObjs, 2)
	require.Equal(t, "from database", cache[8].Name)
	require.True(t, target.MatchGroupId(1, 8))
	require.False(t, target.MatchGroupId(1, 2))
}

func TestTarget_GetTagMapsSeparatesCustomAndHostTags(t *testing.T) {
	target := Target{Tags: "env=prod malformed zone=west ", HostTags: []string{"os=linux", "malformed"}}

	custom := target.GetTagsMap()
	host := target.GetHostTagsMap()

	require.Equal(t, map[string]string{"env": "prod", "zone": "west"}, custom)
	require.Equal(t, map[string]string{"os": "linux"}, host)
}

func TestTarget_FillMetaCopiesRuntimeMeasurements(t *testing.T) {
	target := Target{Ident: "node-a", Tags: "env=prod "}
	meta := HostMeta{
		MemUtil: 50, CpuUtil: 25, CpuNum: 8, UnixTime: 100, Offset: 12, Arch: "arm64", RemoteAddr: "192.0.2.1",
	}

	target.FillMeta(&meta)

	require.Equal(t, float64(50), target.MemUtil)
	require.Equal(t, float64(25), target.CpuUtil)
	require.Equal(t, 8, target.CpuNum)
	require.Equal(t, int64(100), target.UnixTime)
	require.Equal(t, int64(12), target.Offset)
	require.Equal(t, "arm64", target.Arch)
	require.Equal(t, "192.0.2.1", target.RemoteAddr)
	require.Equal(t, "node-a", target.Ident)
	require.Equal(t, "env=prod ", target.Tags)
}
