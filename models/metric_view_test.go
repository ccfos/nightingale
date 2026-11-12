package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricViewUpdate_RejectsBlankReplacementFields(t *testing.T) {
	c := pipelineContext(t, &MetricView{})
	view := &MetricView{Name: "cpu", Configs: `{"a":1}`, Cate: 1, CreateBy: 7}
	require.NoError(t, view.Add(c))

	err := view.Update(c, " ", `{"a":1}`, 1, 7)

	require.EqualError(t, err, "name is blank")
	var stored MetricView
	require.NoError(t, c.DB.First(&stored, view.Id).Error)
	require.Equal(t, "cpu", stored.Name)
}
