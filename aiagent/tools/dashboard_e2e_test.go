package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

// TestDashboardToolsE2E_FrontendConfigAlignment exercises the public AI-tool
// flow end-to-end: create_dashboard persists the FE-compatible defaults for
// every supported chart type, update_dashboard changes a timeseries to tableNG
// through its approval flow, and get_dashboard_detail reads the persisted
// shape back.
func TestDashboardToolsE2E_FrontendConfigAlignment(t *testing.T) {
	deps := newDashboardTestDeps(t)
	params := map[string]string{"user_id": "1", "chat_id": "dashboard-e2e", "seq_id": "1"}

	created, err := createDashboard(context.Background(), deps, map[string]interface{}{
		"group_id":      float64(1),
		"name":          "AI dashboard tableNG e2e",
		"datasource_id": float64(1),
		"panels": `[
			{"name":"Legacy table alias","type":"table","queries":[{"promql":"up","instant":false}]},
			{"name":"CPU trend","type":"timeseries","queries":[{"promql":"rate(cpu_usage_active[5m])","instant":false}]},
			{"name":"Summary stat","type":"stat","queries":[{"promql":"avg(cpu_usage_active)"}]},
			{"name":"Usage gauge","type":"gauge","queries":[{"promql":"avg(cpu_usage_active)"}],"unit":"percent"},
			{"name":"Top hosts","type":"barGauge","queries":[{"promql":"sum by (ident) (cpu_usage_active)"}]},
			{"name":"Usage distribution","type":"pie","queries":[{"promql":"sum by (ident) (cpu_usage_active)"}]},
			{"name":"Runbook","type":"text","description":"# CPU runbook"}
		]`,
	}, params)
	require.NoError(t, err)

	var createResp struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(created), &createResp))
	require.NotZero(t, createResp.ID)

	payload, err := models.BoardPayloadGet(deps.DBCtx, createResp.ID)
	require.NoError(t, err)
	var createdConfigs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payload), &createdConfigs))
	require.Equal(t, n9eVersion, createdConfigs["version"])
	datasourceVar := createdConfigs["var"].([]interface{})[0].(map[string]interface{})
	require.Equal(t, float64(1), datasourceVar["defaultValue"])
	for _, raw := range createdConfigs["panels"].([]interface{}) {
		panel := raw.(map[string]interface{})
		options := panel["options"].(map[string]interface{})
		thresholds := options["thresholds"].(map[string]interface{})
		steps := thresholds["steps"].([]interface{})
		require.NotEmpty(t, steps, "%s must have threshold defaults", panel["name"])
		require.Equal(t, "base", steps[0].(map[string]interface{})["type"], "%s must have a base threshold", panel["name"])
		require.Equal(t, "dashed", options["thresholdsStyle"].(map[string]interface{})["mode"])
	}

	legacyTable := findPanel(createdConfigs["panels"].([]interface{}), "", "Legacy table alias")
	require.NotNil(t, legacyTable)
	require.Equal(t, "tableNG", legacyTable["type"], "the legacy input alias must create tableNG")
	require.Equal(t, true, legacyTable["targets"].([]interface{})[0].(map[string]interface{})["instant"])
	require.NotContains(t, legacyTable, "transformationsNG")
	require.NotContains(t, legacyTable["layout"].(map[string]interface{}), "isResizable")
	require.Equal(t, map[string]interface{}{"type": "none"}, legacyTable["custom"].(map[string]interface{})["cellOptions"])

	trend := findPanel(createdConfigs["panels"].([]interface{}), "", "CPU trend")
	require.NotNil(t, trend)
	trendCustom := trend["custom"].(map[string]interface{})
	require.Equal(t, float64(0.01), trendCustom["fillOpacity"])
	require.Equal(t, float64(5), trendCustom["pointSize"])
	require.Equal(t, float64(0), trendCustom["barAlignment"])
	require.Equal(t, float64(0.6), trendCustom["barWidthFactor"])
	require.Empty(t, trend["description"])

	stat := findPanel(createdConfigs["panels"].([]interface{}), "", "Summary stat")
	require.NotNil(t, stat)
	statCustom := stat["custom"].(map[string]interface{})
	require.Equal(t, float64(0), statCustom["colSpan"])
	require.Equal(t, "Value", statCustom["valueField"])
	require.Equal(t, "auto", statCustom["orientation"])

	gauge := findPanel(createdConfigs["panels"].([]interface{}), "", "Usage gauge")
	require.NotNil(t, gauge)
	gaugeOptions := gauge["options"].(map[string]interface{})
	gaugeStandard := gaugeOptions["standardOptions"].(map[string]interface{})
	require.Equal(t, float64(0), gaugeStandard["min"])
	require.Equal(t, float64(100), gaugeStandard["max"])
	require.Len(t, gaugeOptions["thresholds"].(map[string]interface{})["steps"], 3)
	require.NotContains(t, gauge["custom"].(map[string]interface{}), "min")

	barGauge := findPanel(createdConfigs["panels"].([]interface{}), "", "Top hosts")
	require.NotNil(t, barGauge)
	barGaugeCustom := barGauge["custom"].(map[string]interface{})
	require.Equal(t, "Value", barGaugeCustom["valueField"])
	require.Equal(t, "desc", barGaugeCustom["sortOrder"])
	require.Equal(t, "none", barGaugeCustom["otherPosition"])
	require.Equal(t, "color", barGaugeCustom["valueMode"])
	require.NotContains(t, barGaugeCustom, "orientation")
	require.NotContains(t, barGaugeCustom, "baseColor")

	pie := findPanel(createdConfigs["panels"].([]interface{}), "", "Usage distribution")
	require.NotNil(t, pie)
	pieCustom := pie["custom"].(map[string]interface{})
	require.Equal(t, "Value", pieCustom["valueField"])
	require.Equal(t, "", pieCustom["detailName"])
	require.Equal(t, "right", pieCustom["legengPosition"])
	require.Equal(t, "valueAndName", pieCustom["textMode"])
	require.Equal(t, "value", pieCustom["colorMode"])
	require.Equal(t, map[string]interface{}{}, pieCustom["textSize"])

	text := findPanel(createdConfigs["panels"].([]interface{}), "", "Runbook")
	require.NotNil(t, text)
	textCustom := text["custom"].(map[string]interface{})
	require.Equal(t, "# CPU runbook", textCustom["content"])
	require.Equal(t, float64(12), textCustom["textSize"])
	require.Equal(t, "#000000", textCustom["textColor"])
	require.Equal(t, "#FFFFFF", textCustom["textDarkColor"])
	require.Equal(t, "rgba(0, 0, 0, 0)", textCustom["bgColor"])
	require.Equal(t, "center", textCustom["justifyContent"])
	require.Equal(t, "center", textCustom["alignItems"])
	require.NotContains(t, text, "description")

	trendID := trend["id"].(string)

	// A request may carry instant:false while changing to tableNG; the resulting
	// stored target must still be instant:true after the approval confirmation.
	ti, proposalID := proposeInterrupt(t, deps, map[string]interface{}{
		"id":     float64(createResp.ID),
		"panels": `[{"id":"` + trendID + `","type":"tableNG","queries":[{"ref":"A","promql":"rate(cpu_usage_active[5m])","instant":false}]}]`,
	}, params)
	require.Contains(t, ti.Prompt, "tableNG")

	confirmed, err := updateDashboard(context.Background(), deps, map[string]interface{}{
		"id":          float64(createResp.ID),
		"proposal_id": proposalID,
		"confirmed":   true,
	}, map[string]string{"user_id": "1", "chat_id": "dashboard-e2e", "seq_id": "2"})
	require.NoError(t, err)
	require.Contains(t, confirmed, `"applied":true`)

	payload, err = models.BoardPayloadGet(deps.DBCtx, createResp.ID)
	require.NoError(t, err)
	var updatedConfigs map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payload), &updatedConfigs))
	updatedTrend := findPanel(updatedConfigs["panels"].([]interface{}), trendID, "")
	require.Equal(t, "tableNG", updatedTrend["type"])
	require.Equal(t, true, updatedTrend["targets"].([]interface{})[0].(map[string]interface{})["instant"])
	require.Equal(t, false, updatedTrend["custom"].(map[string]interface{})["filterable"])
	require.Equal(t, "none", updatedTrend["custom"].(map[string]interface{})["cellOptions"].(map[string]interface{})["type"])
	require.NotContains(t, updatedTrend["options"].(map[string]interface{}), "legend")

	// The detail tool is the read API exposed to the agent. It must surface the
	// persisted tableNG type and its forced instant setting after the full flow.
	detail, err := getDashboardDetail(context.Background(), deps,
		map[string]interface{}{"id": float64(createResp.ID), "include_config": true},
		map[string]string{"user_id": "1"})
	require.NoError(t, err)
	var detailResp struct {
		Panels []panelSummary `json:"panels"`
	}
	require.NoError(t, json.Unmarshal([]byte(detail), &detailResp))
	for _, panel := range detailResp.Panels {
		if panel.Id != trendID {
			continue
		}
		require.Equal(t, "tableNG", panel.Type)
		require.Len(t, panel.Queries, 1)
		require.NotNil(t, panel.Queries[0].Instant)
		require.True(t, *panel.Queries[0].Instant)
		return
	}
	t.Fatalf("updated panel %q missing from get_dashboard_detail response: %#v", trendID, detailResp.Panels)
}
