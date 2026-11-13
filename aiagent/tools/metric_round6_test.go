package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type metricAPIRound6 struct {
	*fakePromAPI
	label    string
	matchers []string
	start    time.Time
	end      time.Time
	series   []model.LabelSet
	err      error
}

func (f *metricAPIRound6) LabelValues(_ context.Context, label string, matchers []string) (model.LabelValues, prom.Warnings, error) {
	f.label, f.matchers = label, matchers
	return f.labelVals, nil, f.err
}

func (f *metricAPIRound6) Series(_ context.Context, matchers []string, start, end time.Time) ([]model.LabelSet, prom.Warnings, error) {
	f.matchers, f.start, f.end = matchers, start, end
	return f.series, nil, f.err
}

func TestListMetrics_FiltersCaseInsensitivelyBeforeApplyingLimit(t *testing.T) {
	fake := &metricAPIRound6{fakePromAPI: &fakePromAPI{
		labelVals: model.LabelValues{"up", "CPU_usage", "cpu_idle", "cpu_load"},
	}}
	deps := &aiagent.ToolDeps{GetPromClient: func(id int64) prom.API {
		require.Equal(t, int64(7), id)
		return fake
	}}

	out, err := listMetrics(context.Background(), deps, map[string]interface{}{
		"datasource_id": "7", "keyword": "cPu", "limit": float64(2),
	}, map[string]string{"datasource_id": "3"})

	require.NoError(t, err)
	require.JSONEq(t, `["CPU_usage","cpu_idle"]`, out)
	require.Equal(t, "__name__", fake.label)
	require.Nil(t, fake.matchers)
}

func TestListMetrics_ReturnsEmptyJSONArrayWhenNoNamesMatch(t *testing.T) {
	fake := &fakePromAPI{labelVals: model.LabelValues{"up"}}
	deps := &aiagent.ToolDeps{GetPromClient: func(id int64) prom.API {
		require.Equal(t, int64(7), id)
		return fake
	}}

	out, err := listMetrics(context.Background(), deps, map[string]interface{}{"keyword": "missing"},
		map[string]string{"datasource_id": "7"})

	require.NoError(t, err)
	require.JSONEq(t, `[]`, out)
}

func TestGetMetricLabels_DeduplicatesValuesAndOmitsMetricName(t *testing.T) {
	fake := &metricAPIRound6{fakePromAPI: &fakePromAPI{}, series: []model.LabelSet{
		{"__name__": "up", "job": "node", "instance": "web-1"},
		{"__name__": "up", "job": "node", "instance": "web-2"},
		{"__name__": "up", "job": "agent", "instance": "web-1"},
	}}
	deps := &aiagent.ToolDeps{GetPromClient: func(id int64) prom.API {
		require.Equal(t, int64(7), id)
		return fake
	}}

	out, err := getMetricLabels(context.Background(), deps, map[string]interface{}{"metric": "up"},
		map[string]string{"datasource_id": "7"})

	require.NoError(t, err)
	var labels map[string][]string
	require.NoError(t, json.Unmarshal([]byte(out), &labels))
	require.Equal(t, map[string][]string{"job": {"node", "agent"}, "instance": {"web-1", "web-2"}}, labels)
	require.Equal(t, []string{"up"}, fake.matchers)
	require.Equal(t, time.Hour, fake.end.Sub(fake.start))
}

func TestMetricDiscovery_PropagatesDatasourceFailure(t *testing.T) {
	cases := []struct {
		name    string
		handler aiagent.BuiltinToolFunc
		want    string
	}{
		{name: "names", handler: listMetrics, want: "failed to get metrics: unavailable"},
		{name: "labels", handler: getMetricLabels, want: "failed to get metric series: unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &metricAPIRound6{fakePromAPI: &fakePromAPI{}, err: errors.New("unavailable")}
			deps := &aiagent.ToolDeps{GetPromClient: func(int64) prom.API { return fake }}

			out, err := tc.handler(context.Background(), deps,
				map[string]interface{}{"datasource_id": "7", "metric": "up"}, nil)

			require.Empty(t, out)
			require.EqualError(t, err, tc.want)
		})
	}
}
