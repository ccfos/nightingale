package prom

import (
	"context"
	"io"
	"math"
	"net/http"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestSamplePairJSON_FormatsMillisecondsAndSpecialValues(t *testing.T) {
	tests := []struct {
		name  string
		point model.SamplePair
		want  string
	}{
		{name: "whole second", point: model.SamplePair{Timestamp: 1000, Value: 2}, want: `[1,"2"]`},
		{name: "one millisecond", point: model.SamplePair{Timestamp: 1001, Value: 2.5}, want: `[1.001,"2.5"]`},
		{name: "ten milliseconds", point: model.SamplePair{Timestamp: 1010, Value: 2}, want: `[1.010,"2"]`},
		{name: "hundred milliseconds", point: model.SamplePair{Timestamp: 1123, Value: 2}, want: `[1.123,"2"]`},
		{name: "small value", point: model.SamplePair{Timestamp: 0, Value: 1e-8}, want: `[0,"1e-08"]`},
		{name: "large value", point: model.SamplePair{Timestamp: 0, Value: 1e22}, want: `[0,"1e+22"]`},
		{name: "infinity", point: model.SamplePair{Timestamp: 0, Value: model.SampleValue(math.Inf(1))}, want: `[0,"+Inf"]`},
		{name: "NaN", point: model.SamplePair{Timestamp: 0, Value: model.SampleValue(math.NaN())}, want: `[0,"NaN"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := jsoniter.Marshal(tt.point)

			require.NoError(t, err)
			require.Equal(t, tt.want, string(encoded))
			var decoded model.SamplePair
			require.NoError(t, jsoniter.Unmarshal(encoded, &decoded))
			require.Equal(t, tt.point.Timestamp, decoded.Timestamp)
			if math.IsNaN(float64(tt.point.Value)) {
				require.True(t, math.IsNaN(float64(decoded.Value)))
			} else {
				require.Equal(t, tt.point.Value, decoded.Value)
			}
		})
	}
}

func TestSamplePairJSON_EncodesNegativeTimestamp(t *testing.T) {
	point := model.SamplePair{Timestamp: -1001, Value: -2}

	encoded, err := jsoniter.Marshal(point)

	require.NoError(t, err)
	require.Equal(t, `[-1.001,"-2"]`, string(encoded))
}

func TestSamplePairJSON_RejectsInvalidShapeTimestampAndValue(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty array", body: `[]`},
		{name: "invalid timestamp", body: `["bad","1"]`},
		{name: "missing value", body: `[1]`},
		{name: "invalid value", body: `[1,"bad"]`},
		{name: "extra value", body: `[1,"2","3"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var point model.SamplePair

			err := jsoniter.Unmarshal([]byte(tt.body), &point)

			require.Error(t, err)
		})
	}
}

func TestLabelsWithoutMetric_FormatsSortedEscapedLabels(t *testing.T) {
	labels := model.Metric{"__name__": "up", "zone": "b", "job": "a\"b"}

	got := LabelsWithoutMetric(labels)

	require.Equal(t, `{job="a\"b", zone="b"}`, got)
	require.Equal(t, "up", string(labels[model.MetricNameLabel]))
}

func TestLabelsWithoutMetric_FormatsEmptyLabelSet(t *testing.T) {
	for _, labels := range []model.Metric{nil, {model.MetricNameLabel: "up"}} {
		got := LabelsWithoutMetric(labels)

		require.Equal(t, "{}", got)
	}
}

func TestConvertPromQL_ReplacesSelectorLabelsWithMetricLabels(t *testing.T) {
	metric := Metric{Labels: model.Metric{"__name__": "ignored", "job": "api", "zone": "a"}}

	got, err := ConvertPromQL(`sum(rate(requests_total{env="prod"}[5m]))`, metric)

	require.NoError(t, err)
	require.Equal(t, `sum(rate(requests_total{job="api", zone="a"}[5m]))`, got)
}

func TestConvertPromQL_RejectsInvalidExpression(t *testing.T) {
	got, err := ConvertPromQL("sum(", Metric{})

	require.Error(t, err)
	require.Empty(t, got)
}

func TestAlerts_DecodesAlertStateLabelsAndAnnotations(t *testing.T) {
	var method, path string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_, _ = io.WriteString(w, `{"status":"success","data":{"alerts":[{"labels":{"alertname":"Down"},`+
			`"annotations":{"summary":"Host down"},"state":"firing","value":"0"}]}}`)
	}, ClientOptions{})

	result, err := client.Alerts(context.Background())

	require.NoError(t, err)
	require.Equal(t, http.MethodGet, method)
	require.Equal(t, "/api/v1/alerts", path)
	require.Equal(t, AlertsResult{Alerts: []Alert{{
		Labels: model.LabelSet{"alertname": "Down"}, Annotations: model.LabelSet{"summary": "Host down"},
		State: AlertStateFiring, Value: "0",
	}}}, result)
}

func TestAlertManagers_DecodesActiveAndDroppedURLs(t *testing.T) {
	var method, path string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_, _ = io.WriteString(w, `{"status":"success","data":{"activeAlertManagers":[{"url":"http://active.test"}],`+
			`"droppedAlertManagers":[{"url":"http://dropped.test"}]}}`)
	}, ClientOptions{})

	result, err := client.AlertManagers(context.Background())

	require.NoError(t, err)
	require.Equal(t, http.MethodGet, method)
	require.Equal(t, "/api/v1/alertmanagers", path)
	require.Equal(t, AlertManagersResult{
		Active: []AlertManager{{URL: "http://active.test"}}, Dropped: []AlertManager{{URL: "http://dropped.test"}},
	}, result)
}

func TestFlags_DecodesFlagValues(t *testing.T) {
	var path string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, `{"status":"success","data":{"storage.tsdb.retention.time":"15d","web.enable-admin-api":"true"}}`)
	}, ClientOptions{})

	flags, err := client.Flags(context.Background())

	require.NoError(t, err)
	require.Equal(t, "/api/v1/status/flags", path)
	require.Equal(t, FlagsResult{"storage.tsdb.retention.time": "15d", "web.enable-admin-api": "true"}, flags)
}

func TestTargets_DecodesDiscoveredAndFinalLabels(t *testing.T) {
	var path string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, `{"status":"success","data":{"activeTargets":[{"discoveredLabels":{"__address__":"node:9100"},`+
			`"labels":{"job":"node"},"scrapeUrl":"http://node:9100/metrics","health":"up"}],`+
			`"droppedTargets":[{"discoveredLabels":{"__address__":"old:9100"}}]}}`)
	}, ClientOptions{})

	result, err := client.Targets(context.Background())

	require.NoError(t, err)
	require.Equal(t, "/api/v1/targets", path)
	require.Equal(t, TargetsResult{
		Active: []ActiveTarget{{
			DiscoveredLabels: map[string]string{"__address__": "node:9100"}, Labels: model.LabelSet{"job": "node"},
			ScrapeURL: "http://node:9100/metrics", Health: HealthGood,
		}},
		Dropped: []DroppedTarget{{DiscoveredLabels: map[string]string{"__address__": "old:9100"}}},
	}, result)
}

func TestCleanTombstones_PostsAdminRequest(t *testing.T) {
	var method, path string
	client := localPromAPI(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}, ClientOptions{})

	err := client.CleanTombstones(context.Background())

	require.NoError(t, err)
	require.Equal(t, http.MethodPost, method)
	require.Equal(t, "/api/v1/admin/tsdb/clean_tombstones", path)
}

func TestDiscoveryEndpoints_PropagateHTTPFailure(t *testing.T) {
	client := localPromAPI(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}, ClientOptions{})
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "alerts", run: func() error { _, err := client.Alerts(context.Background()); return err }},
		{name: "alert managers", run: func() error { _, err := client.AlertManagers(context.Background()); return err }},
		{name: "flags", run: func() error { _, err := client.Flags(context.Background()); return err }},
		{name: "targets", run: func() error { _, err := client.Targets(context.Background()); return err }},
		{name: "clean tombstones", run: func() error { return client.CleanTombstones(context.Background()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run()

			var apiErr *Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, ErrServer, apiErr.Type)
			require.Contains(t, apiErr.Detail, "unavailable")
		})
	}
}
