package models

import (
	"encoding/json"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestHTTP_IsLokiRecognizesEndpointAndTenantHeaders(t *testing.T) {
	cases := []struct {
		name   string
		config HTTP
		want   bool
	}{
		{name: "endpoint", config: HTTP{Url: "http://127.0.0.1/loki/api/v1/query"}, want: true},
		{name: "header", config: HTTP{Headers: map[string]string{"X-Loki-Tenant": "ops"}}, want: true},
		{
			name:   "ordinary prometheus",
			config: HTTP{Url: "http://127.0.0.1/prom", Headers: map[string]string{"Accept": "json"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isLoki := tc.config.IsLoki()

			require.Equal(t, tc.want, isLoki)
		})
	}
}

func TestHTTP_ParseUrlPreservesComponents(t *testing.T) {
	config := HTTP{Url: "http://127.0.0.1:9090/prometheus?tenant=ops"}

	parsed, err := config.ParseUrl()

	require.NoError(t, err)
	require.Equal(t, "http", parsed.Scheme)
	require.Equal(t, "127.0.0.1:9090", parsed.Host)
	require.Equal(t, "/prometheus", parsed.Path)
	require.Equal(t, "ops", parsed.Query().Get("tenant"))
}

func TestHTTP_ParseUrlRejectsMalformedEscapes(t *testing.T) {
	config := HTTP{Urls: []string{"http://127.0.0.1/%zz"}}

	parsed, err := config.ParseUrl()

	require.Error(t, err)
	require.Nil(t, parsed)
}

func TestHostMeta_BinaryEncodingPreservesMetadata(t *testing.T) {
	original := HostMeta{
		Hostname: "node", CpuNum: 8, Offset: -300, EngineName: "edge-a",
		GlobalLabels: map[string]string{"env": "prod"}, ExtendInfo: map[string]interface{}{"rack": "a1"},
	}

	encoded, err := original.MarshalBinary()

	require.NoError(t, err)
	var decoded HostMeta
	require.NoError(t, decoded.UnmarshalBinary(encoded))
	require.Equal(t, original, decoded)
	require.Equal(t, "n9e_meta_node", WrapIdent(original.Hostname))
	require.Equal(t, "n9e_extend_meta_node", WrapExtendIdent(original.Hostname))
}

func TestHostMeta_UnmarshalBinaryRejectsInvalidJSON(t *testing.T) {
	var meta HostMeta

	err := meta.UnmarshalBinary([]byte("{"))

	require.Error(t, err)
}

func TestHostUpdateTime_UnmarshalBinaryPreservesHeartbeat(t *testing.T) {
	var heartbeat HostUpdateTime

	err := heartbeat.UnmarshalBinary([]byte(`{"ident":"node","update_time":100}`))

	require.NoError(t, err)
	require.Equal(t, HostUpdateTime{Ident: "node", UpdateTime: 100}, heartbeat)
}

func TestMatchDatasource_RequiresMembershipExceptForWildcardEvent(t *testing.T) {
	cases := []struct {
		name string
		ids  []int64
		id   int64
		want bool
	}{
		{name: "included", ids: []int64{7, 8}, id: 8, want: true},
		{name: "excluded", ids: []int64{7, 8}, id: 9},
		{name: "no selection", id: 7},
		{name: "wildcard event", id: DatasourceIdAll, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched := MatchDatasource(tc.ids, tc.id)

			require.Equal(t, tc.want, matched)
		})
	}
}

func TestLabelAndKeyHasKey_MatchesKeyRatherThanDisplayLabel(t *testing.T) {
	keys := []LabelAndKey{{Label: "Host", Key: "ident"}, {Label: "Environment", Key: "env"}}

	require.True(t, LabelAndKeyHasKey(keys, "env"))
	require.False(t, LabelAndKeyHasKey(keys, "Environment"))
	require.False(t, LabelAndKeyHasKey(nil, "env"))
}

func TestChildVarConfig_MarshalJSONUsesEmptyArrayForMissingParameters(t *testing.T) {
	cases := []struct {
		name   string
		config ChildVarConfig
		want   string
	}{
		{name: "missing", want: `{"param_val":[],"child_var_configs":null}`},
		{
			name: "configured",
			config: ChildVarConfig{ParamVal: []map[string]ParamQuery{{
				"ident": {ParamType: "enum", Query: []string{"node"}},
			}}},
			want: `{"param_val":[{"ident":{"param_type":"enum","query":["node"]}}],"child_var_configs":null}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.config)

			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(encoded))
		})
	}
}

func TestDataResp_LastRejectsMalformedFinalPoint(t *testing.T) {
	cases := []struct {
		name   string
		values [][]float64
	}{
		{name: "empty"},
		{name: "timestamp only", values: [][]float64{{100, 2}, {200}}},
		{name: "extra value", values: [][]float64{{100, 2}, {200, 3, 4}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			series := DataResp{Metric: model.Metric{"__name__": "cpu"}, Values: tc.values}

			timestamp, value, ok := series.Last()

			require.False(t, ok)
			require.Zero(t, timestamp)
			require.Zero(t, value)
		})
	}
}

func TestAssistantContentType_IsStructuredPayloadIdentifiesJSONCards(t *testing.T) {
	cases := []struct {
		contentType AssistantContentType
		want        bool
	}{
		{contentType: ContentTypeAlertRule, want: true},
		{contentType: ContentTypeDashboard, want: true},
		{contentType: ContentTypeFormSelect, want: true},
		{contentType: "markdown"},
		{contentType: "hint"},
		{contentType: "unknown"},
	}
	for _, tc := range cases {
		t.Run(string(tc.contentType), func(t *testing.T) {
			structured := tc.contentType.IsStructuredPayload()

			require.Equal(t, tc.want, structured)
		})
	}
}

func TestDataResp_LastReturnsFinalSample(t *testing.T) {
	series := DataResp{Values: [][]float64{{100, 2.5}, {200, 3.75}}}

	timestamp, value, ok := series.Last()

	require.True(t, ok)
	require.Equal(t, 200.0, timestamp)
	require.Equal(t, 3.75, value)
}
