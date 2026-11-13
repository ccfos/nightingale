package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestDispatchTaskStateless_ProposesScriptWithoutDispatching(t *testing.T) {
	isolatedResourceProposals(t)
	deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.Target{}, &models.TaskRecord{})
	deps.IbexEnabled = true
	db := models.DB(deps.DBCtx)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
	require.NoError(t, db.Create(&models.Target{Ident: "web-1"}).Error)
	script := "#!/bin/sh\necho inspect-only"
	args := map[string]interface{}{
		"busi_group_id": "1", "host": " web-1 ", "script": script, "args": "--verbose", "timeout": float64(-1),
	}
	params := map[string]string{"user_id": "1", "chat_id": "task-round5", "seq_id": "1", "lang": "en_US"}

	out, err := dispatchTaskStateless(context.Background(), deps, args, params)

	require.Empty(t, out)
	var interrupt *aiagent.ToolInterrupt
	require.ErrorAs(t, err, &interrupt)
	require.Contains(t, interrupt.Prompt, "web-1")
	require.Contains(t, interrupt.Prompt, script)
	require.Contains(t, interrupt.Prompt, "`root`")
	require.Contains(t, interrupt.Prompt, "30")
	require.Contains(t, interrupt.Prompt, "--verbose")
	var replay map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(interrupt.ResumeArgs), &replay))
	require.Equal(t, script, replay["script"])
	require.Equal(t, true, replay["confirmed"])
	require.NotEmpty(t, replay["proposal_id"])
	var count int64
	require.NoError(t, db.Model(&models.TaskRecord{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestDispatchTaskStateless_RejectsInvalidRequestsBeforeProposal(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{name: "missing group", args: map[string]interface{}{}, want: "busi_group_id is required"},
		{name: "missing host", args: map[string]interface{}{"busi_group_id": "1"}, want: "host is required"},
		{
			name: "missing script", args: map[string]interface{}{"busi_group_id": "1", "host": "web-1"},
			want: "script is required",
		},
		{
			name: "invalid auth level",
			args: map[string]interface{}{"busi_group_id": "1", "host": "web-1", "script": "echo ok", "auth_level": float64(4)},
			want: "auth_level invalid",
		},
		{
			name: "unknown target", args: map[string]interface{}{"busi_group_id": "1", "host": "absent", "script": "echo ok"},
			want: "targets not exist: absent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deps := newResourceTestDeps(t, &models.BusiGroup{}, &models.Target{}, &models.TaskRecord{})
			deps.IbexEnabled = true
			db := models.DB(deps.DBCtx)
			require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)

			out, err := dispatchTaskStateless(context.Background(), deps, tc.args, map[string]string{"user_id": "1"})

			require.Empty(t, out)
			require.ErrorContains(t, err, tc.want)
			var interrupt *aiagent.ToolInterrupt
			require.NotErrorAs(t, err, &interrupt)
			var count int64
			require.NoError(t, db.Model(&models.TaskRecord{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestGetTaskStatus_ReturnsMetadataWithoutIbex(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TaskRecord{})
	resourceMembership(t, deps, "ro", PermJobTasks)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TaskRecord{
		Id: 3, GroupId: 1, Title: "Inspect CPU", Account: "operator", Timeout: 90, AuthLevel: 2,
		CreateAt: 1000, CreateBy: "root", IbexAuthPass: "private-password",
	}).Error)

	out, err := getTaskStatus(context.Background(), deps, map[string]interface{}{"task_id": "3"},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	require.JSONEq(t, `{"task_id":3,"group_id":1,"title":"Inspect CPU","account":"operator",`+
		`"timeout":90,"auth_level":2,"create_at":1000,"create_by":"root"}`, out)
	require.NotContains(t, out, "private-password")
}

func TestGetTaskStatus_DeniesOtherGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TaskRecord{})
	resourceMembership(t, deps, "ro", PermJobTasks)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TaskRecord{Id: 3, GroupId: 2}).Error)

	out, err := getTaskStatus(context.Background(), deps, map[string]interface{}{"task_id": "3"},
		map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to this task")
}

func TestListTaskRecords_FiltersVisibilityAgeAndAuthorizationLevel(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TaskRecord{})
	resourceMembership(t, deps, "ro", PermJobTasks)
	now := time.Now().Unix()
	// All rows are far from the seven-day cutoff; only their relative age matters.
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.TaskRecord{
		{Id: 1, GroupId: 2, Title: "CPU private", AuthLevel: 2, CreateAt: now - 60},
		{Id: 2, GroupId: 1, Title: "Disk", AuthLevel: 2, CreateAt: now - 120},
		{Id: 3, GroupId: 1, Title: "CPU old", AuthLevel: 2, CreateAt: now - 30*86400},
		{Id: 4, GroupId: 1, Title: "CPU unauthorized level", AuthLevel: 1, CreateAt: now - 180},
		{Id: 5, GroupId: 1, Title: "CPU recent", AuthLevel: 2, CreateAt: now - 240},
		{Id: 6, GroupId: 1, Title: "CPU earlier", AuthLevel: 2, CreateAt: now - 300},
	}).Error)

	out, err := listTaskRecords(context.Background(), deps, map[string]interface{}{
		"query": "CPU", "auth_level": " 2, invalid, , 3 ", "limit": float64(1), "days": float64(0),
	}, map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int                 `json:"total"`
		Items []models.TaskRecord `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 2, result.Total, "total counts matches before pagination")
	require.Len(t, result.Items, 1)
	require.Equal(t, int64(5), result.Items[0].Id)
	require.Equal(t, "CPU recent", result.Items[0].Title)
}

func TestListTaskRecords_DeniesExplicitInaccessibleGroup(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TaskRecord{})
	resourceMembership(t, deps, "ro", PermJobTasks)

	out, err := listTaskRecords(context.Background(), deps, map[string]interface{}{"busi_group_id": "2"},
		map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to busi group 2")
}

func TestListTaskTpls_FiltersGroupAndQueryBeforePagination(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TaskTpl{})
	resourceMembership(t, deps, "ro", PermJobTpls)
	require.NoError(t, models.DB(deps.DBCtx).Create(&[]models.TaskTpl{
		{Id: 1, GroupId: 2, Title: "A CPU private"},
		{Id: 2, GroupId: 1, Title: "B disk"},
		{Id: 3, GroupId: 1, Title: "C CPU inspect", Tags: "diagnostic cpu", Account: "operator", CreateBy: "root"},
		{Id: 4, GroupId: 1, Title: "D CPU inspect"},
	}).Error)

	out, err := listTaskTpls(context.Background(), deps, map[string]interface{}{"query": "CPU", "limit": float64(1)},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result struct {
		Total int             `json:"total"`
		Items []taskTplResult `json:"items"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, 1, result.Total)
	require.Equal(t, []taskTplResult{{
		Id: 3, GroupId: 1, Title: "C CPU inspect", Tags: []string{"diagnostic", "cpu"}, Account: "operator", CreateBy: "root",
	}}, result.Items)
}

func TestGetTaskTplDetail_ReturnsScriptAndExecutionOptions(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TaskTpl{})
	resourceMembership(t, deps, "ro", PermJobTpls)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TaskTpl{
		Id: 3, GroupId: 1, Title: "CPU inspect", Tags: "diagnostic cpu", Batch: 2, Tolerance: 1, Timeout: 90,
		Account: "operator", Args: "--verbose", Script: "echo inspect", CreateBy: "root", UpdateBy: "alice",
	}).Error)

	out, err := getTaskTplDetail(context.Background(), deps, map[string]interface{}{"id": "3"},
		map[string]string{"user_id": "2"})

	require.NoError(t, err)
	var result taskTplDetailResult
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, taskTplDetailResult{
		Id: 3, GroupId: 1, Title: "CPU inspect", Tags: []string{"diagnostic", "cpu"}, Batch: 2, Tolerance: 1, Timeout: 90,
		Account: "operator", Args: "--verbose", Script: "echo inspect", CreateBy: "root", UpdateBy: "alice",
	}, result)
}

func TestGetTaskTplDetail_DeniesOtherGroups(t *testing.T) {
	deps := newResourceTestDeps(t, &models.TaskTpl{})
	resourceMembership(t, deps, "ro", PermJobTpls)
	require.NoError(t, models.DB(deps.DBCtx).Create(&models.TaskTpl{Id: 3, GroupId: 2}).Error)

	out, err := getTaskTplDetail(context.Background(), deps, map[string]interface{}{"id": "3"},
		map[string]string{"user_id": "2"})

	require.Empty(t, out)
	require.ErrorContains(t, err, "no access to this task template")
}
