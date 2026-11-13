package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDashboardAdd_RejectsInvalidNames(t *testing.T) {
	c := pipelineContext(t, &Dashboard{})
	cases := []struct {
		name string
		want string
	}{
		{name: "", want: "Name is blank"},
		{name: "<script>", want: "invalid characters"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			dashboard := Dashboard{Name: tc.name}

			err := dashboard.Add(c)

			require.ErrorContains(t, err, tc.want)
			count, err := DashboardCount(c, "1 = 1")
			require.NoError(t, err)
			require.Zero(t, count)
		})
	}
}

func TestDashboardAdd_EnforcesNameUniquenessWithinGroup(t *testing.T) {
	c := pipelineContext(t, &Dashboard{})
	dashboard := Dashboard{Name: "CPU", GroupId: 7}
	require.NoError(t, dashboard.Add(c))
	duplicate := Dashboard{Name: "CPU", GroupId: 7}
	other := Dashboard{Name: "CPU", GroupId: 8}

	err := duplicate.Add(c)

	require.EqualError(t, err, "Dashboard already exists")
	require.NoError(t, other.Add(c))
	require.NotZero(t, dashboard.Id)
	require.Equal(t, dashboard.CreateAt, dashboard.UpdateAt)
	count, err := DashboardCount(c, "name = ?", "CPU")
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}

func TestDashboardUpdate_WritesOnlySelectedFields(t *testing.T) {
	c := pipelineContext(t, &Dashboard{})
	dashboard := Dashboard{Id: 1, Name: "CPU", GroupId: 7, Configs: "original", Tags: "linux prod "}
	require.NoError(t, c.DB.Create(&dashboard).Error)
	dashboard.Name = "Memory"
	dashboard.Configs = "discarded"

	require.NoError(t, dashboard.Update(c, "name"))
	got, err := DashboardGet(c, "id = ?", 1)

	require.NoError(t, err)
	require.Equal(t, "Memory", got.Name)
	require.Equal(t, "original", got.Configs)
	require.Equal(t, []string{"linux", "prod"}, got.TagsLst)
	dashboard.Name = ""
	require.ErrorContains(t, dashboard.Update(c, "name"), "blank")
	missing, err := DashboardGet(c, "id = ?", 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestDashboardGets_CombinesGroupAndPositiveNegativeTerms(t *testing.T) {
	c := pipelineContext(t, &Dashboard{})
	require.NoError(t, c.DB.Create(&[]Dashboard{
		{Id: 1, GroupId: 7, Name: "CPU alpha", Tags: "prod linux", Configs: "large payload"},
		{Id: 2, GroupId: 7, Name: "CPU beta", Tags: "prod legacy"},
		{Id: 3, GroupId: 8, Name: "CPU gamma", Tags: "prod"},
		{Id: 4, GroupId: 7, Name: "Memory", Tags: "prod"},
	}).Error)

	rows, err := DashboardGets(c, 7, "CPU prod -legacy")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(1), rows[0].Id)
	require.Equal(t, []string{"prod", "linux"}, rows[0].TagsLst)
	// List responses omit the potentially large dashboard configuration.
	require.Empty(t, rows[0].Configs)
	all, err := DashboardGetAll(c)
	require.NoError(t, err)
	require.Len(t, all, 4)
	selected, err := DashboardGetsByIds(c, []int64{4, 1})
	require.NoError(t, err)
	require.Equal(t, []int64{1, 4}, []int64{selected[0].Id, selected[1].Id})
	empty, err := DashboardGetsByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, empty)
}

func TestDashboardDel_CascadesOnlyItsChartsAndGroups(t *testing.T) {
	for _, withGroups := range []bool{false, true} {
		name := "without groups"
		if withGroups {
			name = "with groups"
		}
		t.Run(name, func(t *testing.T) {
			c := pipelineContext(t, &Dashboard{}, &ChartGroup{}, &Chart{})
			require.NoError(t, c.DB.Create(&[]Dashboard{{Id: 1}, {Id: 2}}).Error)
			require.NoError(t, c.DB.Create(&ChartGroup{Id: 20, DashboardId: 2}).Error)
			require.NoError(t, c.DB.Create(&Chart{Id: 200, GroupId: 20}).Error)
			if withGroups {
				require.NoError(t, c.DB.Create(&ChartGroup{Id: 10, DashboardId: 1}).Error)
				require.NoError(t, c.DB.Create(&Chart{Id: 100, GroupId: 10}).Error)
			}

			require.NoError(t, (&Dashboard{Id: 1}).Del(c))

			rows, err := DashboardGetAll(c)
			require.NoError(t, err)
			require.Equal(t, []Dashboard{{Id: 2}}, rows)
			var groups []ChartGroup
			require.NoError(t, c.DB.Find(&groups).Error)
			require.Equal(t, []ChartGroup{{Id: 20, DashboardId: 2}}, groups)
			var charts []Chart
			require.NoError(t, c.DB.Find(&charts).Error)
			require.Equal(t, []Chart{{Id: 200, GroupId: 20}}, charts)
		})
	}
}

func TestDashAnnotationWrites_RoundTripTagsAndClearEditableFields(t *testing.T) {
	c := pipelineContext(t, &DashAnnotation{})
	row := DashAnnotation{
		DashboardId: 7, PanelId: "cpu", TagsJSON: []string{"deploy"},
		Description: "release", CreateBy: "alice",
	}
	require.NoError(t, row.Add(c))
	got, err := DashAnnotationGet(c, "id = ?", row.Id)
	require.NoError(t, err)
	require.Equal(t, []string{"deploy"}, got.TagsJSON)
	got.TagsJSON = []string{}
	got.Description = ""
	got.CreateBy = "eve"

	require.NoError(t, got.Update(c))
	stored, err := DashAnnotationGet(c, "id = ?", row.Id)

	require.NoError(t, err)
	require.Empty(t, stored.TagsJSON)
	require.Empty(t, stored.Description)
	require.Equal(t, "alice", stored.CreateBy)
	require.NoError(t, DashAnnotationDel(c, row.Id))
	missing, err := DashAnnotationGet(c, "id = ?", row.Id)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestDashAnnotationGets_FiltersOverlappingIntervalsBeforeLimit(t *testing.T) {
	c := pipelineContext(t, &DashAnnotation{})
	require.NoError(t, c.DB.Create(&[]DashAnnotation{
		{Id: 1, DashboardId: 7, TimeStart: 0, TimeEnd: 9, Tags: `[]`},
		{Id: 2, DashboardId: 7, TimeStart: 0, TimeEnd: 10, Tags: `["boundary"]`},
		{Id: 3, DashboardId: 7, TimeStart: 20, TimeEnd: 30, Tags: `[]`},
		{Id: 4, DashboardId: 7, TimeStart: 21, TimeEnd: 30, Tags: `[]`},
		{Id: 5, DashboardId: 8, TimeStart: 10, TimeEnd: 20, Tags: `[]`},
	}).Error)

	rows, err := DashAnnotationGets(c, 7, 10, 20, 1)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].Id)
	require.Equal(t, []string{"boundary"}, rows[0].TagsJSON)
	rows, err = DashAnnotationGets(c, 7, 10, 20, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(3), rows[1].Id)
}

func TestDashAnnotationGet_ReportsMalformedStoredTags(t *testing.T) {
	c := pipelineContext(t, &DashAnnotation{})
	require.NoError(t, c.DB.Create(&DashAnnotation{Id: 1, Tags: "invalid"}).Error)

	_, err := DashAnnotationGet(c, "id = ?", 1)

	require.Error(t, err)
}
