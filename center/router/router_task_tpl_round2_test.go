package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskTplAdd_NormalizesHostsAndSortsTags(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.Target{})
	require.NoError(t, rt.Ctx.DB.Table("task_tpl_host").AutoMigrate(&models.TaskTplHost{}))
	rt.Ibex.Enable = true
	require.NoError(t, rt.Ctx.DB.Create(&models.Target{Ident: "host-a"}).Error)
	form := taskTplForm{
		Title: "diagnose", Script: "echo status", Tags: []string{"z", "a"},
		Hosts: []string{"", " host-a ", " "}, AuthLevel: 2, Account: "nobody",
	}

	w := identityRequest(t, rt, rt.taskTplAdd, form, "7", &models.User{Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	var saved models.TaskTpl
	require.NoError(t, rt.Ctx.DB.First(&saved).Error)
	require.Equal(t, "diagnose", saved.Title)
	require.Equal(t, "a z ", saved.Tags)
	require.Equal(t, int64(7), saved.GroupId)
	require.Equal(t, 2, saved.AuthLevel)
	require.Equal(t, "nobody", saved.Account)
	require.Equal(t, "alice", saved.CreateBy)
	require.Equal(t, 30, saved.Timeout)
	var hosts []models.TaskTplHost
	require.NoError(t, rt.Ctx.DB.Table("task_tpl_host").Find(&hosts).Error)
	require.Equal(t, []models.TaskTplHost{{Id: saved.Id, Host: "host-a"}}, hosts)
}

func TestTaskTplAdd_RejectsDisabledFeatureAndInvalidAuthorization(t *testing.T) {
	cases := []struct {
		name      string
		enabled   bool
		authLevel int
		message   string
	}{
		{name: "disabled feature", message: "not been enabled"},
		{name: "negative authorization", enabled: true, authLevel: -1, message: "auth_level invalid"},
		{name: "unknown authorization", enabled: true, authLevel: 4, message: "auth_level invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.TaskTpl{})
			rt.Ibex.Enable = tc.enabled
			form := taskTplForm{Title: "diagnose", Script: "echo status", AuthLevel: tc.authLevel}

			w := identityRequest(t, rt, rt.taskTplAdd, form, "7", &models.User{Username: "alice"})

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.TaskTpl{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestTaskTplAdd_RejectsMissingHostsWithoutSaving(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.Target{})
	rt.Ibex.Enable = true
	form := taskTplForm{Title: "diagnose", Script: "echo status", Hosts: []string{"missing-host"}}

	w := identityRequest(t, rt, rt.taskTplAdd, form, "7", &models.User{Username: "alice"})

	require.Contains(t, identityReply(t, w, nil), "targets not exist: missing-host")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.TaskTpl{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestTaskTplPut_ReplacesEditableFieldsAndHosts(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.Target{})
	require.NoError(t, rt.Ctx.DB.Table("task_tpl_host").AutoMigrate(&models.TaskTplHost{}))
	original := models.TaskTpl{Id: 1, GroupId: 7, Title: "old", Script: "old", CreateBy: "creator", CreateAt: 100}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	require.NoError(t, rt.Ctx.DB.Table("task_tpl_host").Create(&models.TaskTplHost{Id: 1, Host: "old"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.Target{Ident: "new"}).Error)
	form := taskTplForm{
		Title: "updated", Script: "echo updated", Hosts: []string{" new ", ""}, Tags: []string{"b", "a"},
		Batch: 2, Tolerance: 1, Timeout: 60, Pause: "1,2", Args: "arg", Account: "nobody", AuthLevel: 3,
	}
	handler := func(c *gin.Context) { c.Params = append(c.Params, gin.Param{Key: "tid", Value: "1"}); rt.taskTplPut(c) }

	w := identityRequest(t, rt, handler, form, "7", &models.User{Username: "editor"})

	require.Empty(t, identityReply(t, w, nil))
	var saved models.TaskTpl
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, original.CreateBy, saved.CreateBy)
	require.Equal(t, original.CreateAt, saved.CreateAt)
	require.Equal(t, original.GroupId, saved.GroupId)
	require.Equal(t, "editor", saved.UpdateBy)
	require.Equal(t, "a b ", saved.Tags)
	require.Equal(t, form.Title, saved.Title)
	require.Equal(t, form.Script, saved.Script)
	require.Equal(t, form.Batch, saved.Batch)
	require.Equal(t, form.Tolerance, saved.Tolerance)
	require.Equal(t, form.Timeout, saved.Timeout)
	require.Equal(t, form.AuthLevel, saved.AuthLevel)
	var hosts []models.TaskTplHost
	require.NoError(t, rt.Ctx.DB.Table("task_tpl_host").Find(&hosts).Error)
	require.Equal(t, []models.TaskTplHost{{Id: 1, Host: "new"}}, hosts)
}

func TestTaskTplDel_RejectsTemplatesReferencedByAlertRules(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.AlertRule{})
	require.NoError(t, rt.Ctx.DB.Create(&models.TaskTpl{Id: 1, Title: "used"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 9, RuleConfig: `{"tpl_id":1}`}).Error)
	handler := func(c *gin.Context) { c.Params = append(c.Params, gin.Param{Key: "tid", Value: "1"}); rt.taskTplDel(c) }

	w := identityRequest(t, rt, handler, nil, "7", nil)

	require.Contains(t, identityReply(t, w, nil), "used by alert rule ids([9])")
	var saved models.TaskTpl
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "used", saved.Title)
}

func TestTaskTplDel_RemovesTemplateAndHostBindings(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.AlertRule{})
	require.NoError(t, rt.Ctx.DB.Table("task_tpl_host").AutoMigrate(&models.TaskTplHost{}))
	require.NoError(t, rt.Ctx.DB.Create(&models.TaskTpl{Id: 1, Title: "unused"}).Error)
	require.NoError(t, rt.Ctx.DB.Table("task_tpl_host").Create(&models.TaskTplHost{Id: 1, Host: "old"}).Error)
	handler := func(c *gin.Context) { c.Params = append(c.Params, gin.Param{Key: "tid", Value: "1"}); rt.taskTplDel(c) }

	w := identityRequest(t, rt, handler, nil, "7", nil)

	require.Empty(t, identityReply(t, w, nil))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.TaskTpl{}).Count(&count).Error)
	require.Zero(t, count)
	require.NoError(t, rt.Ctx.DB.Table("task_tpl_host").Count(&count).Error)
	require.Zero(t, count)
}

func TestTaskTplTags_BindsAndUnbindsTrimmedTags(t *testing.T) {
	cases := []struct {
		name     string
		handler  func(*Router) gin.HandlerFunc
		tags     []string
		expected string
	}{
		{
			name: "bind", handler: func(rt *Router) gin.HandlerFunc { return rt.taskTplBindTags },
			tags: []string{" b ", "a", ""}, expected: "a b c ",
		},
		{
			name: "unbind", handler: func(rt *Router) gin.HandlerFunc { return rt.taskTplUnbindTags },
			tags: []string{" c ", ""}, expected: "a ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.TaskTpl{})
			require.NoError(t, rt.Ctx.DB.Create(&models.TaskTpl{Id: 1, Tags: "a c "}).Error)

			w := identityRequest(t, rt, tc.handler(rt), tplTagsForm{Ids: []int64{404, 1}, Tags: tc.tags}, "7", nil)

			require.Empty(t, identityReply(t, w, nil))
			var got models.TaskTpl
			require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
			require.Equal(t, tc.expected, got.Tags)
			require.Equal(t, "operator", got.UpdateBy)
		})
	}
}

func TestTaskTplGets_FiltersGroupQueryAndAuthorizationLevels(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.TaskTpl{
		{Id: 1, GroupId: 7, Title: "diagnose A", AuthLevel: 2, Tags: "cpu "},
		{Id: 2, GroupId: 7, Title: "diagnose B", AuthLevel: 2},
		{Id: 3, GroupId: 7, Title: "diagnose C", AuthLevel: 1},
		{Id: 4, GroupId: 8, Title: "diagnose D", AuthLevel: 2},
	}).Error)

	w := identityRequest(t, rt, rt.taskTplGets, nil, "7?query=diagnose&auth_level=2,,invalid&limit=1&p=2", nil)

	var got struct {
		Total int64            `json:"total"`
		List  []models.TaskTpl `json:"list"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, int64(2), got.Total)
	require.Len(t, got.List, 1)
	require.Equal(t, int64(2), got.List[0].Id)
}

func TestTaskTplGetsByGids_RejectsUnrelatedGroups(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)

	w := identityRequest(t, rt, rt.taskTplGetsByGids, nil, "1?gids=2", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestTaskTplGetsByGids_NoMembershipReturnsEmptyList(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
	require.NoError(t, rt.Ctx.DB.Create(&models.TaskTpl{GroupId: 7, Title: "hidden"}).Error)

	w := identityRequest(t, rt, rt.taskTplGetsByGids, nil, "1", &models.User{Id: 99})

	var got []int
	require.Empty(t, identityReply(t, w, &got))
	require.Empty(t, got)
}

func TestTaskTplGetByService_ReturnsTemplateOrNotFound(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{})
	require.NoError(t,
		rt.Ctx.DB.Create(&models.TaskTpl{Id: 1, Title: "diagnose", Script: "echo status", AuthLevel: 2}).Error)
	for _, id := range []string{"1", "404"} {
		t.Run(id, func(t *testing.T) {
			handler := func(c *gin.Context) {
				c.Params = append(c.Params, gin.Param{Key: "tid", Value: id})
				rt.taskTplGetByService(c)
			}

			w := identityRequest(t, rt, handler, nil, "7", nil)

			if id == "404" {
				require.Equal(t, http.StatusNotFound, w.Code)
				require.Contains(t, w.Body.String(), "no such task template")
			} else {
				var got models.TaskTpl
				require.Empty(t, identityReply(t, w, &got))
				require.Equal(t, "diagnose", got.Title)
				require.Equal(t, "echo status", got.Script)
				require.Equal(t, 2, got.AuthLevel)
			}
		})
	}
}

func TestTaskTplGetsByGids_ListsOnlySelectedReadableGroups(t *testing.T) {
	rt := identityRouter(t, &models.TaskTpl{}, &models.User{}, &models.BusiGroup{},
		&models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.TaskTpl{
		{Id: 1, GroupId: 1, Title: "diagnose", AuthLevel: 2},
		{Id: 2, GroupId: 2, Title: "private", AuthLevel: 2},
	}).Error)

	w := identityRequest(t, rt, rt.taskTplGetsByGids, nil, "1?gids=1&auth_level=2", &models.User{Id: 1})

	var got struct {
		Total int64            `json:"total"`
		List  []models.TaskTpl `json:"list"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, int64(1), got.Total)
	require.Len(t, got.List, 1)
	require.Equal(t, "diagnose", got.List[0].Title)
}
