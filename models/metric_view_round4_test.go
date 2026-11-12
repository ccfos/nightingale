package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricViewGets_IncludesPublicAndOwnedViewsSortedByCategoryName(t *testing.T) {
	c := pipelineContext(t, &MetricView{})
	require.NoError(t, c.DB.Create(&[]MetricView{
		{Id: 1, Name: "z public", Cate: 0, CreateBy: 8},
		{Id: 2, Name: "b private", Cate: 1, CreateBy: 7},
		{Id: 3, Name: "a private", Cate: 1, CreateBy: 7},
		{Id: 4, Name: "foreign", Cate: 1, CreateBy: 8},
		{Id: 5, Name: "a public", Cate: 0, CreateBy: 8},
	}).Error)

	rows, err := MetricViewGets(c, 7)

	require.NoError(t, err)
	ids := make([]int64, len(rows))
	for i := range rows {
		ids[i] = rows[i].Id
	}
	require.Equal(t, []int64{5, 1, 3, 2}, ids)
}

func TestMetricViewAdd_RejectsBlankConfiguration(t *testing.T) {
	c := pipelineContext(t, &MetricView{})
	view := MetricView{Name: "CPU", Configs: " "}

	err := view.Add(c)

	require.EqualError(t, err, "configs is blank")
	rows, err := MetricViewGets(c, 7)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestMetricViewDel_EnforcesOptionalCreatorScope(t *testing.T) {
	c := pipelineContext(t, &MetricView{})
	require.NoError(t, c.DB.Create(&[]MetricView{{Id: 1, CreateBy: 7}, {Id: 2, CreateBy: 8}}).Error)
	require.NoError(t, MetricViewDel(c, nil))

	require.NoError(t, MetricViewDel(c, []int64{1, 2}, 7))
	missing, err := MetricViewGet(c, "id = ?", 1)

	require.NoError(t, err)
	require.Nil(t, missing)
	other, err := MetricViewGet(c, "id = ?", 2)
	require.NoError(t, err)
	require.NotNil(t, other)
	require.NoError(t, MetricViewDel(c, []int64{2}))
	missing, err = MetricViewGet(c, "id = ?", 2)
	require.NoError(t, err)
	require.Nil(t, missing)
}
