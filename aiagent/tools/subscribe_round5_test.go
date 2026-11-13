package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestCreateAlertSubscribe_PersistsDefaultsAndAuditFields(t *testing.T) {
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertSubscribe{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	config := `{"id":999,"name":"CPU subscription","severities":[1,2],"notify_version":1,"notify_rule_ids":[7]}`

	out, err := createAlertSubscribe(context.Background(), deps, map[string]interface{}{"config": config},
		map[string]string{"user_id": "1", "busi_group_id": "1"})

	require.NoError(t, err)
	var result struct {
		ID        int64  `json:"id"`
		GroupName string `json:"group_name"`
		URL       string `json:"url"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Positive(t, result.ID)
	require.NotEqual(t, int64(999), result.ID)
	require.Equal(t, "Operations", result.GroupName)
	require.Equal(t, fmt.Sprintf("/alert-subscribes/edit/%d", result.ID), result.URL)
	saved, err := models.AlertSubscribeGet(deps.DBCtx, "id=?", result.ID)
	require.NoError(t, err)
	require.NotNil(t, saved)
	require.NoError(t, saved.DB2FE())
	require.Equal(t, "CPU subscription", saved.Name)
	require.Equal(t, int64(1), saved.GroupId)
	require.Equal(t, []int64{0}, saved.DatasourceIdsJson)
	require.Equal(t, []int{1, 2}, saved.SeveritiesJson)
	require.Equal(t, []int64{7}, saved.NotifyRuleIds)
	require.Equal(t, "root", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
}

func TestCreateAlertSubscribe_RejectsInvalidConfigWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   string
	}{
		{name: "missing", want: "config is required"},
		{name: "malformed", config: "{", want: "invalid config JSON"},
		{name: "missing group", config: `{"group_id":9}`, want: "busi group not found"},
		{name: "missing severities", config: `{"group_id":1}`, want: "severities is required"},
		{
			name: "missing notification rule", config: `{"group_id":1,"severities":[1],"notify_version":1}`,
			want: "no notify rules selected",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertSubscribe{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)

			out, err := createAlertSubscribe(context.Background(), deps, map[string]interface{}{"config": tc.config},
				map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			var count int64
			require.NoError(t, db.Model(&models.AlertSubscribe{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestCreateAlertSubscribe_RejectsReadOnlyGroup(t *testing.T) {
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertSubscribe{})
	resourceMembership(t, deps, "ro", PermAlertSubscribesAdd)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)

	out, err := createAlertSubscribe(context.Background(), deps, map[string]interface{}{
		"config": `{"group_id":1,"name":"CPU","severities":[1]}`,
	}, map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "forbidden")
	var count int64
	require.NoError(t, models.DB(deps.DBCtx).Model(&models.AlertSubscribe{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestListAlertSubscribes_FiltersGroupAndNameBeforeLimit(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertSubscribe{})
	resourceMembership(t, deps, "ro", PermAlertSubscribes)
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.AlertSubscribe{
		{Id: 5, Name: "Private CPU", GroupId: 2},
		{Id: 4, Name: "Disk", GroupId: 1},
		{Id: 3, Name: "CPU critical", GroupId: 1, Disabled: 1, Note: "maintenance", CreateBy: "root"},
		{Id: 2, Name: "CPU warning", GroupId: 1},
	}).Error)

	out, err := listAlertSubscribes(context.Background(), deps, map[string]interface{}{"query": "cpu", "limit": float64(1)},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int                    `json:"total"`
		Items []alertSubscribeResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []alertSubscribeResult{{
		Id: 3, Name: "CPU critical", GroupId: 1, Disabled: 1, Note: "maintenance", CreateBy: "root",
	}}, result.Items)
}

func TestListAlertSubscribes_NoMembershipReturnsEmptyArray(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertSubscribe{}, &models.UserGroupMember{},
		&models.BusiGroupMember{}, &models.RoleOperation{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.RoleOperation{RoleName: "Standard", Operation: PermAlertSubscribes}).Error)
	require.NoError(t, db.Create(&models.AlertSubscribe{Id: 3, GroupId: 1, Name: "Private"}).Error)

	out, err := listAlertSubscribes(context.Background(), deps, nil, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	require.JSONEq(t, `{"total":0,"items":[]}`, out)
}

func TestListAlertSubscribes_AdminCanListAcrossGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertSubscribe{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.AlertSubscribe{
		{Id: 1, GroupId: 1, Name: "CPU"},
		{Id: 2, GroupId: 2, Name: "Disk"},
	}).Error)

	out, err := listAlertSubscribes(context.Background(), deps, nil, map[string]string{"user_id": "1"})

	require.NoError(t, err)
	var result struct {
		Total int                    `json:"total"`
		Items []alertSubscribeResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 2, result.Total)
	require.Equal(t, []alertSubscribeResult{
		{Id: 2, GroupId: 2, Name: "Disk"},
		{Id: 1, GroupId: 1, Name: "CPU"},
	}, result.Items)
}

func TestGetAlertSubscribeDetail_ReturnsMatchingConfiguration(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertSubscribe{})
	resourceMembership(t, deps, "ro", PermAlertSubscribes)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertSubscribe{
		Id: 3, GroupId: 1, Name: "CPU", Prod: "metric", Cate: "prometheus", Note: "critical only",
		RedefineSeverity: 1, NewSeverity: 2, RuleIds: []int64{7, 8}, NotifyRuleIds: []int64{9},
		CreateBy: "root", UpdateBy: "alice",
	}).Error)

	out, err := getAlertSubscribeDetail(context.Background(), deps, map[string]interface{}{"id": "3"},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result alertSubscribeDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, alertSubscribeDetailResult{
		Id: 3, GroupId: 1, Name: "CPU", Prod: "metric", Cate: "prometheus", Note: "critical only",
		RedefineSeverity: 1, NewSeverity: 2, RuleIds: []int64{7, 8}, NotifyRuleIds: []int64{9},
		CreateBy: "root", UpdateBy: "alice",
	}, result)
}

func TestGetAlertSubscribeDetail_DeniesOtherGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertSubscribe{})
	resourceMembership(t, deps, "ro", PermAlertSubscribes)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertSubscribe{Id: 3, GroupId: 2, Name: "Private"}).Error)

	out, err := getAlertSubscribeDetail(context.Background(), deps, map[string]interface{}{"id": "3"},
		map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to this alert subscribe")
}

func TestGetAlertSubscribeDetail_ReportsMissingSubscription(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertSubscribe{})

	out, err := getAlertSubscribeDetail(context.Background(), deps, map[string]interface{}{"id": "3"},
		map[string]string{"user_id": "1"})

	require.NoError(t, err)
	require.JSONEq(t, `{"error":"alert subscribe not found: id=3"}`, out)
}

func TestUpdateAlertSubscribe_ConfirmsPatchAndPreservesSerializedFields(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertSubscribe{})
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	sub := &models.AlertSubscribe{
		Id: 3, GroupId: 1, Name: "CPU", SeveritiesJson: []int{1, 2}, DatasourceIdsJson: []int64{7},
		WebhooksJson: []string{"http://127.0.0.1/unused"}, ExtraConfigJson: map[string]interface{}{"key": "value"},
		RuleId: 99, RuleIds: []int64{8}, CreateAt: 1000, CreateBy: "original", UpdateBy: "original",
	}
	require.NoError(t, sub.FE2DB())
	require.NoError(t, db.Create(sub).Error)
	params := map[string]string{"user_id": "1", "chat_id": "subscribe-round5", "seq_id": "1", "lang": "en_US"}

	out, err := updateAlertSubscribe(context.Background(), deps, map[string]interface{}{
		"id": "3", "config": `{"note":"maintenance","disabled":1}`,
	}, params)

	require.Empty(t, out)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	require.Contains(t, interrupt.Prompt, "CPU")
	before, err := models.AlertSubscribeGet(deps.DBCtx, "id=?", 3)
	require.NoError(t, err)
	require.Zero(t, before.Disabled)
	require.Empty(t, before.Note)
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))
	params["seq_id"] = "2"

	out, err = updateAlertSubscribe(context.Background(), deps, replay, params)

	require.NoError(t, err)
	var receipt map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(out), &receipt))
	require.Equal(t, true, receipt["applied"])
	require.Equal(t, []interface{}{"disabled", "note"}, receipt["updated"])
	saved, err := models.AlertSubscribeGet(deps.DBCtx, "id=?", 3)
	require.NoError(t, err)
	require.NoError(t, saved.DB2FE())
	require.Equal(t, 1, saved.Disabled)
	require.Equal(t, "maintenance", saved.Note)
	require.Equal(t, []int{1, 2}, saved.SeveritiesJson)
	require.Equal(t, []int64{7}, saved.DatasourceIdsJson)
	require.Equal(t, sub.WebhooksJson, saved.WebhooksJson)
	require.Equal(t, sub.ExtraConfigJson, saved.ExtraConfigJson)
	require.Equal(t, []int64{8}, saved.RuleIds)
	require.Zero(t, saved.RuleId, "the legacy rule_id must not resurrect an obsolete subscription")
	require.Equal(t, int64(1000), saved.CreateAt)
	require.Equal(t, "original", saved.CreateBy)
	require.Equal(t, "root", saved.UpdateBy)
}

func TestUpdateAlertSubscribe_RejectsInvalidPatchesWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   string
	}{
		{name: "missing config", want: "config is required"},
		{name: "malformed", config: "{", want: "invalid config JSON"},
		{name: "identity only", config: `{"id":9}`, want: "no changeable fields"},
		{name: "move group", config: `{"group_id":2}`, want: "group_id"},
		{name: "empty webhooks", config: `{"webhooks":[]}`, want: "webhooks"},
		{name: "wrong type", config: `{"disabled":"yes"}`, want: "invalid config JSON"},
		{name: "empty severities", config: `{"severities":[]}`, want: "severities is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.AlertSubscribe{})
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			require.NoError(t, db.Create(&models.AlertSubscribe{
				Id: 3, GroupId: 1, Name: "Original", Severities: "[1]", UpdateBy: "original",
			}).Error)

			out, err := updateAlertSubscribe(context.Background(), deps, map[string]interface{}{"id": "3", "config": tc.config},
				map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			saved, err := models.AlertSubscribeGet(deps.DBCtx, "id=?", 3)
			require.NoError(t, err)
			require.Equal(t, "Original", saved.Name)
			require.Equal(t, "[1]", saved.Severities)
			require.Equal(t, int64(1), saved.GroupId)
			require.Equal(t, "original", saved.UpdateBy)
		})
	}
}

func TestUpdateAlertSubscribe_RejectsCorruptStoredConfiguration(t *testing.T) {
	deps := newResourceTestDeps(t, &models.AlertSubscribe{})
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.AlertSubscribe{
		Id: 3, GroupId: 1, Name: "Original", DatasourceIds: "[invalid", UpdateBy: "original",
	}).Error)

	out, err := updateAlertSubscribe(context.Background(), deps, map[string]interface{}{
		"id": "3", "config": `{"note":"maintenance"}`,
	}, map[string]string{"user_id": "1"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "failed to parse alert subscribe")
	saved, err := models.AlertSubscribeGet(deps.DBCtx, "id=?", 3)
	require.NoError(t, err)
	require.Equal(t, "[invalid", saved.DatasourceIds)
	require.Empty(t, saved.Note)
	require.Equal(t, "original", saved.UpdateBy)
}
