package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChartGroupAdd_RejectsInvalidDashboardAndUnsafeName(t *testing.T) {
	c := pipelineContext(t, &ChartGroup{})
	cases := []struct {
		name  string
		group ChartGroup
		want  string
	}{
		{name: "missing dashboard", group: ChartGroup{Name: "CPU"}, want: "dashboard_id"},
		{name: "unsafe name", group: ChartGroup{DashboardId: 1, Name: "<script>"}, want: "invalid characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.group.Add(c)

			require.ErrorContains(t, err, tc.want)
			groups, err := ChartGroupsOf(c, 1)
			require.NoError(t, err)
			require.Empty(t, groups)
		})
	}
}

func TestChartGroupUpdate_WritesSelectedWeightAndPreservesName(t *testing.T) {
	c := pipelineContext(t, &ChartGroup{})
	group := ChartGroup{DashboardId: 7, Name: "CPU", Weight: 5}
	require.NoError(t, group.Add(c))
	require.NoError(t, NewDefaultChartGroup(c, 7))
	group.Name = "discarded"
	group.Weight = -1

	require.NoError(t, group.Update(c, "weight"))
	groups, err := ChartGroupsOf(c, 7)

	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, group.Id, groups[0].Id)
	require.Equal(t, "CPU", groups[0].Name)
	require.Equal(t, -1, groups[0].Weight)
	require.Equal(t, "Default chart group", groups[1].Name)
	group.DashboardId = 0
	require.ErrorContains(t, group.Update(c, "weight"), "dashboard_id")
}

func TestChartGroupDel_CascadesOnlyItsCharts(t *testing.T) {
	c := pipelineContext(t, &ChartGroup{}, &Chart{})
	require.NoError(t, c.DB.Create(&[]ChartGroup{{Id: 1, DashboardId: 7}, {Id: 2, DashboardId: 7}}).Error)
	require.NoError(t, c.DB.Create(&[]Chart{{Id: 10, GroupId: 1}, {Id: 20, GroupId: 2}}).Error)

	require.NoError(t, (&ChartGroup{Id: 1}).Del(c))
	groups, err := ChartGroupsOf(c, 7)

	require.NoError(t, err)
	require.Equal(t, []ChartGroup{{Id: 2, DashboardId: 7}}, groups)
	deleted, err := ChartsOf(c, 1)
	require.NoError(t, err)
	require.Empty(t, deleted)
	retained, err := ChartsOf(c, 2)
	require.NoError(t, err)
	require.Equal(t, []Chart{{Id: 20, GroupId: 2}}, retained)
}

func TestChartWrites_PersistSelectedFieldsAndOrderByWeight(t *testing.T) {
	c := pipelineContext(t, &Chart{})
	first := Chart{GroupId: 7, Configs: "first", Weight: 10}
	second := Chart{GroupId: 7, Configs: "second", Weight: 20}
	other := Chart{GroupId: 8, Configs: "other"}
	require.NoError(t, first.Add(c))
	require.NoError(t, second.Add(c))
	require.NoError(t, other.Add(c))
	second.Configs = "discarded"
	second.Weight = 0

	require.NoError(t, second.Update(c, "weight"))
	rows, err := ChartsOf(c, 7)

	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, second.Id, rows[0].Id)
	require.Equal(t, "second", rows[0].Configs)
	require.NoError(t, second.Del(c))
	rows, err = ChartsOf(c, 7)
	require.NoError(t, err)
	require.Equal(t, []Chart{first}, rows)
	retained, err := ChartsOf(c, 8)
	require.NoError(t, err)
	require.Equal(t, []Chart{other}, retained)
}

func TestChartShareGetsByIds_ReturnsRequestedSharesInIDOrder(t *testing.T) {
	c := pipelineContext(t, &ChartShare{})
	first := ChartShare{Id: 1, DatasourceId: 7, Configs: "first"}
	second := ChartShare{Id: 2, DatasourceId: 8, Configs: "second"}
	require.NoError(t, first.Add(c))
	require.NoError(t, second.Add(c))

	rows, err := ChartShareGetsByIds(c, []int64{2, 99, 1})

	require.NoError(t, err)
	require.Equal(t, []ChartShare{first, second}, rows)
	empty, err := ChartShareGetsByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}
