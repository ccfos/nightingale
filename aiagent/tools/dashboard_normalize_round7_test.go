package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizePanel_MovesGrafanaGridPosition(t *testing.T) {
	panel := map[string]interface{}{
		"id": "cpu", "name": "CPU", "type": "timeseries",
		"gridPos": map[string]interface{}{"x": float64(1), "y": float64(2), "w": float64(10), "h": float64(6)},
	}

	normalizePanel(panel, 0, 7)

	require.Equal(t, map[string]interface{}{
		"i": "cpu", "x": float64(1), "y": float64(2), "w": float64(10), "h": float64(6),
	}, panel["layout"])
	require.NotContains(t, panel, "gridPos")
}

func TestNormalizePanel_AssignsDefaultNameAndGridSlot(t *testing.T) {
	panel := map[string]interface{}{"type": "stat"}

	normalizePanel(panel, 3, 0)

	require.Equal(t, "Panel 4", panel["name"])
	require.Equal(t, "panel-3", panel["id"])
	require.Equal(t, map[string]interface{}{
		"i": "panel-3", "x": float64(12), "y": float64(8), "w": float64(12), "h": float64(8),
	}, panel["layout"])
	require.Equal(t, float64(1), panel["datasourceValue"])
	require.Equal(t, "valueAndName", panel["custom"].(map[string]interface{})["textMode"])
}

func TestNormalizePanel_PreservesCustomSettingsAndRepairsLayoutID(t *testing.T) {
	panel := map[string]interface{}{
		"id": "cpu", "name": "CPU", "version": n9eVersion, "type": "timeseries",
		"layout":         map[string]interface{}{"i": "stale", "x": float64(4)},
		"custom":         map[string]interface{}{"lineWidth": float64(5)},
		"datasourceCate": "prometheus", "datasourceValue": "${prom}",
	}

	normalizePanel(panel, 0, 7)

	require.Equal(t, map[string]interface{}{"i": "cpu", "x": float64(4)}, panel["layout"])
	require.Equal(t, map[string]interface{}{"version": n9eVersion, "lineWidth": float64(5)}, panel["custom"])
	require.Equal(t, "${prom}", panel["datasourceValue"])
}

func TestNormalizePanel_FillsRequiredFieldsForUnknownTypes(t *testing.T) {
	panel := map[string]interface{}{"type": "text"}

	normalizePanel(panel, 0, 7)

	require.Equal(t, map[string]interface{}{"version": n9eVersion}, panel["custom"])
	require.Equal(t, map[string]interface{}{}, panel["options"])
	require.Equal(t, []interface{}{}, panel["overrides"])
	require.Equal(t, []interface{}{}, panel["transformationsNG"])
}

func TestNormalizeVariable_RepairsInvalidDatasourceAndPreservesExplicitOptions(t *testing.T) {
	cases := []struct {
		name       string
		datasource interface{}
		want       interface{}
	}{
		{name: "string", datasource: "prom", want: map[string]interface{}{"cate": "prometheus", "value": int64(7)}},
		{name: "scalar", datasource: true, want: map[string]interface{}{"cate": "prometheus", "value": int64(7)}},
		{
			name: "object missing category", datasource: map[string]interface{}{"value": "${missing}"},
			want: map[string]interface{}{"cate": "prometheus", "value": int64(7)},
		},
		{
			name: "valid object", datasource: map[string]interface{}{"cate": "elasticsearch", "value": int64(9)},
			want: map[string]interface{}{"cate": "elasticsearch", "value": int64(9)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			variable := map[string]interface{}{
				"datasource": tc.datasource, "allOption": false, "hide": true,
				"defaultValue": "web-1", "options": []string{"stale"},
			}

			normalizeVariable(variable, 7)

			require.Equal(t, tc.want, variable["datasource"])
			require.Equal(t, false, variable["allOption"])
			require.Equal(t, true, variable["hide"])
			require.Equal(t, "web-1", variable["defaultValue"])
			require.NotContains(t, variable, "options")
		})
	}
}

func TestNormalizeConfigs_SkipsNonObjectEntries(t *testing.T) {
	input := `{"var":[null,"ignored",{"name":"host"}],"panels":[null,12,{"type":"text"}]}`

	got, err := normalizeConfigs(input, 7)

	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(got), &result))
	variables := result["var"].([]interface{})
	require.Nil(t, variables[0])
	require.Equal(t, "ignored", variables[1])
	require.Equal(t, "host", variables[2].(map[string]interface{})["name"])
	panels := result["panels"].([]interface{})
	require.Nil(t, panels[0])
	require.Equal(t, float64(12), panels[1])
	require.Equal(t, "Panel 3", panels[2].(map[string]interface{})["name"])
}

func TestNormalizeConfigs_RejectsMalformedJSON(t *testing.T) {
	got, err := normalizeConfigs(`{"panels":`, 7)

	require.Empty(t, got)
	require.ErrorContains(t, err, "invalid configs JSON")
}
