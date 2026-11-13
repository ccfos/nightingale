package tools

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/prometheus/common/model"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func hostHealthRedis(t *testing.T) *redis.Client {
	t.Helper()
	srv, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(srv.Close)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	return client
}

func TestClassifyLag_UsesHeartbeatThresholds(t *testing.T) {
	cases := []struct {
		lag  int64
		want string
	}{
		{lag: 0, want: statusActive},
		{lag: 59, want: statusActive},
		{lag: 60, want: statusLagging},
		{lag: 179, want: statusLagging},
		{lag: 180, want: statusStale},
	}
	for _, tc := range cases {
		t.Run(strconv.FormatInt(tc.lag, 10), func(t *testing.T) {
			got := classifyLag(tc.lag)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestReadRedisHostState_DecodesHeartbeatAndMetadata(t *testing.T) {
	client := hostHealthRedis(t)
	beat := models.HostUpdateTime{Ident: "web-1", UpdateTime: 123456}
	meta := models.HostMeta{CpuNum: 8, CpuUtil: 12.5, MemUtil: 50, AgentVersion: "v1", Offset: -3}
	require.NoError(t, client.Set(context.Background(), models.WrapIdentUpdateTime("web-1"), beat, 0).Err())
	require.NoError(t, client.Set(context.Background(), models.WrapIdent("web-1"), meta, 0).Err())

	gotBeat, gotMeta := readRedisHostState(client, "web-1")

	require.Equal(t, beat.UpdateTime, gotBeat)
	require.Equal(t, &meta, gotMeta)
}

func TestReadRedisHostState_IgnoresMissingAndMalformedValues(t *testing.T) {
	client := hostHealthRedis(t)
	require.NoError(t, client.Set(context.Background(), models.WrapIdentUpdateTime("malformed"), "{", 0).Err())
	require.NoError(t, client.Set(context.Background(), models.WrapIdent("malformed"), "{", 0).Err())
	for _, ident := range []string{"missing", "malformed"} {
		t.Run(ident, func(t *testing.T) {
			beat, meta := readRedisHostState(client, ident)

			require.Zero(t, beat)
			require.Nil(t, meta)
		})
	}

	beat, meta := readRedisHostState(nil, "missing")

	require.Zero(t, beat)
	require.Nil(t, meta)
}

func TestGetTargetRealtimeStatus_CombinesDBIdentityAndLiveMetadata(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{}, &models.TargetBusiGroup{})
	client := hostHealthRedis(t)
	deps.Redis = client
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Target{
		Ident: "web-1", OS: "linux", HostIp: "10.0.0.1", AgentVersion: "old", UpdateAt: 1000,
	}).Error)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TargetBusiGroup{TargetIdent: "web-1", GroupId: 1}).Error)
	meta := models.HostMeta{
		CpuNum: 8, CpuUtil: 12.5, MemUtil: 50, Offset: -3, AgentVersion: "live",
		HostIp: "10.0.0.2", RemoteAddr: "127.0.0.1", ExtendInfo: map[string]interface{}{"zone": "east"},
	}
	require.NoError(t, client.Set(context.Background(), models.WrapIdent("web-1"), meta, 0).Err())

	out, err := getTargetRealtimeStatus(context.Background(), deps,
		map[string]interface{}{"ident": " web-1 "}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var got realtimeStatusResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Equal(t, realtimeStatusResult{
		Ident: "web-1", Status: statusStaleNoHeartbeat, UpdateAtDB: 1000, GroupIds: []int64{1},
		OS: "linux", HostIp: "10.0.0.1", AgentVersion: "live", RemoteAddr: "127.0.0.1",
		CpuNum: 8, CpuUtil: 12.5, MemUtil: 50, Offset: -3, ExtendInfo: map[string]interface{}{"zone": "east"},
	}, got)
}

func TestGetTargetRealtimeStatus_RejectsTargetOutsideVisibleGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{}, &models.TargetBusiGroup{})
	resourceMembership(t, deps, "ro")
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Target{Ident: "private"}).Error)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TargetBusiGroup{TargetIdent: "private", GroupId: 2}).Error)

	out, err := getTargetRealtimeStatus(context.Background(), deps,
		map[string]interface{}{"ident": "private"}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "forbidden: no access to this target")
}

func TestGetTargetRealtimeStatus_ReportsUnknownTarget(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{})

	out, err := getTargetRealtimeStatus(context.Background(), deps,
		map[string]interface{}{"ident": "missing"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.JSONEq(t, `{"error":"target not found"}`, out)
}

func TestListNeighborTargets_RestrictsNeighborsToSharedVisibleGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{}, &models.TargetBusiGroup{})
	deps.Redis = hostHealthRedis(t)
	resourceMembership(t, deps, "ro")
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&[]models.Target{
		{Ident: "self"}, {Ident: "visible", HostIp: "10.0.0.1"}, {Ident: "secret", HostIp: "10.0.0.2"},
	}).Error)
	require.NoError(t, db.Create(&[]models.TargetBusiGroup{
		{TargetIdent: "self", GroupId: 1}, {TargetIdent: "self", GroupId: 2},
		{TargetIdent: "visible", GroupId: 1}, {TargetIdent: "secret", GroupId: 2},
	}).Error)

	out, err := listNeighborTargets(context.Background(), deps,
		map[string]interface{}{"ident": "self"}, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var got struct {
		Items   []neighborItem  `json:"items"`
		Summary neighborSummary `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Equal(t, []neighborItem{{Ident: "visible", HostIp: "10.0.0.1", Status: statusStaleNoHeartbeat}}, got.Items)
	require.Equal(t, neighborSummary{Total: 1, Stale: 1}, got.Summary)
}

func TestListNeighborTargets_RejectsInaccessibleTarget(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TargetBusiGroup{})
	resourceMembership(t, deps, "ro")
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TargetBusiGroup{TargetIdent: "private", GroupId: 2}).Error)

	out, err := listNeighborTargets(context.Background(), deps,
		map[string]interface{}{"ident": "private"}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "forbidden: no access to this target")
}

func TestListNeighborTargets_UngroupedTargetHasEmptySummary(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TargetBusiGroup{})

	out, err := listNeighborTargets(context.Background(), deps,
		map[string]interface{}{"ident": "ungrouped"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.JSONEq(t, `{
		"items": [], "summary": {"total":0,"active":0,"lagging":0,"stale":0}, "note": "target has no busi group"
	}`, out)
}

func TestAggregateMetric_SummarizesAllSeriesAndSkipsNaN(t *testing.T) {
	start := time.Unix(1000, 0)
	fake := &fakePromAPI{rangeFn: func(query string, r prom.Range) model.Matrix {
		require.Equal(t, `cpu_usage_active{ident="web\\one\"two"}`, query)
		return model.Matrix{
			mkStream(nil, start.Add(10*time.Second), 10, []float64{math.NaN(), 4, 8}),
			mkStream(nil, start, 10, []float64{2, 6}),
		}
	}}

	got := aggregateMetric(context.Background(), fake, `web\one"two`, "cpu_usage_active", prom.Range{Start: start})

	require.Equal(t, &metricAggregate{
		Metric: "cpu_usage_active", SamplesCount: 4, Series: 2, FirstTs: 1000, LastTs: 1030,
		Min: 2, Max: 8, Avg: 5, Last: 8,
	}, got)
}

type hostRangeErrorAPI struct{ *fakePromAPI }

func (f *hostRangeErrorAPI) QueryRange(context.Context, string, prom.Range) (model.Value, prom.Warnings, error) {
	return nil, nil, errors.New("query failed")
}

func TestAggregateMetric_ReportsQueryFailure(t *testing.T) {
	fake := &hostRangeErrorAPI{fakePromAPI: &fakePromAPI{}}

	got := aggregateMetric(context.Background(), fake, "web-1", "cpu_usage_active", prom.Range{})

	require.Equal(t, &metricAggregate{Metric: "cpu_usage_active", Error: "query failed"}, got)
}

func TestAggregateMetric_EmptyOrNaNSeriesHaveNoSamples(t *testing.T) {
	cases := []struct {
		name   string
		matrix model.Matrix
		series int
	}{
		{name: "empty matrix", matrix: model.Matrix{}, series: 0},
		{
			name: "NaN values", matrix: model.Matrix{mkStream(nil, time.Unix(1000, 0), 10, []float64{math.NaN()})},
			series: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakePromAPI{rangeFn: func(string, prom.Range) model.Matrix { return tc.matrix }}

			got := aggregateMetric(context.Background(), fake, "web-1", "cpu_usage_active", prom.Range{})

			require.Equal(t, &metricAggregate{Metric: "cpu_usage_active", Series: tc.series}, got)
		})
	}
}

func TestQueryHostMetricsWindow_DeduplicatesMetricsAndUsesContextDatasource(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TargetBusiGroup{})
	fake := &fakePromAPI{rangeFn: func(string, prom.Range) model.Matrix {
		return model.Matrix{mkStream(nil, time.Unix(1000, 0), 10, []float64{2, 4})}
	}}
	deps.GetPromClient = func(id int64) prom.API {
		require.Equal(t, int64(7), id)
		return fake
	}

	out, err := queryHostMetricsWindow(context.Background(), deps, map[string]interface{}{
		"ident": "web-1", "metrics": "cpu_usage_active,system_load1 cpu_usage_active\n", "time_range": "10m",
	}, map[string]string{"user_id": "1", "datasource_id": "7"})

	require.NoError(t, err)
	var got struct {
		Ident        string             `json:"ident"`
		DatasourceID int64              `json:"datasource_id"`
		Items        []*metricAggregate `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Equal(t, "web-1", got.Ident)
	require.Equal(t, int64(7), got.DatasourceID)
	require.Len(t, got.Items, 2)
	require.Equal(t, "cpu_usage_active", got.Items[0].Metric)
	require.Equal(t, "system_load1", got.Items[1].Metric)
	for _, item := range got.Items {
		require.Equal(t, 2, item.SamplesCount)
		require.Equal(t, float64(3), item.Avg)
	}
	queries := fake.recorded()
	require.Len(t, queries, 2)
	var expressions []string
	for _, q := range queries {
		expressions = append(expressions, q.Query)
		require.Equal(t, 10*time.Minute, q.End.Sub(q.Start))
	}
	require.ElementsMatch(t, []string{`cpu_usage_active{ident="web-1"}`, `system_load1{ident="web-1"}`}, expressions)
}

func TestQueryHostMetricsWindow_RejectsTargetOutsideVisibleGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TargetBusiGroup{})
	resourceMembership(t, deps, "ro")
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TargetBusiGroup{TargetIdent: "private", GroupId: 2}).Error)

	out, err := queryHostMetricsWindow(context.Background(), deps,
		map[string]interface{}{"ident": "private"}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "forbidden: no access to this target")
}
