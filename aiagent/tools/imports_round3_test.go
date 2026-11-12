package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func dashboardImportFixture(t *testing.T, deps *aiagent.ToolDeps, payload string) {
	t.Helper()
	root := t.TempDir()
	deps.SkillsPath = filepath.Join(root, "skills")
	directory := filepath.Join(root, "integrations", "Test", "dashboards")
	require.NoError(t, os.MkdirAll(directory, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "overview.json"), []byte(payload), 0600))
}

func TestImportDashboardTemplate_PersistsReboundPayloadAndMetadata(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Board{}, &models.BoardPayload{}, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 3, Name: "metrics", PluginType: "prometheus"}).Error)
	dashboardImportFixture(t, deps, `{
  "name":"Template title","tags":"template-tag","configs":{
   "var":[{"name":"host","type":"query","definition":"label_values(up, instance)"}],
   "panels":[{"id":"cpu","name":"CPU","type":"timeseries","datasourceValue":99,"targets":[{"expr":"up"}]}]
  }
 }`)

	out, err := importDashboardTemplate(context.Background(), deps, map[string]interface{}{
		"group_id": "1", "component": "Test", "file": "overview.json", "name": "Imported CPU", "tags": "production",
	}, map[string]string{"user_id": "1", "datasource_id": "3"})

	require.NoError(t, err)
	var result struct {
		ID             int64  `json:"id"`
		GroupName      string `json:"group_name"`
		Panels         int    `json:"panels_count"`
		Variables      int    `json:"variables_count"`
		DatasourceName string `json:"datasource_name"`
		Source         string `json:"source"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Positive(t, result.ID)
	require.Equal(t, "Operations", result.GroupName)
	require.Equal(t, "metrics", result.DatasourceName)
	require.Equal(t, "Test/overview.json", result.Source)
	require.Equal(t, 1, result.Panels)
	require.Equal(t, 1, result.Variables)
	board, err := models.BoardGetByID(deps.DBCtx, result.ID)
	require.NoError(t, err)
	require.Equal(t, "Imported CPU", board.Name)
	require.Equal(t, "production", board.Tags)
	require.Equal(t, int64(1), board.GroupId)
	require.Equal(t, "root", board.CreateBy)
	payload, err := models.BoardPayloadGet(deps.DBCtx, result.ID)
	require.NoError(t, err)
	var config map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(payload), &config))
	require.Equal(t, "3.4.0", config["version"])
	variables := config["var"].([]interface{})
	require.Equal(t, "datasource", variables[0].(map[string]interface{})["type"])
	require.Equal(t, float64(3), variables[0].(map[string]interface{})["defaultValue"])
	panels := config["panels"].([]interface{})
	require.Equal(t, "${prom}", panels[0].(map[string]interface{})["datasourceValue"])
}

func TestImportDashboardTemplate_PreservesTemplateNameAndUnpinnedDatasource(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Board{}, &models.BoardPayload{}, &models.BusiGroup{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	dashboardImportFixture(t, deps, `{"name":"Template title","tags":"template-tag","configs":"{\"panels\":[]}"}`)

	out, err := importDashboardTemplate(context.Background(), deps, map[string]interface{}{
		"group_id": "1", "component": "Test", "file": "overview.json",
	}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "Template title", result["name"])
	require.Equal(t, "template-tag", result["tags"])
	require.NotContains(t, result, "datasource_id")
	require.Equal(t, float64(0), result["panels_count"])
}

func TestImportDashboardTemplate_RejectsInvalidTemplateWithoutWriting(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    string
	}{
		{name: "invalid JSON", payload: `{`, want: "invalid template JSON"},
		{name: "missing config", payload: `{"name":"CPU"}`, want: "has no configs"},
		{name: "invalid config", payload: `{"name":"CPU","configs":"{"}`, want: "failed to parse template configs"},
		{name: "missing name", payload: `{"configs":{"panels":[]}}`, want: "has no name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.Board{}, &models.BoardPayload{}, &models.BusiGroup{})
			require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			dashboardImportFixture(t, deps, tc.payload)

			out, err := importDashboardTemplate(context.Background(), deps, map[string]interface{}{
				"group_id": "1", "component": "Test", "file": "overview.json",
			}, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			var count int64
			require.NoError(t, models.DB(deps.DBCtx).Model(&models.Board{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestImportDashboardTemplate_RejectsReadOnlyGroup(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Board{}, &models.BusiGroup{})
	resourceMembership(t, deps, "ro", PermDashboardsAdd)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)

	out, err := importDashboardTemplate(context.Background(), deps, map[string]interface{}{
		"group_id": "1", "component": "Test", "file": "overview.json",
	}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no rw permission")
	var count int64
	require.NoError(t, models.DB(deps.DBCtx).Model(&models.Board{}).Count(&count).Error)
	require.Zero(t, count)
}

const promImportRound3YAML = `groups:
- name: operations
  interval: 30s
  rules:
  - alert: CPUHigh
    expr: cpu_usage > 80
    for: 2m
    labels:
      severity: critical
      service: api
    annotations:
      summary: CPU usage is high
  - record: job:cpu:avg
    expr: avg(cpu_usage)
`

func TestPreviewPromRuleYAML_ReturnsAlertSemanticsWithoutDB(t *testing.T) {
	out, err := previewPromRuleYAML(context.Background(), nil, map[string]interface{}{"payload": promImportRound3YAML}, nil)

	require.NoError(t, err)
	var result struct {
		Total int                   `json:"total"`
		Items []promRulePreviewItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []promRulePreviewItem{{
		Name: "CPUHigh", Severity: 1, PromQl: "cpu_usage > 80", ForDurationSec: 120, EvalIntervalSec: 30,
		AppendTags: []string{"service=api"}, Annotations: map[string]string{"summary": "CPU usage is high"},
	}}, result.Items)
}

func TestImportPromRuleYAML_PersistsDatasourceAndAuditFields(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 3, Name: "metrics", PluginType: "prometheus"}).Error)
	args := map[string]interface{}{
		"group_id": "1", "datasource_ids": "[3]", "payload": promImportRound3YAML,
		"name_prefix": "prod-", "name_suffix": "-imported", "disabled": float64(1),
	}

	out, err := importPromRuleYAML(context.Background(), deps, args, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result struct {
		Created int                  `json:"created"`
		Skipped int                  `json:"skipped"`
		Failed  int                  `json:"failed"`
		Items   []promRuleImportItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Created)
	require.Zero(t, result.Failed)
	require.Zero(t, result.Skipped)
	require.Len(t, result.Items, 1)
	require.Equal(t, "created", result.Items[0].Status)
	saved, err := models.AlertRuleGetById(deps.DBCtx, result.Items[0].Id)
	require.NoError(t, err)
	require.Equal(t, "prod-CPUHigh-imported", saved.Name)
	require.Equal(t, int64(1), saved.GroupId)
	require.Equal(t, "root", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
	require.Equal(t, 1, saved.Disabled)
	require.Equal(t, 1, saved.Severity)
	require.Equal(t, 120, saved.PromForDuration)
	require.Equal(t, "@every 30s", saved.CronPattern)
	require.Equal(t, []models.DatasourceQuery{{
		MatchType: 0, Op: "in", Values: []interface{}{float64(3)},
	}}, saved.DatasourceQueries)
	require.Equal(t, []string{"service=api"}, saved.AppendTagsJSON)
	require.Equal(t, "CPU usage is high", saved.AnnotationsJSON["summary"])
}

func TestImportPromRuleYAML_SkipsDuplicateWithoutChangingExistingRule(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 3, PluginType: "prometheus"}).Error)
	require.NoError(t, db.Create(&models.AlertRule{Id: 7, Name: "CPUHigh", GroupId: 1, Note: "keep me"}).Error)

	out, err := importPromRuleYAML(context.Background(), deps, map[string]interface{}{
		"group_id": "1", "datasource_ids": "[3]", "payload": promImportRound3YAML,
	}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result struct {
		Created int `json:"created"`
		Skipped int `json:"skipped"`
		Failed  int `json:"failed"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Zero(t, result.Created)
	require.Equal(t, 1, result.Skipped)
	require.Zero(t, result.Failed)
	saved, err := models.AlertRuleGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, "keep me", saved.Note)
	var count int64
	require.NoError(t, db.Model(&models.AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestImportPromRuleYAML_RejectsHiddenDatasource(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 3, PluginType: "prometheus"}).Error)
	deps.FilterDatasources = func(rows []*models.Datasource, user *models.User) []*models.Datasource { return nil }

	out, err := importPromRuleYAML(context.Background(), deps, map[string]interface{}{
		"group_id": "1", "datasource_ids": "[3]", "payload": promImportRound3YAML,
	}, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to datasource 3")
	var count int64
	require.NoError(t, db.Model(&models.AlertRule{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestImportPromRuleYAML_RejectsInvalidArgumentsWithoutWriting(t *testing.T) {
	cases := []struct {
		name    string
		ids     string
		payload string
		want    string
	}{
		{name: "missing datasources", payload: promImportRound3YAML, want: "datasource_ids is required"},
		{name: "malformed datasources", ids: "[", payload: promImportRound3YAML, want: "JSON array of integers"},
		{name: "empty datasources", ids: "[]", payload: promImportRound3YAML, want: "datasource_ids cannot be empty"},
		{name: "missing payload", ids: "[3]", want: "payload or payload_file is required"},
		{name: "invalid YAML", ids: "[3]", payload: "{", want: "invalid yaml format"},
		{
			name: "recording rules only", ids: "[3]",
			payload: "record: job:cpu:avg\nexpr: avg(cpu_usage)", want: "no alert rules parsed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{}, &models.Datasource{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			require.NoError(t, db.Create(&models.Datasource{Id: 3, PluginType: "prometheus"}).Error)

			out, err := importPromRuleYAML(context.Background(), deps, map[string]interface{}{
				"group_id": "1", "datasource_ids": tc.ids, "payload": tc.payload,
			}, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			var count int64
			require.NoError(t, db.Model(&models.AlertRule{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}
