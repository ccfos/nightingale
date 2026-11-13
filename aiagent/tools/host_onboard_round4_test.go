package tools

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestProbeTargetOnboardStatus_ReportsMissingHostWithoutDatasource(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{})
	deps.Redis = hostHealthRedis(t)

	out, err := probeTargetOnboardStatus(context.Background(), deps,
		map[string]interface{}{"ident": "new-host"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var got onboardProbeResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Equal(t, "new-host", got.Ident)
	require.False(t, got.InTargetDb)
	require.False(t, got.InRedisBeat)
	require.False(t, got.DatasourceChecked)
	require.Equal(t, segmentCategrafOrHTTP, got.LikelySegment)
	require.NotEmpty(t, got.LikelyCauses)
}

func TestProbeTargetOnboardStatus_ReportsDBHostWithoutHeartbeat(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{}, &models.TargetBusiGroup{})
	deps.Redis = hostHealthRedis(t)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Target{
		Id: 10, Ident: "web-1", OS: "linux", AgentVersion: "v1", UpdateAt: 1000,
	}).Error)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TargetBusiGroup{TargetIdent: "web-1", GroupId: 1}).Error)

	out, err := probeTargetOnboardStatus(context.Background(), deps,
		map[string]interface{}{"ident": "web-1"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var got onboardProbeResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.True(t, got.InTargetDb)
	require.Equal(t, &onboardTargetSnap{
		Id: 10, OS: "linux", AgentVersion: "v1", UpdateAt: 1000, UpdateAtStr: formatUnixTime(1000),
		GroupIds: []int64{1}, HasGroup: true,
	}, got.Target)
	require.False(t, got.InRedisBeat)
	require.Equal(t, segmentTargetDB, got.LikelySegment)
}

func TestProbeTargetOnboardStatus_ChecksPrometheusWithEscapedIdent(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{})
	fake := &instantRound3API{fakePromAPI: &fakePromAPI{}, value: model.Vector{&model.Sample{Value: 1}}}
	deps.GetPromClient = func(id int64) prom.API {
		require.Equal(t, int64(7), id)
		return fake
	}

	out, err := probeTargetOnboardStatus(context.Background(), deps,
		map[string]interface{}{"ident": `web"1`, "datasource_id": "7"}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var got onboardProbeResult
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.True(t, got.DatasourceChecked)
	require.Equal(t, int64(7), got.DatasourceId)
	require.True(t, got.InPromTargetUp)
	require.Equal(t, 2, got.PromMetricsHit)
	require.NotNil(t, got.TargetUpLast)
	require.Equal(t, float64(1), *got.TargetUpLast)
	require.Equal(t, `system_load1{ident="web\"1"}`, fake.query)
}

func TestProbeTargetOnboardStatus_RejectsInaccessibleGroupedHost(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{}, &models.TargetBusiGroup{})
	resourceMembership(t, deps, "ro")
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.Target{Ident: "private"}).Error)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TargetBusiGroup{TargetIdent: "private", GroupId: 2}).Error)

	out, err := probeTargetOnboardStatus(context.Background(), deps,
		map[string]interface{}{"ident": "private"}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "forbidden: no access to this target")
}

func TestPromInstantFirstSample_UsesFirstNonNaNValue(t *testing.T) {
	fake := &instantRound3API{fakePromAPI: &fakePromAPI{}, value: model.Vector{
		&model.Sample{Value: model.SampleValue(math.NaN())}, &model.Sample{Value: 0}, &model.Sample{Value: 3},
	}}

	got, ok := promInstantFirstSample(context.Background(), fake, "target_up", time.Unix(1000, 0))

	require.True(t, ok, "a zero-valued target_up is still a real sample")
	require.Zero(t, got)
	require.Equal(t, "target_up", fake.query)
	require.Equal(t, time.Unix(1000, 0), fake.timestamp)
}

func TestPromInstantFirstSample_TreatsUnavailableSamplesAsAbsent(t *testing.T) {
	cases := []struct {
		name  string
		value model.Value
		err   error
	}{
		{name: "nil"},
		{name: "empty vector", value: model.Vector{}},
		{name: "scalar", value: &model.Scalar{Value: 1}},
		{name: "query failure", err: errors.New("unavailable")},
		{name: "only NaN", value: model.Vector{&model.Sample{Value: model.SampleValue(math.NaN())}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &instantRound3API{fakePromAPI: &fakePromAPI{}, value: tc.value, err: tc.err}

			got, ok := promInstantFirstSample(context.Background(), fake, "target_up", time.Unix(1000, 0))

			require.False(t, ok)
			require.Zero(t, got)
		})
	}
}

func TestDiagnoseOnboardSegment_IdentifiesDeepestEvidence(t *testing.T) {
	cases := []struct {
		name   string
		result onboardProbeResult
		want   string
		cause  string
	}{
		{
			name: "fully connected", result: onboardProbeResult{InTargetDb: true, InRedisBeat: true, PromMetricsHit: 1},
			want: segmentOK,
		},
		{
			name: "prom missing", result: onboardProbeResult{InRedisBeat: true, DatasourceChecked: true},
			want: segmentPromIngest, cause: "writers",
		},
		{
			name: "DB only", result: onboardProbeResult{InTargetDb: true},
			want: segmentTargetDB, cause: "redis",
		},
		{
			name: "missing metadata",
			result: onboardProbeResult{
				InTargetDb: true, InRedisBeat: true, Target: &onboardTargetSnap{OS: "unknown"},
			},
			want: segmentServerRecv, cause: "heartbeat",
		},
		{
			name: "nothing seen", result: onboardProbeResult{},
			want: segmentCategrafOrHTTP, cause: "categraf",
		},
		{
			name: "ungrouped",
			result: onboardProbeResult{
				InTargetDb: true, InRedisBeat: true, Target: &onboardTargetSnap{OS: "linux", AgentVersion: "v1"},
			},
			want: segmentOK, cause: "未归入任何业务组",
		},
		{
			name: "datasource not checked", result: onboardProbeResult{InRedisBeat: true},
			want: segmentOK,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			segment, causes := diagnoseOnboardSegment(&tc.result)

			require.Equal(t, tc.want, segment)
			if tc.cause == "" {
				require.Empty(t, causes)
			} else {
				require.NotEmpty(t, causes)
				require.Contains(t, causes[0], tc.cause)
			}
		})
	}
}
