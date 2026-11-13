package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestAppendLabels_AddsEnabledBusinessGroupLabel(t *testing.T) {
	rt := &Router{}
	rt.Pushgw.BusiGroupLabelKey = "busigroup"
	groups := &memsto.BusiGroupCacheType{}
	groups.Set(map[int64]*models.BusiGroup{7: {Id: 7, LabelEnable: 1, LabelValue: "operations"}}, 1, 10)
	series := &prompb.TimeSeries{Labels: []prompb.Label{{Name: "__name__", Value: "cpu_usage"}}}

	rt.AppendLabels(series, &models.Target{GroupId: 7}, groups)

	require.Equal(t, map[string]string{"__name__": "cpu_usage", "busigroup": "operations"}, labelValues(series.Labels))
}

func TestAppendLabels_SkipsDisabledOrMissingBusinessGroup(t *testing.T) {
	for _, group := range []*models.BusiGroup{nil, {Id: 7, LabelValue: "operations"}} {
		rt := &Router{}
		rt.Pushgw.BusiGroupLabelKey = "busigroup"
		groups := &memsto.BusiGroupCacheType{}
		if group != nil {
			groups.Set(map[int64]*models.BusiGroup{7: group}, 1, 10)
		}
		series := &prompb.TimeSeries{Labels: []prompb.Label{{Name: "__name__", Value: "cpu_usage"}}}

		rt.AppendLabels(series, &models.Target{GroupId: 7}, groups)

		require.Equal(t, []prompb.Label{{Name: "__name__", Value: "cpu_usage"}}, series.Labels)
	}
}
