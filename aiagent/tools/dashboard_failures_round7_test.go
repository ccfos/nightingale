package tools

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type failingAnalysisAPI struct {
	*fakePromAPI
	calls  atomic.Int64
	failAt int64
}

func (f *failingAnalysisAPI) QueryRange(ctx context.Context, query string, r prom.Range) (model.Value, prom.Warnings, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if f.calls.Add(1) == f.failAt {
		return nil, nil, errors.New("fixture query failed")
	}
	return f.fakePromAPI.QueryRange(ctx, query, r)
}

const singleAnalysisPanel = `{"panels":[{
	"id":"cpu","name":"CPU","type":"timeseries","datasourceValue":1,
	"targets":[{"expr":"up"}]
}]}`

func TestGetDashboardData_ReportsUnavailableDatasource(t *testing.T) {
	deps := newAnalyzeDeps(t, &fakePromAPI{})
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, singleAnalysisPanel))
	deps.GetPromClient = func(int64) prom.API { return nil }

	got, err := getDashboardDataTool(context.Background(), deps, map[string]interface{}{"id": "51"},
		map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.Contains(t, got, `面板 "CPU" 查询失败: 数据源 1 不可用`)
}

func TestGetDashboardData_ReportsCurrentWindowQueryFailure(t *testing.T) {
	deps := newAnalyzeDeps(t, &fakePromAPI{})
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, singleAnalysisPanel))
	fake := &failingAnalysisAPI{fakePromAPI: &fakePromAPI{}, failAt: 1}
	deps.GetPromClient = func(int64) prom.API { return fake }

	got, err := getDashboardDataTool(context.Background(), deps, map[string]interface{}{"id": "51"},
		map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.Contains(t, got, `面板 "CPU" 查询失败: fixture query failed`)
	require.NotContains(t, got, "因分析总预算")
	require.Equal(t, int64(1), fake.calls.Load(), "a failed current query must not issue a comparison query")
}

func TestGetDashboardData_KeepsCurrentDataWhenComparisonFails(t *testing.T) {
	base := &fakePromAPI{rangeFn: func(_ string, r prom.Range) model.Matrix {
		return model.Matrix{mkStream(map[string]string{"host": "web-1"}, r.Start, 60, []float64{1, 1, 1, 1})}
	}}
	deps := newAnalyzeDeps(t, base)
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, singleAnalysisPanel))
	fake := &failingAnalysisAPI{fakePromAPI: base, failAt: 2}
	deps.GetPromClient = func(int64) prom.API { return fake }

	got, err := getDashboardDataTool(context.Background(), deps, map[string]interface{}{"id": "51"},
		map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.Contains(t, got, "1 个查询的同比窗口取数失败(同比检测跳过，其余照常)")
	require.Contains(t, got, "web-1")
	require.NotContains(t, got, `面板 "CPU" 查询失败`)
	require.Equal(t, int64(2), fake.calls.Load())
}

func TestGetDashboardData_LabelsCanceledAnalysisAsIncomplete(t *testing.T) {
	deps := newAnalyzeDeps(t, &fakePromAPI{})
	require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, singleAnalysisPanel))
	fake := &failingAnalysisAPI{fakePromAPI: &fakePromAPI{}}
	deps.GetPromClient = func(int64) prom.API { return fake }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := getDashboardDataTool(ctx, deps, map[string]interface{}{"id": "51"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.Contains(t, got, "1 个查询因分析总预算")
	require.Contains(t, got, "结果不完整")
	require.NotContains(t, got, `面板 "CPU" 查询失败`)
	require.Zero(t, fake.calls.Load())
}

func TestGetDashboardData_RejectsUnusablePayload(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "empty", payload: " ", want: "has no config payload"},
		{name: "malformed JSON", payload: "{", want: "invalid dashboard config payload"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newAnalyzeDeps(t, &fakePromAPI{})
			require.NoError(t, models.BoardPayloadSave(deps.DBCtx, 51, tc.payload))

			got, err := getDashboardDataTool(context.Background(), deps, map[string]interface{}{"id": "51"},
				map[string]string{"user_id": "1"})

			require.Empty(t, got)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestGetDashboardData_RequiresQueryCapability(t *testing.T) {
	deps := newAnalyzeDeps(t, &fakePromAPI{})
	deps.GetPromClient = nil

	got, err := getDashboardDataTool(context.Background(), deps, map[string]interface{}{"id": "51"},
		map[string]string{"user_id": "1"})

	require.Empty(t, got)
	require.EqualError(t, err, "prometheus query capability is not wired")
}

func TestFirstPromDatasourceID_UsesOnlyVisibleDatasources(t *testing.T) {
	cases := []struct {
		name   string
		hidden []int64
		want   int64
	}{
		{name: "prefer enabled", want: 2},
		{name: "fallback to disabled", hidden: []int64{2}, want: 3},
		{name: "none visible", hidden: []int64{2, 3}, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.Datasource{})
			require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.Datasource{
				{Id: 4, PluginType: "elasticsearch", Status: "enabled"},
				{Id: 3, PluginType: "prometheus", Status: "disabled"},
				{Id: 2, PluginType: "prometheus", Status: "enabled"},
			}).Error)
			user := &models.User{Id: 2, Username: "alice"}
			deps.FilterDatasources = func(items []*models.Datasource, gotUser *models.User) []*models.Datasource {
				require.Same(t, user, gotUser)
				var visible []*models.Datasource
				for _, item := range items {
					if !int64SliceContains(tc.hidden, item.Id) {
						visible = append(visible, item)
					}
				}
				return visible
			}

			got := firstPromDatasourceID(deps, user)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestGetDashboardData_DeniesOtherBusinessGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Board{})
	resourceMembership(t, deps, "ro", PermDashboards)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Board{Id: 51, GroupId: 2, Name: "private"}).Error)

	got, err := getDashboardDataTool(context.Background(), deps, map[string]interface{}{"id": "51"},
		map[string]string{"user_id": "2"})

	require.Empty(t, got)
	require.EqualError(t, err, "forbidden: no access to this dashboard")
}

func TestGetDashboardData_RejectsMissingDashboard(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Board{})

	got, err := getDashboardDataTool(context.Background(), deps, map[string]interface{}{"id": "51"},
		map[string]string{"user_id": "1"})

	require.Empty(t, got)
	require.EqualError(t, err, "dashboard not found: id=51")
}

func TestDashboardReaders_RequireID(t *testing.T) {
	deps := newResourceTestDeps(t)
	for _, handler := range []aiagent.BuiltinToolFunc{getDashboardDetail, getDashboardDataTool} {
		got, err := handler(context.Background(), deps, nil, map[string]string{"user_id": "1"})

		require.Empty(t, got)
		require.EqualError(t, err, "id is required")
	}
}
