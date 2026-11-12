package router

import (
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeNotifyRules_AllowsTeamMembersAndAdmins(t *testing.T) {
	cases := []struct {
		name   string
		groups map[int64]struct{}
		admin  bool
	}{
		{name: "member", groups: map[int64]struct{}{10: {}}},
		{name: "admin", admin: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.NotifyRule{})
			require.NoError(t, rt.Ctx.DB.Create(&models.NotifyRule{
				ID: 1, Name: "oncall", Enable: true, UserGroupIds: []int64{20, 10},
			}).Error)
			config := &models.AlertRule{NotifyVersion: 1, NotifyRuleIds: []int64{404, 1}}

			got := rt.authorizeNotifyRules(config, tc.groups, tc.admin)

			require.Len(t, got, 1)
			require.Equal(t, "oncall", got[1].Name)
			require.Equal(t, []int64{20, 10}, got[1].UserGroupIds)
		})
	}
}

func TestAuthorizeNotifyRules_RejectsForeignTeamWithoutChangingRule(t *testing.T) {
	rt := identityRouter(t, &models.NotifyRule{})
	original := models.NotifyRule{ID: 1, Name: "private", Enable: true, UserGroupIds: []int64{20}}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	handler := func(c *gin.Context) {
		rt.authorizeNotifyRules(&models.AlertRule{NotifyVersion: 1, NotifyRuleIds: []int64{1}}, map[int64]struct{}{10: {}}, false)
		t.Error("foreign notification rule passed authorization")
	}

	w := identityRequest(t, rt, handler, nil, "0", nil)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "forbidden notify rule: 1")
	var saved models.NotifyRule
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, original.Name, saved.Name)
	require.Equal(t, original.UserGroupIds, saved.UserGroupIds)
	require.Equal(t, original.Enable, saved.Enable)
}

func TestAuthorizePipelines_EnforcesTeamOwnership(t *testing.T) {
	cases := []struct {
		name    string
		user    models.User
		admin   bool
		allowed bool
	}{
		{name: "member", user: models.User{Id: 1}, allowed: true},
		{name: "outsider", user: models.User{Id: 2}},
		{name: "admin", user: models.User{Id: 2}, admin: true, allowed: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.EventPipeline{}, &models.UserGroupMember{})
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.EventPipeline{ID: 1, Name: "private", TeamIds: []int64{10}}).Error)
			handler := func(c *gin.Context) {
				got := rt.authorizePipelines(&models.AlertRule{
					PipelineConfigs: []models.PipelineConfig{{PipelineId: 1, Enable: true}},
				}, 9, tc.admin, &tc.user)
				c.JSON(http.StatusOK, got)
			}

			w := identityRequest(t, rt, handler, nil, "0", &tc.user)

			if tc.allowed {
				require.Equal(t, http.StatusOK, w.Code)
				require.Contains(t, w.Body.String(), `"name":"private"`)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Contains(t, w.Body.String(), "forbidden event pipeline: 1")
			}
			var saved models.EventPipeline
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, []int64{10}, saved.TeamIds)
		})
	}
}

func TestCheckBusiGroupPerm_ReturnsPermissionDecision(t *testing.T) {
	cases := []struct {
		name    string
		uid     int64
		perm    string
		allowed bool
	}{
		{name: "reader read", uid: 1, perm: "ro", allowed: true},
		{name: "reader write", uid: 1, perm: "rw"},
		{name: "writer write", uid: 2, perm: "rw", allowed: true},
		{name: "outsider read", uid: 3, perm: "ro"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := permissionRouter(t, &models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
			seedPermissionMembership(t, rt)
			handler := func(c *gin.Context) {
				c.Params = append(c.Params, gin.Param{Key: "perm", Value: tc.perm})
				rt.checkBusiGroupPerm(c)
			}

			w := identityRequest(t, rt, handler, nil, "10", &models.User{Id: tc.uid})

			var allowed bool
			require.Empty(t, identityReply(t, w, &allowed))
			require.Equal(t, tc.allowed, allowed)
		})
	}
}
