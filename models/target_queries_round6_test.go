package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanMigrateBg_RequiresLegacyGroupAssignments(t *testing.T) {
	cases := []struct {
		name    string
		targets []Target
		want    bool
	}{
		{name: "empty table"},
		{name: "already migrated", targets: []Target{{Ident: "node", GroupId: 0}}},
		{name: "legacy assignment", targets: []Target{{Ident: "node", GroupId: 7}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := permissionContext(t, &Target{})
			if len(tc.targets) > 0 {
				require.NoError(t, c.DB.Create(&tc.targets).Error)
			}

			allowed := CanMigrateBg(c)

			require.Equal(t, tc.want, allowed)
		})
	}
}

func TestCanMigrateBg_MissingTablePreventsMigration(t *testing.T) {
	c := permissionContext(t, &BusiGroup{})

	allowed := CanMigrateBg(c)

	require.False(t, allowed)
}

func TestTargetGetsByFilter_PaginatesAndEnrichesGroupsAndTags(t *testing.T) {
	c := permissionContext(t, &Target{}, &TargetBusiGroup{}, &BusiGroup{})
	require.NoError(t, c.DB.Create(&BusiGroup{Id: 7, Name: "ops"}).Error)
	require.NoError(t, c.DB.Create(&[]Target{
		{Ident: "node-c", Tags: "env=prod team=ops"},
		{Ident: "node-a", Tags: "env=prod"},
		{Ident: "node-b", Tags: "env=prod team=ops"},
	}).Error)
	require.NoError(t, c.DB.Create(&TargetBusiGroup{Id: 1, TargetIdent: "node-b", GroupId: 7}).Error)
	query := GetHostsQuery([]HostQuery{{Key: "hosts", Op: "=~", Values: []interface{}{"node-*"}}})

	rows, err := TargetGetsByFilter(c, query, 1, 1)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "node-b", rows[0].Ident)
	require.Equal(t, []string{"env=prod", "team=ops"}, rows[0].TagsJSON)
	require.Equal(t, []int64{7}, rows[0].GroupIds)
	require.Len(t, rows[0].GroupObjs, 1)
	require.Equal(t, "ops", rows[0].GroupObjs[0].Name)
}

func TestTarget_UpdateFieldsMapPreservesUnselectedFields(t *testing.T) {
	c := permissionContext(t, &Target{})
	target := Target{Id: 1, Ident: "node", Note: "old", Tags: "env=prod"}
	require.NoError(t, c.DB.Create(&target).Error)

	err := target.UpdateFieldsMap(c, map[string]interface{}{"note": "", "engine_name": "edge-a"})

	require.NoError(t, err)
	stored, err := TargetGet(c, "ident = ?", "node")
	require.NoError(t, err)
	require.Empty(t, stored.Note)
	require.Equal(t, "edge-a", stored.EngineName)
	require.Equal(t, "env=prod", stored.Tags)
}
