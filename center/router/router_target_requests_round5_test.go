package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestTargetsOfHostQuery_ReturnsOnlyMatchingHosts(t *testing.T) {
	rt := identityRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	seedTargetMutationHosts(t, rt)
	queries := []models.HostQuery{{Key: "hosts", Op: "==", Values: []interface{}{"host-b"}}}

	w := identityRequest(t, rt, rt.targetsOfHostQuery, queries, "0", nil)

	var got []models.Target
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, "host-b", got[0].Ident)
	require.Equal(t, "10.0.0.2", got[0].HostIp)
}

func TestTargetGets_CombinesHostSeparatorsAndReadableGroups(t *testing.T) {
	rt := targetMutationRouter(t, &models.Target{}, &models.TargetBusiGroup{}, &models.BusiGroup{},
		&models.BusiGroupMember{}, &models.UserGroupMember{})
	seedTargetMutationMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Target{
		{Ident: "host-a"}, {Ident: "host-b"}, {Ident: "host-c"}, {Ident: "private"},
	}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.TargetBusiGroup{
		{TargetIdent: "host-a", GroupId: 1}, {TargetIdent: "host-b", GroupId: 1},
		{TargetIdent: "private", GroupId: 2},
	}).Error)
	targetReadRedis(t, rt)

	w := identityRequest(t, rt, rt.targetGets, nil, "0?hosts=host-a,host-b%20host-c%0Aprivate", &models.User{Id: 1})

	var got struct {
		List  []models.Target `json:"list"`
		Total int64           `json:"total"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, int64(3), got.Total)
	require.Len(t, got.List, 3)
	require.Equal(t, []string{"host-a", "host-b", "host-c"}, []string{
		got.List[0].Ident, got.List[1].Ident, got.List[2].Ident,
	})
	// Unassigned hosts stay visible; hosts in an unreadable group are excluded.
	require.Empty(t, got.List[2].GroupIds)
	require.Equal(t, []int64{1}, got.List[0].GroupIds)
}

func TestTargetGets_FillsRedisMetadataAndIgnoresMalformedRecords(t *testing.T) {
	rt := identityRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Target{{Ident: "host-a"}, {Ident: "host-b"}}).Error)
	server := targetReadRedis(t, rt)
	meta, err := json.Marshal(models.HostMeta{Hostname: "host-a", CpuNum: 8, CpuUtil: 31, MemUtil: 42, Offset: 123})
	require.NoError(t, err)
	server.Set(models.WrapIdent("host-a"), string(meta))
	server.Set(models.WrapIdent("host-b"), "invalid JSON")
	user := &models.User{RolesLst: []string{models.AdminRole}}

	w := identityRequest(t, rt, rt.targetGets, nil, "0", user)

	var got struct {
		List  []models.Target `json:"list"`
		Total int64           `json:"total"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, int64(2), got.Total)
	require.Len(t, got.List, 2)
	require.Equal(t, "host-a", got.List[0].Ident)
	require.Equal(t, 8, got.List[0].CpuNum)
	require.Equal(t, float64(31), got.List[0].CpuUtil)
	require.Equal(t, float64(42), got.List[0].MemUtil)
	require.Equal(t, int64(123), got.List[0].Offset)
	require.Equal(t, -1, got.List[1].CpuNum)
}

func TestTaskAdd_DisabledFeatureRejectsBeforePersistence(t *testing.T) {
	rt := identityRouter(t, &models.TaskRecord{})

	w := identityRequest(t, rt, rt.taskAdd, models.TaskForm{}, "1", &models.User{Username: "alice"})

	require.Contains(t, identityReply(t, w, nil), "This functionality has not been enabled")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.TaskRecord{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestTaskAdd_TrimsHostnamesBeforeExistenceCheck(t *testing.T) {
	rt := identityRouter(t, &models.Target{}, &models.TaskRecord{})
	rt.Ibex.Enable = true
	form := models.TaskForm{Hosts: []string{"  missing-host  ", " ", "\t"}, Title: "task"}

	w := identityRequest(t, rt, rt.taskAdd, form, "1", &models.User{Username: "alice"})

	require.Equal(t, "targets not exist: missing-host", identityReply(t, w, nil))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.TaskRecord{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestTaskAdd_InvalidFormDoesNotCreateTaskRecord(t *testing.T) {
	rt := identityRouter(t, &models.Target{}, &models.TaskRecord{})
	rt.Ibex.Enable = true
	require.NoError(t, rt.Ctx.DB.Create(&models.Target{Ident: "host-a"}).Error)
	form := models.TaskForm{Hosts: []string{"host-a"}, Batch: -1}

	w := identityRequest(t, rt, rt.taskAdd, form, "1", &models.User{Username: "alice"})

	require.Equal(t, "arg(batch) should be nonnegative", identityReply(t, w, nil))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.TaskRecord{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestTaskAdd_RejectsUnauthorizedHostBeforeCallingIbex(t *testing.T) {
	rt := identityRouter(t, &models.Target{}, &models.TargetBusiGroup{}, &models.TaskRecord{},
		&models.UserGroupMember{}, &models.BusiGroupMember{})
	rt.Ibex.Enable = true
	require.NoError(t, rt.Ctx.DB.Create(&models.Target{Ident: "private"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.TargetBusiGroup{TargetIdent: "private", GroupId: 9}).Error)
	form := models.TaskForm{Hosts: []string{"private"}, Title: "maintenance", Script: "echo hello", Action: "start"}

	w := identityRequest(t, rt, rt.taskAdd, form, "1", &models.User{Id: 1, Username: "alice"})

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "forbidden")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.TaskRecord{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestUserFindAll_ReturnsProfilesWithoutPasswordHashes(t *testing.T) {
	rt := identityRouter(t, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.User{
		{Username: "alice", Nickname: "Alice", Password: "private-hash", Roles: "Viewer"},
		{Username: "bob", Nickname: "Bob", Password: "other-hash", Roles: "Standard"},
	}).Error)

	w := identityRequest(t, rt, rt.userFindAll, nil, "0", nil)

	var got []models.User
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 2)
	require.ElementsMatch(t, []string{"alice", "bob"}, []string{got[0].Username, got[1].Username})
	require.NotContains(t, w.Body.String(), "private-hash")
	require.NotContains(t, w.Body.String(), "other-hash")
	for _, user := range got {
		require.NotEmpty(t, user.Nickname)
		require.Len(t, user.RolesLst, 1)
	}
}
