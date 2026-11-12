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

func round5AlertTemplate(t *testing.T, deps *aiagent.ToolDeps, payload string) {
	t.Helper()
	root := t.TempDir()
	deps.SkillsPath = filepath.Join(root, "skills")
	dir := filepath.Join(root, "integrations", "Test", "alerts")
	require.NoError(t, os.MkdirAll(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rules.json"), []byte(payload), 0600))
}

func TestImportAlertRuleTemplate_PersistsSelectedRulesAndReportsMissingNames(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 7, Name: "metrics", PluginType: "prometheus"}).Error)
	round5AlertTemplate(t, deps, alertPackFixture)

	out, err := importAlertRuleTemplate(context.Background(), deps, map[string]interface{}{
		"component": "Test", "file": "rules.json", "names": `["CPU high","missing"]`,
		"disabled": float64(1), "name_prefix": "Imported ", "name_suffix": " prod",
	}, map[string]string{"user_id": "1", "busi_group_id": "1", "datasource_id": "7"})

	require.NoError(t, err)
	var result struct {
		Created       int                      `json:"created"`
		Failed        int                      `json:"failed"`
		Source        string                   `json:"source"`
		Datasource    string                   `json:"datasource_name"`
		NotFoundNames []string                 `json:"not_found_names"`
		Items         []promRuleImportItem     `json:"items"`
		Cards         []map[string]interface{} `json:"cards"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Created)
	require.Zero(t, result.Failed)
	require.Equal(t, "Test/rules.json", result.Source)
	require.Equal(t, "metrics", result.Datasource)
	require.Equal(t, []string{"missing"}, result.NotFoundNames)
	require.Len(t, result.Items, 1)
	require.Equal(t, "created", result.Items[0].Status)
	saved, err := models.AlertRuleGetById(deps.DBCtx, result.Items[0].Id)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.NoError(t, saved.DB2FE())
	require.Equal(t, "Imported CPU high prod", saved.Name)
	require.Equal(t, int64(1), saved.GroupId)
	require.Equal(t, 1, saved.Disabled)
	require.Equal(t, "root", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
	// DatasourceQueries is the persisted binding; datasource_ids is a deprecated display field.
	require.Equal(t, []models.DatasourceQuery{{
		MatchType: 0, Op: "in", Values: []interface{}{float64(7)},
	}}, saved.DatasourceQueries)
	require.Equal(t, 1, saved.NotifyVersion)
	require.Empty(t, saved.NotifyChannelsJSON)
	require.Empty(t, saved.NotifyGroupsJSON)
	require.Empty(t, saved.CallbacksJSON)
	require.Len(t, result.Cards, 1)
	require.Equal(t, "Imported CPU high prod", result.Cards[0]["name"])
	require.Equal(t, float64(7), result.Cards[0]["datasource_id"])
	require.Equal(t, float64(2), result.Cards[0]["severity"])
	require.Equal(t, "cpu_usage_active > 80", result.Cards[0]["prom_ql"])
	var count int64
	require.NoError(t, db.Model(&models.AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestImportAlertRuleTemplate_SeparatesCreatedDuplicateAndInvalidRules(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.AlertRule{Id: 40, GroupId: 1, Name: "CPU high", Note: "keep existing"}).Error)
	rules := parseAlertPack(t, alertPackFixture)
	rules = append(rules, models.AlertRule{Name: "invalid", Cate: "prometheus"})
	payload, err := json.Marshal(rules)
	require.NoError(t, err)
	round5AlertTemplate(t, deps, string(payload))

	out, err := importAlertRuleTemplate(context.Background(), deps, map[string]interface{}{
		"component": "Test", "file": "rules.json", "group_id": "1",
	}, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result struct {
		Total   int                      `json:"total"`
		Created int                      `json:"created"`
		Skipped int                      `json:"skipped"`
		Failed  int                      `json:"failed"`
		Items   []promRuleImportItem     `json:"items"`
		Cards   []map[string]interface{} `json:"cards"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 3, result.Total)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 1, result.Skipped)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, "skipped_duplicate", result.Items[0].Status)
	require.Empty(t, result.Items[0].Error)
	require.Zero(t, result.Items[0].Id)
	require.Equal(t, "created", result.Items[1].Status)
	require.Positive(t, result.Items[1].Id)
	require.Equal(t, "failed", result.Items[2].Status)
	require.Contains(t, result.Items[2].Error, "prom_ql is blank")
	require.Len(t, result.Cards, 1)
	require.Equal(t, "Target lost", result.Cards[0]["name"])
	require.NotContains(t, result.Cards[0], "datasource_id")
	existing, err := models.AlertRuleGetById(deps.DBCtx, 40)
	require.NoError(t, err)
	require.Equal(t, "keep existing", existing.Note)
	var count int64
	require.NoError(t, db.Model(&models.AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestImportAlertRuleTemplate_RejectsInvalidSelectionWithoutWriting(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{name: "missing component", args: map[string]interface{}{"file": "rules.json"}, want: "component is required"},
		{name: "missing file", args: map[string]interface{}{"component": "Test"}, want: "file is required"},
		{
			name: "invalid names", args: map[string]interface{}{"component": "Test", "file": "rules.json", "names": "CPU"},
			want: "names must be a JSON array",
		},
		{
			name: "unknown name",
			args: map[string]interface{}{"component": "Test", "file": "rules.json", "names": `["missing"]`},
			want: "none of the requested names",
		},
		{
			name: "invalid disabled",
			args: map[string]interface{}{"component": "Test", "file": "rules.json", "disabled": float64(2)},
			want: "disabled",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
			require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			round5AlertTemplate(t, deps, alertPackFixture)

			out, err := importAlertRuleTemplate(context.Background(), deps, tc.args,
				map[string]string{"user_id": "1", "busi_group_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			var count int64
			require.NoError(t, models.DB(deps.DBCtx).Model(&models.AlertRule{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestImportAlertRuleTemplate_DeniesInaccessibleDatasource(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 7, Name: "private"}).Error)
	deps.FilterDatasources = func(sources []*models.Datasource, user *models.User) []*models.Datasource {
		require.Equal(t, int64(1), user.Id)
		require.Len(t, sources, 1)
		require.Equal(t, int64(7), sources[0].Id)
		return nil
	}

	out, err := importAlertRuleTemplate(context.Background(), deps, map[string]interface{}{
		"component": "Test", "file": "rules.json", "datasource_id": "7", "group_id": "1",
	}, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "forbidden: no access to datasource 7")
	var count int64
	require.NoError(t, db.Model(&models.AlertRule{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestPreviewAlertRuleTemplate_ReturnsSummariesWithoutWriting(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{})
	round5AlertTemplate(t, deps, alertPackFixture)

	out, err := previewAlertRuleTemplate(context.Background(), deps, map[string]interface{}{
		"component": " Test ", "file": " rules.json ",
	}, nil)

	require.NoError(t, err)
	var result struct {
		Total int                        `json:"total"`
		Items []alertTemplatePreviewItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 2, result.Total)
	require.Equal(t, []alertTemplatePreviewItem{
		{Name: "CPU high", Cate: "prometheus", Summary: "cpu_usage_active > 80", Disabled: 1},
		{Name: "Target lost", Cate: "host", Summary: "offset > [300]", Disabled: 1},
	}, result.Items)
	var count int64
	require.NoError(t, models.DB(deps.DBCtx).Model(&models.AlertRule{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestLoadAlertRuleTemplate_RejectsInvalidFiles(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		file    string
		want    string
	}{
		{name: "malformed JSON", payload: "{", file: "rules.json", want: "invalid alert template JSON"},
		{name: "wrong shape", payload: "{}", file: "rules.json", want: "expected an array"},
		{name: "empty pack", payload: "[]", file: "rules.json", want: "has no rules"},
		{name: "absent file", payload: "[]", file: "absent.json", want: "failed to read template"},
		{name: "path escape", payload: "[]", file: "../../../outside.json", want: "invalid path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := &aiagent.ToolDeps{}
			round5AlertTemplate(t, deps, tc.payload)

			rules, err := loadAlertRuleTemplate(deps, "Test", tc.file)

			require.Nil(t, rules)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestNormalizeQueryIntervals_ConvertsExplicitUnitsToSeconds(t *testing.T) {
	cases := []struct {
		name     string
		interval interface{}
		unit     string
		want     int
	}{
		{name: "seconds", interval: float64(15), unit: "s", want: 15},
		{name: "minutes", interval: 2, unit: "MINUTE", want: 120},
		{name: "hours", interval: int64(2), unit: "hour", want: 7200},
		{name: "days", interval: float64(0.5), unit: "d", want: 43200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query := map[string]interface{}{"interval": tc.interval, "interval_unit": tc.unit, "sql": "select 1"}
			config := map[string]interface{}{"queries": []interface{}{query}}

			normalizeQueryIntervals(config)

			require.Equal(t, tc.want, query["interval"])
			require.NotContains(t, query, "interval_unit")
			require.Equal(t, "select 1", query["sql"])
		})
	}
}
