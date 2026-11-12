package models

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMetricFilter_AddTrimsFieldsAndPersistsPermissions(t *testing.T) {
	c := permissionContext(t, &MetricFilter{})
	filter := MetricFilter{
		ID: 1, Name: "  CPU  ", Configs: "  cpu_usage > 80  ", CreateBy: "alice",
		GroupsPerm: []GroupPerm{{Gid: 7, Write: true}},
	}

	err := filter.Add(c)

	require.NoError(t, err)
	stored, err := MetricFilterGet(c, filter.ID)
	require.NoError(t, err)
	require.Equal(t, "CPU", stored.Name)
	require.Equal(t, "cpu_usage > 80", stored.Configs)
	require.Equal(t, filter.GroupsPerm, stored.GroupsPerm)
	require.Equal(t, "alice", stored.CreateBy)
	require.Positive(t, stored.CreateAt)
	require.Equal(t, stored.CreateAt, stored.UpdateAt)
}

func TestMetricFilter_InvalidInputIsNotPersisted(t *testing.T) {
	cases := []struct {
		name   string
		filter MetricFilter
		want   string
	}{
		{name: "blank name", filter: MetricFilter{Name: "  ", Configs: "cpu"}, want: "name is blank"},
		{name: "blank config", filter: MetricFilter{Name: "CPU", Configs: "  "}, want: "configs is blank"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := permissionContext(t, &MetricFilter{})

			err := tc.filter.Add(c)

			require.EqualError(t, err, tc.want)
			rows, err := MetricFilterGets(c, "")
			require.NoError(t, err)
			require.Empty(t, rows)
		})
	}
}

func TestMetricFilter_UpdatePreservesCreationMetadataAndClearsPermissions(t *testing.T) {
	c := permissionContext(t, &MetricFilter{})
	filter := MetricFilter{
		ID: 1, Name: "CPU", Configs: "old", CreateAt: 100, CreateBy: "alice", UpdateAt: 100,
		GroupsPerm: []GroupPerm{{Gid: 7, Write: true}},
	}
	require.NoError(t, c.DB.Create(&filter).Error)
	filter.Name, filter.Configs = "  Memory  ", " mem > 80 "
	filter.CreateBy, filter.CreateAt = "forged", 200
	filter.GroupsPerm, filter.UpdateBy = []GroupPerm{}, "bob"

	err := filter.Update(c)

	require.NoError(t, err)
	stored, err := MetricFilterGet(c, filter.ID)
	require.NoError(t, err)
	require.Equal(t, "Memory", stored.Name)
	require.Equal(t, "mem > 80", stored.Configs)
	require.Empty(t, stored.GroupsPerm)
	require.Equal(t, "bob", stored.UpdateBy)
	require.Equal(t, "alice", stored.CreateBy)
	require.Equal(t, int64(100), stored.CreateAt)
}

func TestMetricFilter_InvalidUpdateLeavesStoredConfiguration(t *testing.T) {
	c := permissionContext(t, &MetricFilter{})
	filter := MetricFilter{ID: 1, Name: "CPU", Configs: "original"}
	require.NoError(t, filter.Add(c))
	filter.Configs = " "

	err := filter.Update(c)

	require.EqualError(t, err, "configs is blank")
	stored, err := MetricFilterGet(c, filter.ID)
	require.NoError(t, err)
	require.Equal(t, "original", stored.Configs)
}

func TestMetricFilterDel_DeletesOnlyRequestedFilters(t *testing.T) {
	c := permissionContext(t, &MetricFilter{})
	require.NoError(t, c.DB.Create(&[]MetricFilter{
		{ID: 1, Name: "CPU", Configs: "cpu"},
		{ID: 2, Name: "Memory", Configs: "mem"},
	}).Error)

	require.NoError(t, MetricFilterDel(c, nil))
	require.NoError(t, MetricFilterDel(c, []int64{2}))

	rows, err := MetricFilterGets(c, "name = ?", "CPU")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(1), rows[0].ID)
	_, err = MetricFilterGet(c, 2)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestRole_AddRejectsDuplicateName(t *testing.T) {
	c := permissionContext(t, &Role{})
	role := Role{Name: "reader", Note: "Can read"}
	require.NoError(t, role.Add(c))

	err := (&Role{Name: "reader", Note: "duplicate"}).Add(c)

	require.EqualError(t, err, "role name already exists")
	count, err := RoleCount(c, "name = ?", "reader")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	stored, err := RoleGet(c, "id = ?", role.Id)
	require.NoError(t, err)
	require.Equal(t, "Can read", stored.Note)
}

func TestRole_UpdateWritesOnlySelectedFields(t *testing.T) {
	c := permissionContext(t, &Role{})
	role := Role{Name: "reader", Note: "old"}
	require.NoError(t, role.Add(c))
	role.Name, role.Note = "ignored", "new"

	err := role.Update(c, "note")

	require.NoError(t, err)
	stored, err := RoleGet(c, "id = ?", role.Id)
	require.NoError(t, err)
	require.Equal(t, "reader", stored.Name)
	require.Equal(t, "new", stored.Note)
}

func TestRole_DelPreservesOtherRoles(t *testing.T) {
	c := permissionContext(t, &Role{})
	role := Role{Name: "remove"}
	require.NoError(t, role.Add(c))
	require.NoError(t, (&Role{Name: "keep"}).Add(c))

	err := role.Del(c)

	require.NoError(t, err)
	roles, err := RoleGetsAll(c)
	require.NoError(t, err)
	require.Len(t, roles, 1)
	require.Equal(t, "keep", roles[0].Name)
	missing, err := RoleGet(c, "id = ?", role.Id)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestRole_QueriesPropagateDatabaseErrors(t *testing.T) {
	c := permissionContext(t, &Role{})
	sqlDB, err := c.DB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = RoleGets(c, "")

	require.ErrorContains(t, err, "failed to query roles")
	require.ErrorContains(t, (&Role{Name: "reader"}).Add(c), "failed to query user")
}

func TestSsoConfig_UpdatePreservesNameAndAdvancesRevision(t *testing.T) {
	c := permissionContext(t, &SsoConfig{})
	config := SsoConfig{Name: "oidc", Content: `{"enabled":false}`, UpdateAt: 1}
	require.NoError(t, config.Create(c))
	config.Name, config.Content = "ignored", `{"enabled":true}`

	err := config.Update(c)

	require.NoError(t, err)
	stored, err := (&SsoConfig{Name: "oidc"}).Query(c)
	require.NoError(t, err)
	require.Equal(t, `{"enabled":true}`, stored.Content)
	require.Equal(t, "oidc", stored.Name)
	require.Greater(t, stored.UpdateAt, int64(1))
	revision, err := SsoConfigLastUpdateTime(c)
	require.NoError(t, err)
	require.Equal(t, stored.UpdateAt, revision)
	count, err := SsoConfigCountByName(c, "oidc")
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	rows, err := SsoConfigGets(c)
	require.NoError(t, err)
	require.Equal(t, []SsoConfig{stored}, rows)
}

func TestBoard_UpdateValidatesBeforeWritingSelectedFields(t *testing.T) {
	c := permissionContext(t, &Board{})
	board := Board{Id: 1, Name: "CPU", Note: "original", GroupId: 7}
	require.NoError(t, c.DB.Create(&board).Error)
	board.Name, board.Note = "Memory", "ignored"

	err := board.Update(c, "name")

	require.NoError(t, err)
	stored, err := BoardGetByID(c, 1)
	require.NoError(t, err)
	require.Equal(t, "Memory", stored.Name)
	require.Equal(t, "original", stored.Note)
	board.Name = ""
	require.EqualError(t, board.Update(c, "name"), "Name is blank")
	stored, err = BoardGetByID(c, 1)
	require.NoError(t, err)
	require.Equal(t, "Memory", stored.Name)
}
