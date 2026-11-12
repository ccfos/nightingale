package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newResourceTestDeps(t *testing.T, tables ...interface{}) *aiagent.ToolDeps {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(append([]interface{}{&models.User{}}, tables...)...))
	require.NoError(t, db.Create(&models.User{Id: 1, Username: "root", Roles: models.AdminRole}).Error)
	require.NoError(t, db.Create(&models.User{Id: 2, Username: "alice", Roles: "Standard"}).Error)
	return &aiagent.ToolDeps{DBCtx: ctxpkg.NewContext(context.Background(), db, true)}
}

func resourceMembership(t *testing.T, deps *aiagent.ToolDeps, perm string, operations ...string) {
	t.Helper()
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.AutoMigrate(&models.UserGroupMember{}, &models.BusiGroupMember{}))
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 2}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: perm}).Error)
	for _, op := range operations {
		if op == "" {
			continue
		}
		require.NoError(t, db.AutoMigrate(&models.RoleOperation{}))
		require.NoError(t, db.Create(&models.RoleOperation{RoleName: "Standard", Operation: op}).Error)
	}
}

func isolatedResourceProposals(t *testing.T) {
	t.Helper()
	previous := updateProposals
	updateProposals = &proposalStore{items: map[string]*updateProposal{}}
	t.Cleanup(func() { updateProposals = previous })
}

func TestCreateAlertMute_PersistsNormalizedConfig(t *testing.T) {
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertMute{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	config := `{"id":999,"note":"maintenance","cause":"release","btime":1000,
		"tags":[{"key":"service","op":"in","value":["api","worker"]}],"severities":[1,2]}`

	out, err := createAlertMute(context.Background(), deps, map[string]interface{}{
		"config": config, "duration": "2h",
	}, map[string]string{"user_id": "1", "busi_group_id": "1"})

	require.NoError(t, err)
	var result struct {
		ID    int64  `json:"id"`
		URL   string `json:"url"`
		Group string `json:"group_name"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Positive(t, result.ID)
	require.NotEqual(t, int64(999), result.ID, "a caller-supplied primary key must be discarded")
	require.Equal(t, "Operations", result.Group)
	require.Equal(t, fmt.Sprintf("/alert-mutes/edit/%d?bgid=1", result.ID), result.URL)
	saved, err := models.AlertMuteGetById(deps.DBCtx, result.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, int64(1), saved.GroupId)
	require.Equal(t, int64(1000), saved.Btime)
	require.Equal(t, int64(8200), saved.Etime)
	require.Equal(t, []int64{0}, saved.DatasourceIdsJson)
	require.Equal(t, []int{1, 2}, saved.SeveritiesJson)
	require.Equal(t, "root", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
	var filters []models.TagFilter
	require.NoError(t, json.Unmarshal(saved.Tags, &filters))
	require.Len(t, filters, 1)
	require.Equal(t, "service", filters[0].Key)
	require.Equal(t, "in", filters[0].Func)
	require.Equal(t, "in", filters[0].Op)
	require.Equal(t, "api worker", filters[0].Value)
}

func TestCreateAlertMute_RejectsInvalidConfigWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		config string
		error  string
	}{
		{name: "missing", config: "", error: "config is required"},
		{name: "invalid JSON", config: "{", error: "invalid config JSON"},
		{name: "missing group", config: `{"group_id":9}`, error: "busi group not found"},
		{name: "missing end time", config: `{"group_id":1,"btime":1000}`, error: "duration"},
		{name: "inverted times", config: `{"group_id":1,"btime":2000,"etime":1000}`, error: "etime"},
		{
			name:   "invalid tag operator",
			config: `{"group_id":1,"btime":1000,"etime":2000,"tags":[{"key":"host","func":"contains","value":"web"}]}`,
			error:  "tags[0]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertMute{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)

			out, err := createAlertMute(context.Background(), deps,
				map[string]interface{}{"config": tc.config}, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.error)
			var count int64
			require.NoError(t, db.Model(&models.AlertMute{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestCreateAlertMute_ReadOnlyGroupCannotWrite(t *testing.T) {
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertMute{})
	resourceMembership(t, deps, "ro", PermAlertMutesAdd)
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)

	out, err := createAlertMute(context.Background(), deps, map[string]interface{}{
		"config": `{"group_id":1,"btime":1000,"etime":2000}`,
	}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no rw permission on busi group 1")
	var count int64
	require.NoError(t, db.Model(&models.AlertMute{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestUpdateAlertMute_ConfirmsPatchAndPreservesOtherFields(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertMute{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	mute := &models.AlertMute{
		Id: 7, GroupId: 1, Note: "maintenance", Cause: "old", Btime: 1000, Etime: 2000,
		DatasourceIdsJson: []int64{3}, SeveritiesJson: []int{1, 2}, Tags: ormx.JSONArr(`[]`), CreateBy: "original",
	}
	require.NoError(t, mute.Add(deps.DBCtx))

	out, err := updateAlertMute(context.Background(), deps, map[string]interface{}{
		"id": "7", "config": `{"cause":"release","disabled":1,"severities":[1]}`,
	}, map[string]string{"user_id": "1", "chat_id": "mute-patch", "seq_id": "1", "lang": "en_US"})

	require.Empty(t, out)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	require.Equal(t, aiagent.InterruptKindApproval, interrupt.Kind)
	require.Contains(t, interrupt.Prompt, "maintenance")
	before, err := models.AlertMuteGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, "old", before.Cause)
	require.Zero(t, before.Disabled)
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))

	out, err = updateAlertMute(context.Background(), deps, replay,
		map[string]string{"user_id": "1", "chat_id": "mute-patch", "seq_id": "2"})

	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, true, result["applied"])
	require.Equal(t, []interface{}{"cause", "disabled", "severities"}, result["updated"])
	saved, err := models.AlertMuteGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, "release", saved.Cause)
	require.Equal(t, 1, saved.Disabled)
	require.Equal(t, []int{1}, saved.SeveritiesJson)
	require.Equal(t, []int64{3}, saved.DatasourceIdsJson)
	require.Equal(t, "original", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
	require.Equal(t, int64(1000), saved.Btime)
	require.Equal(t, int64(2000), saved.Etime)
}

func TestUpdateAlertMute_RejectsConflictingPatches(t *testing.T) {
	cases := []struct {
		name     string
		config   string
		duration string
		error    string
	}{
		{name: "no fields", error: "nothing to update"},
		{name: "identity only", config: `{"id":99}`, error: "no changeable fields"},
		{name: "move group", config: `{"group_id":2}`, error: "group_id cannot be changed"},
		{name: "clear severity", config: `{"severities":[]}`, error: "severities cannot be cleared"},
		{name: "ambiguous end", config: `{"etime":3000}`, duration: "1h", error: "mutually exclusive"},
		{name: "invalid duration", duration: "tomorrow", error: "tomorrow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertMute{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			mute := &models.AlertMute{Id: 7, GroupId: 1, Cause: "original", Btime: 1000, Etime: 2000}
			require.NoError(t, mute.Add(deps.DBCtx))

			out, err := updateAlertMute(context.Background(), deps, map[string]interface{}{
				"id": "7", "config": tc.config, "duration": tc.duration,
			}, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.error)
			saved, err := models.AlertMuteGetById(deps.DBCtx, 7)
			require.NoError(t, err)
			require.Equal(t, "original", saved.Cause)
			require.Equal(t, int64(1), saved.GroupId)
			require.Equal(t, int64(2000), saved.Etime)
		})
	}
}

func TestListAlertMutes_RestrictsGroupsAndFiltersCause(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertMute{})
	resourceMembership(t, deps, "ro", PermAlertMutes)
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.AlertMute{
		{Id: 1, GroupId: 1, Cause: "Release Window", Btime: 1000, Etime: 2000, CreateBy: "root"},
		{Id: 2, GroupId: 1, Cause: "backup", Btime: 1000, Etime: 2000},
		{Id: 3, GroupId: 2, Cause: "Release private", Btime: 1000, Etime: 2000},
	}).Error)

	out, err := listAlertMutes(context.Background(), deps,
		map[string]interface{}{"query": "release", "limit": float64(1)},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int               `json:"total"`
		Items []alertMuteResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []alertMuteResult{{
		Id: 1, GroupId: 1, Cause: "Release Window", CreateBy: "root",
		Btime: formatUnixTime(1000), Etime: formatUnixTime(2000),
	}}, result.Items)
}

func TestListBusiGroups_ReturnsOnlyMembershipAndDefaultHint(t *testing.T) {
	deps := newResourceTestDeps(t, &models.BusiGroup{})
	resourceMembership(t, deps, "ro")
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.BusiGroup{
		{Id: 1, Name: "DEFAULT Operations", LabelValue: "ops"}, {Id: 2, Name: "Private"},
	}).Error)

	out, err := listBusiGroups(context.Background(), deps, nil, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int               `json:"total"`
		Items []busiGroupResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []busiGroupResult{{
		Id: 1, Name: "DEFAULT Operations", LabelValue: "ops", IsDefault: true,
	}}, result.Items)
}

func TestListTargets_RestrictsGroupsAndSplitsTags(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{}, &models.TargetBusiGroup{})
	resourceMembership(t, deps, "ro")
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&[]models.Target{
		{Id: 1, Ident: "web-1", Note: "frontend", Tags: "env=prod  service=web ", HostIp: "127.0.0.1", OS: "linux"},
		{Id: 2, Ident: "web-private"}, {Id: 3, Ident: "db-1"},
	}).Error)
	require.NoError(t, db.Create(&[]models.TargetBusiGroup{
		{TargetIdent: "web-1", GroupId: 1}, {TargetIdent: "web-private", GroupId: 2}, {TargetIdent: "db-1", GroupId: 1},
	}).Error)

	out, err := listTargets(context.Background(), deps, map[string]interface{}{"query": "web", "limit": float64(1)},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int            `json:"total"`
		Items []targetResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []targetResult{{
		Id: 1, Ident: "web-1", Note: "frontend", HostIp: "127.0.0.1", OS: "linux",
		Tags: []string{"env=prod", "service=web"},
	}}, result.Items)
}

func TestGetTargetDetail_ResolvesIdentAndReturnsGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.Target{}, &models.TargetBusiGroup{})
	resourceMembership(t, deps, "ro")
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.Target{
		Id: 7, Ident: "web-1", Tags: "env=prod", HostTags: []string{"arch=arm64"}, AgentVersion: "1.0", UpdateAt: 1234,
	}).Error)
	require.NoError(t, db.Create(&models.TargetBusiGroup{TargetIdent: "web-1", GroupId: 1}).Error)

	out, err := getTargetDetail(context.Background(), deps, map[string]interface{}{"ident": "web-1"},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result targetDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, int64(7), result.Id)
	require.Equal(t, "web-1", result.Ident)
	require.Equal(t, []string{"env=prod"}, result.Tags)
	require.Equal(t, []string{"arch=arm64"}, result.HostTags)
	require.Equal(t, []int64{1}, result.GroupIds)
	require.Equal(t, int64(1234), result.UpdateAt)
}

func TestResourceDetails_RejectOtherGroups(t *testing.T) {
	cases := []struct {
		name    string
		handler aiagent.BuiltinToolFunc
		model   interface{}
		row     interface{}
		op      string
		error   string
	}{
		{
			name: "mute", handler: getAlertMuteDetail, model: &models.AlertMute{}, op: PermAlertMutes,
			row: &models.AlertMute{Id: 7, GroupId: 2}, error: "no access to this alert mute",
		},
		{
			name: "rule", handler: getAlertRuleDetail, model: &models.AlertRule{}, op: PermAlertRules,
			row: &models.AlertRule{Id: 7, GroupId: 2}, error: "no access to this alert rule",
		},
		{
			name: "target", handler: getTargetDetail, model: &models.Target{},
			row: &models.Target{Id: 7, Ident: "private"}, error: "no access to this target",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, tc.model)
			resourceMembership(t, deps, "ro", tc.op)
			db := models.DB(deps.DBCtx)
			if mute, ok := tc.row.(*models.AlertMute); ok {
				require.NoError(t, mute.FE2DB())
			}
			require.NoError(t, db.Create(tc.row).Error)
			if tc.name == "target" {
				require.NoError(t, db.AutoMigrate(&models.TargetBusiGroup{}))
				require.NoError(t, db.Create(&models.TargetBusiGroup{TargetIdent: "private", GroupId: 2}).Error)
			}

			out, err := tc.handler(context.Background(), deps, map[string]interface{}{"id": "7"},
				map[string]string{"user_id": "2"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.error)
		})
	}
}

func TestGetAlertMuteDetail_ReturnsTagsAsJSONArray(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertMute{})
	resourceMembership(t, deps, "ro", PermAlertMutes)
	db := models.DB(deps.DBCtx)
	mute := &models.AlertMute{Id: 7, GroupId: 1, Tags: ormx.JSONArr(`[{"key":"host","func":"==","value":"web-1"}]`)}
	require.NoError(t, mute.FE2DB())
	require.NoError(t, db.Create(mute).Error)

	out, err := getAlertMuteDetail(context.Background(), deps, map[string]interface{}{"id": "7"},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var detail struct {
		Tags []map[string]string `json:"tags"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &detail))
	require.Equal(t, []map[string]string{{"key": "host", "func": "==", "value": "web-1"}}, detail.Tags)
}

func TestCreateAlertRule_PersistsThresholdAndDatasource(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{}, &models.Datasource{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Datasource{Id: 3, Name: "metrics", PluginType: "prometheus"}).Error)

	out, err := createAlertRule(context.Background(), deps, map[string]interface{}{
		"name": "CPU high", "prom_ql": "busy / total", "threshold": "0.8", "operator": ">=", "severity": float64(1),
		"append_tags": "env=prod  service=api", "notify_rule_ids": "[4,5]", "note": "Investigate CPU",
	}, map[string]string{"user_id": "1", "busi_group_id": "1", "datasource_id": "3"})

	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, "CPU high", result["name"])
	require.Equal(t, "metrics", result["datasource_name"])
	require.Equal(t, float64(0.8), result["threshold"])
	require.Equal(t, ">=", result["operator"])
	require.Equal(t, "Operations", result["group_name"])
	saved, err := models.AlertRuleGetById(deps.DBCtx, int64(result["id"].(float64)))
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "(busy / total) >= 0.8", currentBakedPromQL(saved.RuleConfigJson))
	require.Equal(t, 1, saved.Severity)
	require.Equal(t, 30, saved.PromEvalInterval)
	require.Equal(t, 60, saved.PromForDuration)
	require.Equal(t, []string{"env=prod", "service=api"}, saved.AppendTagsJSON)
	require.Equal(t, []int64{4, 5}, saved.NotifyRuleIds)
	require.Len(t, saved.DatasourceQueries, 1)
	require.Equal(t, "in", saved.DatasourceQueries[0].Op)
	require.Equal(t, []interface{}{float64(3)}, saved.DatasourceQueries[0].Values)
	require.Equal(t, "root", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
}

func TestCreateAlertRule_RejectsInvalidArgumentsWithoutWriting(t *testing.T) {
	cases := []struct {
		name  string
		patch map[string]interface{}
		drop  string
		error string
	}{
		{name: "missing name", drop: "name", error: "name is required"},
		{name: "unknown type", patch: map[string]interface{}{"cate": "unknown"}, error: "invalid cate"},
		{name: "severity", patch: map[string]interface{}{"severity": float64(4)}, error: "severity must be"},
		{name: "group", patch: map[string]interface{}{"group_id": float64(9)}, error: "busi group not found"},
		{name: "datasource", drop: "datasource_id", error: "datasource_id is required"},
		{name: "expression", drop: "prom_ql", error: "prom_ql is required"},
		{name: "threshold", drop: "threshold", error: "threshold is required"},
		{name: "operator", patch: map[string]interface{}{"operator": "contains"}, error: "invalid operator"},
		{name: "raw config", patch: map[string]interface{}{"rule_config_json": "["}, error: "valid JSON object"},
		{
			name: "non-prom config", patch: map[string]interface{}{"cate": "mysql"},
			error: "rule_config_json is required",
		},
		{name: "notify ids", patch: map[string]interface{}{"notify_rule_ids": "oops"}, error: "JSON array of integers"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			args := map[string]interface{}{
				"group_id": "1", "name": "CPU high", "datasource_id": "3", "prom_ql": "cpu", "threshold": float64(80),
			}
			for key, value := range tc.patch {
				args[key] = value
			}
			delete(args, tc.drop)

			out, err := createAlertRule(context.Background(), deps, args, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.error)
			var count int64
			require.NoError(t, db.Model(&models.AlertRule{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestCreateAlertRule_RejectsDuplicateName(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.AlertRule{GroupId: 1, Name: "CPU high"}).Error)

	out, err := createAlertRule(context.Background(), deps, map[string]interface{}{
		"group_id": "1", "name": "CPU high", "datasource_id": "3", "prom_ql": "cpu", "threshold": float64(80),
	}, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, `alert rule name "CPU high" already exists`)
	require.ErrorContains(t, err, "CPU high-v2")
	var count int64
	require.NoError(t, db.Model(&models.AlertRule{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestUpdateAlertRule_ConfirmsThresholdAndSeverity(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.AlertRule{
		Id: 7, GroupId: 1, Name: "CPU high", Cate: "prometheus", Severity: 2, Disabled: 1, CreateBy: "original",
		PromEvalInterval: 30, PromForDuration: 60, RuleConfig: `{"queries":[{"prom_ql":"cpu > 80","severity":2}]}`,
	}).Error)

	out, err := updateAlertRule(context.Background(), deps, map[string]interface{}{
		"id": "7", "threshold": float64(90), "severity": float64(1), "disabled": float64(0),
		"for_duration": float64(0), "eval_interval": float64(15),
	}, map[string]string{"user_id": "1", "chat_id": "rule-patch", "seq_id": "1", "lang": "en_US"})

	require.Empty(t, out)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	require.Equal(t, aiagent.InterruptKindApproval, interrupt.Kind)
	require.Contains(t, interrupt.Prompt, "cpu > 90")
	before, err := models.AlertRuleGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, "cpu > 80", currentBakedPromQL(before.RuleConfigJson))
	require.Equal(t, 1, before.Disabled)
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))

	out, err = updateAlertRule(context.Background(), deps, replay,
		map[string]string{"user_id": "1", "chat_id": "rule-patch", "seq_id": "2", "lang": "en_US"})

	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, true, result["applied"])
	require.Equal(t, "cpu > 90", result["prom_ql"])
	saved, err := models.AlertRuleGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, "cpu > 90", currentBakedPromQL(saved.RuleConfigJson))
	queries, ok := promQueries(saved.RuleConfigJson)
	require.True(t, ok)
	require.Equal(t, float64(1), queries[0]["severity"])
	require.Zero(t, saved.Disabled)
	require.Zero(t, saved.PromForDuration)
	require.Equal(t, 15, saved.PromEvalInterval)
	require.Equal(t, "@every 15s", saved.CronPattern)
	require.Equal(t, "original", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
}

func TestUpdateAlertRule_RejectsInvalidEditsWithoutWriting(t *testing.T) {
	cases := []struct {
		name  string
		patch map[string]interface{}
		error string
	}{
		{name: "no fields", error: "no updatable fields"},
		{name: "disabled", patch: map[string]interface{}{"disabled": float64(2)}, error: "disabled must be"},
		{name: "interval", patch: map[string]interface{}{"eval_interval": float64(0)}, error: "positive number"},
		{name: "duration", patch: map[string]interface{}{"for_duration": float64(-1)}, error: "for_duration must be"},
		{name: "severity", patch: map[string]interface{}{"severity": float64(0)}, error: "severity must be"},
		{name: "notify ids", patch: map[string]interface{}{"notify_rule_ids": "oops"}, error: "JSON array of integers"},
		{name: "raw config", patch: map[string]interface{}{"rule_config_json": "{"}, error: "valid JSON object"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			require.NoError(t, db.Create(&models.AlertRule{
				Id: 7, GroupId: 1, Name: "CPU high", Cate: "prometheus", Severity: 2,
				RuleConfig: `{"queries":[{"prom_ql":"cpu > 80","severity":2}]}`,
			}).Error)
			args := map[string]interface{}{"id": "7"}
			for key, value := range tc.patch {
				args[key] = value
			}

			out, err := updateAlertRule(context.Background(), deps, args, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.error)
			saved, err := models.AlertRuleGetById(deps.DBCtx, 7)
			require.NoError(t, err)
			require.Equal(t, "cpu > 80", currentBakedPromQL(saved.RuleConfigJson))
			require.Equal(t, 2, saved.Severity)
		})
	}
}

func TestListAlertRules_FiltersByMembershipNameAndDisabled(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{})
	resourceMembership(t, deps, "ro", PermAlertRules)
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.AlertRule{
		{Id: 1, GroupId: 1, Name: "CPU active", Severity: 1, Cate: "prometheus"},
		{Id: 2, GroupId: 1, Name: "CPU disabled", Disabled: 1},
		{Id: 3, GroupId: 1, Name: "disk"}, {Id: 4, GroupId: 2, Name: "CPU private"},
	}).Error)

	out, err := listAlertRules(context.Background(), deps,
		map[string]interface{}{"query": "cpu", "disabled": float64(0), "limit": float64(1)},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int               `json:"total"`
		Items []alertRuleResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []alertRuleResult{{
		Id: 1, GroupId: 1, Name: "CPU active", Severity: 1, Cate: "prometheus",
	}}, result.Items)
}

func TestGetAlertRuleDetail_ReturnsConfigAndAnnotations(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{})
	resourceMembership(t, deps, "ro", PermAlertRules)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertRule{
		Id: 7, GroupId: 1, Name: "CPU high", Note: "inspect", Severity: 2, Cate: "prometheus",
		RuleConfig: `{"queries":[{"prom_ql":"cpu > 80","severity":2}]}`, Annotations: `{"summary":"CPU saturation"}`,
		AppendTags: "env=prod", NotifyRuleIds: []int64{3}, CreateBy: "original", UpdateBy: "root",
	}).Error)

	out, err := getAlertRuleDetail(context.Background(), deps, map[string]interface{}{"id": "7"},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result alertRuleDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, int64(7), result.Id)
	require.Equal(t, "CPU high", result.Name)
	require.Equal(t, "cpu > 80", currentBakedPromQL(result.RuleConfig))
	require.Equal(t, map[string]string{"summary": "CPU saturation"}, result.Annotations)
	require.Equal(t, []string{"env=prod"}, result.AppendTags)
	require.Equal(t, []int64{3}, result.NotifyRuleIds)
	require.Equal(t, "original", result.CreateBy)
}

func TestResourceWrites_RequireRolePermission(t *testing.T) {
	cases := []struct {
		name    string
		handler aiagent.BuiltinToolFunc
		op      string
	}{
		{name: "create mute", handler: createAlertMute, op: PermAlertMutesAdd},
		{name: "update mute", handler: updateAlertMute, op: PermAlertMutesPut},
		{name: "create rule", handler: createAlertRule, op: PermAlertRulesAdd},
		{name: "update rule", handler: updateAlertRule, op: PermAlertRulesPut},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.RoleOperation{})

			out, err := tc.handler(context.Background(), deps, nil, map[string]string{"user_id": "2"})

			require.Empty(t, out)
			require.ErrorContains(t, err, "forbidden: no permission for "+tc.op)
		})
	}
}

func TestListLegacyNotifyAlertRules_SummarizesOnlyVisibleLegacyRules(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
	resourceMembership(t, deps, "ro", PermAlertRules)
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&[]models.AlertRule{
		{Id: 1, GroupId: 1, Name: "A configured", NotifyVersion: 0, NotifyGroups: "10 11"},
		{Id: 2, GroupId: 1, Name: "B empty", NotifyVersion: 0, Disabled: 1},
		{Id: 3, GroupId: 1, Name: "C modern", NotifyVersion: 1, NotifyRuleIds: []int64{3}},
		{Id: 4, GroupId: 2, Name: "D private", NotifyVersion: 0},
	}).Error)

	out, err := listLegacyNotifyAlertRules(context.Background(), deps,
		map[string]interface{}{"include_disabled": true}, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total   int                     `json:"total"`
		Items   []legacyAlertRuleResult `json:"items"`
		Summary map[string]int          `json:"summary"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 2, result.Total)
	require.Equal(t, map[string]int{
		"total": 2, "enabled": 1, "disabled": 1, "with_groups_configured": 1, "empty_legacy": 1,
	}, result.Summary)
	require.Len(t, result.Items, 2)
	require.Equal(t, []string{"10", "11"}, result.Items[0].NotifyGroups)
	require.Equal(t, "Operations", result.Items[0].GroupName)
	require.Equal(t, int64(2), result.Items[1].Id)
	require.NotNil(t, result.Items[1].NotifyGroups, "empty recipients must be an array in the JSON payload")
	require.NotNil(t, result.Items[1].NotifyRuleIds)
}

func TestListLegacyNotifyAlertRules_RejectsInaccessibleGroupFilter(t *testing.T) {
	deps := newResourceTestDeps(t)
	resourceMembership(t, deps, "ro", PermAlertRules)

	out, err := listLegacyNotifyAlertRules(context.Background(), deps,
		map[string]interface{}{"group_id": "2"}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to busi group 2")
}

func TestCreateAlertMute_WritableMemberMayCreate(t *testing.T) {
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertMute{})
	resourceMembership(t, deps, "rw", PermAlertMutesAdd)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)

	out, err := createAlertMute(context.Background(), deps, map[string]interface{}{
		"config": `{"group_id":1,"btime":1000,"etime":2000,"cause":"deployment"}`,
	}, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	saved, err := models.AlertMuteGetById(deps.DBCtx, result.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.Equal(t, "alice", saved.CreateBy)
	require.Equal(t, "deployment", saved.Cause)
}

func TestUpdateAlertRule_RejectsStaleProposalWithoutClobbering(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.AlertRule{}, &models.BusiGroup{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.AlertRule{
		Id: 7, GroupId: 1, Name: "CPU high", Cate: "prometheus", Severity: 2,
		RuleConfig: `{"queries":[{"prom_ql":"cpu > 80","severity":2}]}`,
	}).Error)
	_, err := updateAlertRule(context.Background(), deps, map[string]interface{}{"id": "7", "note": "AI edit"},
		map[string]string{"user_id": "1", "chat_id": "stale-rule", "seq_id": "1"})
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))
	// Simulate a human edit between the proposal and its confirmation.
	require.NoError(t, db.Model(&models.AlertRule{}).Where("id = ?", 7).Update("note", "human edit").Error)

	out, err := updateAlertRule(context.Background(), deps, replay,
		map[string]string{"user_id": "1", "chat_id": "stale-rule", "seq_id": "2", "lang": "en_US"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "changed")
	saved, err := models.AlertRuleGetById(deps.DBCtx, 7)
	require.NoError(t, err)
	require.Equal(t, "human edit", saved.Note)
}
