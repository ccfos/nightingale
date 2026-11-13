package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskLists_FilterCreatorGroupAuthorizationAndPagination(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Router) gin.HandlerFunc
		id      string
	}{
		{name: "group list", handler: func(rt *Router) gin.HandlerFunc { return rt.taskGets }, id: "1"},
		{name: "selected groups", handler: func(rt *Router) gin.HandlerFunc { return rt.taskGetsByGids }, id: "1?gids=1"},
		{name: "my groups", handler: func(rt *Router) gin.HandlerFunc { return rt.taskGetsByGids }, id: "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.TaskRecord{}, &models.BusiGroup{},
				&models.BusiGroupMember{}, &models.UserGroupMember{})
			seedBoardOwnerMembership(t, rt)
			now := time.Now().Unix()
			require.NoError(t, rt.Ctx.DB.Create(&[]models.TaskRecord{
				{Id: 1, GroupId: 1, Title: "diagnose A", CreateBy: "alice", CreateAt: now - 200, AuthLevel: 2},
				{Id: 2, GroupId: 1, Title: "diagnose B", CreateBy: "alice", CreateAt: now - 100, AuthLevel: 2},
				{Id: 3, GroupId: 2, Title: "diagnose C", CreateBy: "alice", CreateAt: now - 50, AuthLevel: 2},
				{Id: 4, GroupId: 1, Title: "diagnose D", CreateBy: "bob", CreateAt: now - 40, AuthLevel: 2},
				{Id: 5, GroupId: 1, Title: "diagnose E", CreateBy: "alice", CreateAt: now - 30, AuthLevel: 1},
				{Id: 6, GroupId: 1, Title: "diagnose F", CreateBy: "alice", CreateAt: 1, AuthLevel: 2},
			}).Error)
			separator := "?"
			if tc.id != "1" {
				separator = "&"
			}

			w := identityRequest(t, rt, tc.handler(rt), nil,
				tc.id+separator+"mine=true&days=7&query=diagnose&auth_level=2&limit=1&p=2", &models.User{Id: 1, Username: "alice"})

			var got struct {
				Total int64               `json:"total"`
				List  []models.TaskRecord `json:"list"`
			}
			require.Empty(t, identityReply(t, w, &got))
			require.Equal(t, int64(2), got.Total)
			require.Len(t, got.List, 1)
			require.Equal(t, int64(1), got.List[0].Id)
		})
	}
}

func TestTaskGetsByGids_RejectsForeignGroupAndHidesTasksFromNonMembers(t *testing.T) {
	rt := identityRouter(t, &models.TaskRecord{}, &models.BusiGroup{},
		&models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	require.NoError(t,
		rt.Ctx.DB.Create(&models.TaskRecord{GroupId: 2, Title: "private", CreateAt: time.Now().Unix()}).Error)

	denied := identityRequest(t, rt, rt.taskGetsByGids, nil, "1?gids=2", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, denied.Code)
	require.NotContains(t, denied.Body.String(), "private")

	empty := identityRequest(t, rt, rt.taskGetsByGids, nil, "1", &models.User{Id: 99})

	var got []int
	require.Empty(t, identityReply(t, empty, &got))
	require.Empty(t, got)
}

func TestTaskRecordAdd_PersistsServiceExecutionMetadata(t *testing.T) {
	rt := identityRouter(t, &models.TaskRecord{})
	form := models.TaskRecord{
		Id: 123, GroupId: 7, Title: "diagnose", Account: "nobody", Script: "echo status", AuthLevel: 2, CreateBy: "service",
	}

	w := identityRequest(t, rt, rt.taskRecordAdd, form, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got models.TaskRecord
	require.NoError(t, rt.Ctx.DB.First(&got, 123).Error)
	require.Equal(t, form, got)
}
