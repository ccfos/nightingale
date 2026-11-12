package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBoardGet_ResolvesIdentAndNumericIDForAnonymousBoards(t *testing.T) {
	rt := boardOwnerRouter(t, &models.Board{}, &models.BoardPayload{})
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{
		Id: 7, Ident: "overview", Name: "Overview", Public: 1, PublicCate: models.PublicAnonymous,
	}).Error)
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 7, `{"panels":[{"id":1}]}`))
	for _, bid := range []string{"overview", "7", "missing"} {
		t.Run(bid, func(t *testing.T) {
			w := boardOwnerRequest(t, rt.boardGet, nil, "/groups/1/boards/"+bid, nil)

			if bid == "missing" {
				require.Equal(t, http.StatusNotFound, w.Code)
				require.Contains(t, w.Body.String(), "No such dashboard")
			} else {
				var got models.Board
				require.Empty(t, boardOwnerReply(t, w, &got))
				require.Equal(t, int64(7), got.Id)
				require.JSONEq(t, `{"panels":[{"id":1}]}`, got.Configs)
			}
		})
	}
}

func TestBoardGet_ShareTokenIsRestrictedToItsBoard(t *testing.T) {
	cases := []struct {
		name     string
		sourceID string
		expired  bool
		status   int
	}{
		{name: "matching", sourceID: "1", status: 200},
		{name: "different board", sourceID: "2", status: 403},
		{name: "expired fallback to public", sourceID: "2", expired: true, status: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := boardOwnerRouter(t, &models.Board{}, &models.BoardPayload{}, &models.SourceToken{})
			require.NoError(t, rt.Ctx.DB.Create(&models.Board{
				Id: 1, Name: "overview", Public: 1, PublicCate: models.PublicAnonymous,
			}).Error)
			expiry := time.Now().Add(24 * time.Hour).Unix()
			if tc.expired {
				expiry = 1
			}
			require.NoError(t, rt.Ctx.DB.Create(&models.SourceToken{
				SourceType: "board", SourceId: tc.sourceID, Token: "share", ExpireAt: expiry,
			}).Error)
			require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"secret":"panels"}`))

			w := boardOwnerRequest(t, rt.boardGet, nil, "/groups/1/boards/1?__token=share", nil)

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var got models.Board
				require.Empty(t, boardOwnerReply(t, w, &got))
				require.Equal(t, int64(1), got.Id)
				require.JSONEq(t, `{"secret":"panels"}`, got.Configs)
			} else {
				require.Contains(t, w.Body.String(), "share link does not match")
				require.NotContains(t, w.Body.String(), "secret")
			}
		})
	}
}

func TestBoardGet_LoginOnlyBoardRejectsMissingAuthentication(t *testing.T) {
	rt := boardOwnerRouter(t, &models.Board{}, &models.BoardPayload{})
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{
		Id: 1, Name: "overview", Public: 1, PublicCate: models.PublicLogin,
	}).Error)

	w := boardOwnerRequest(t, rt.boardGet, nil, "/groups/1/boards/1", nil)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.NotContains(t, w.Body.String(), "configs")
}

func TestBoardBatchClone_CopiesPayloadIntoWritableDestinations(t *testing.T) {
	rt := boardOwnerRouter(t, &models.Board{}, &models.BoardPayload{}, &models.BusiGroup{},
		&models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "overview", Ident: "source", GroupId: 2}).Error)
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"panels":[1]}`))

	w := boardOwnerRequest(t, rt.boardBatchClone, boardsForm{BoardIds: []int64{1}, Bgids: []int64{1}},
		"/groups/1/boards/1", &models.User{Id: 1, Username: "alice"})

	var failures map[string]string
	require.Empty(t, boardOwnerReply(t, w, &failures))
	require.Empty(t, failures)
	var clone models.Board
	require.NoError(t, rt.Ctx.DB.Where("id <> ?", 1).First(&clone).Error)
	require.Equal(t, int64(1), clone.GroupId)
	require.Equal(t, "overview", clone.Name)
	require.NotEqual(t, "source", clone.Ident)
	require.Equal(t, "alice", clone.CreateBy)
	payload, err := models.BoardPayloadGet(rt.Ctx, clone.Id)
	require.NoError(t, err)
	require.JSONEq(t, `{"panels":[1]}`, payload)
}

func TestBoardGets_FiltersGroupAndQuery(t *testing.T) {
	rt := identityRouter(t, &models.Board{}, &models.User{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Board{
		{Id: 1, GroupId: 1, Name: "CPU overview"}, {Id: 2, GroupId: 1, Name: "Memory"},
		{Id: 3, GroupId: 2, Name: "CPU restricted"},
	}).Error)

	w := identityRequest(t, rt, rt.boardGets, nil, "1?query=CPU", nil)

	var got []models.Board
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
}

func TestPublicBoardGets_IncludesSharedGroupsAndPublicBoards(t *testing.T) {
	rt := identityRouter(t, &models.Board{}, &models.BoardBusigroup{}, &models.User{},
		&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Board{
		{Id: 1, Public: 1, PublicCate: models.PublicAnonymous, Name: "anonymous"},
		{Id: 2, Public: 1, PublicCate: models.PublicLogin, Name: "login"},
		{Id: 3, Public: 1, PublicCate: models.PublicBusi, Name: "shared"},
		{Id: 4, Public: 1, PublicCate: models.PublicBusi, Name: "foreign"},
		{Id: 5, Public: 0, Name: "private"},
	}).Error)
	require.NoError(t, models.BoardBusigroupUpdate(rt.Ctx, 3, []int64{1}))
	require.NoError(t, models.BoardBusigroupUpdate(rt.Ctx, 4, []int64{2}))

	w := identityRequest(t, rt, rt.publicBoardGets, nil, "1", &models.User{Id: 1})

	var got []models.Board
	require.Empty(t, identityReply(t, w, &got))
	ids := make([]int64, 0, len(got))
	for _, board := range got {
		ids = append(ids, board.Id)
	}
	require.ElementsMatch(t, []int64{1, 2, 3}, ids)
}

func TestDashAnnotationGets_SelectsOverlappingTimeRange(t *testing.T) {
	rt := identityRouter(t, &models.DashAnnotation{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.DashAnnotation{
		{Id: 1, DashboardId: 1, TimeStart: 50, TimeEnd: 150, Tags: `["deploy"]`},
		{Id: 2, DashboardId: 1, TimeStart: 250, TimeEnd: 300, Tags: `[]`},
		{Id: 3, DashboardId: 2, TimeStart: 100, TimeEnd: 200, Tags: `[]`},
	}).Error)

	w := identityRequest(t, rt, rt.dashAnnotationGets, nil, "1?dashboard_id=1&from=100&to=200&limit=10", nil)

	var got []models.DashAnnotation
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	require.Equal(t, []string{"deploy"}, got[0].TagsJSON)
}

func TestDashAnnotationMutations_RejectReadOnlyUsersWithoutWriting(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Router) gin.HandlerFunc
	}{
		{name: "add", handler: func(rt *Router) gin.HandlerFunc { return rt.dashAnnotationAdd }},
		{name: "update", handler: func(rt *Router) gin.HandlerFunc { return rt.dashAnnotationPut }},
		{name: "delete", handler: func(rt *Router) gin.HandlerFunc { return rt.dashAnnotationDel }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.DashAnnotation{}, &models.Board{}, &models.BoardPayload{},
				&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
			seedBoardOwnerMembership(t, rt)
			require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, GroupId: 1}).Error)
			original := models.DashAnnotation{Id: 1, DashboardId: 1, Description: "original", Tags: `[]`}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)

			w := identityRequest(t, rt, tc.handler(rt), models.DashAnnotation{DashboardId: 1, Description: "changed"},
				"1", &models.User{Id: 2})

			require.Equal(t, http.StatusForbidden, w.Code)
			var got []models.DashAnnotation
			require.NoError(t, rt.Ctx.DB.Find(&got).Error)
			require.Equal(t, []models.DashAnnotation{original}, got)
		})
	}
}

func TestDashAnnotationPut_UpdatesContentAndPreservesCreation(t *testing.T) {
	rt := identityRouter(t, &models.DashAnnotation{}, &models.Board{}, &models.BoardPayload{},
		&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, GroupId: 1}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.DashAnnotation{
		Id: 1, DashboardId: 1, Description: "old", Tags: `[]`, CreateBy: "creator", CreateAt: 100,
	}).Error)
	form := models.DashAnnotation{
		Id: 999, DashboardId: 1, PanelId: "p1", Description: "deploy", TagsJSON: []string{"release"},
		TimeStart: 100, TimeEnd: 200, Config: "{}", CreateBy: "forged",
	}

	w := identityRequest(t, rt, rt.dashAnnotationPut, form, "1", &models.User{Id: 1})

	require.Empty(t, identityReply(t, w, nil))
	got, err := models.DashAnnotationGet(rt.Ctx, "id = ?", 1)
	require.NoError(t, err)
	require.Equal(t, form.Description, got.Description)
	require.Equal(t, form.TagsJSON, got.TagsJSON)
	require.Equal(t, form.PanelId, got.PanelId)
	require.Equal(t, form.TimeStart, got.TimeStart)
	require.Equal(t, form.TimeEnd, got.TimeEnd)
	require.Equal(t, "creator", got.CreateBy)
	require.Equal(t, int64(100), got.CreateAt)
	require.Equal(t, "operator", got.UpdateBy)
}

func TestDashAnnotationDel_DeletesOnlyAuthorizedAnnotation(t *testing.T) {
	rt := identityRouter(t, &models.DashAnnotation{}, &models.Board{}, &models.BoardPayload{},
		&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, GroupId: 1}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.DashAnnotation{
		{Id: 1, DashboardId: 1, Tags: `[]`}, {Id: 2, DashboardId: 1, Tags: `[]`},
	}).Error)

	w := identityRequest(t, rt, rt.dashAnnotationDel, nil, "1", &models.User{Id: 1})

	require.Empty(t, identityReply(t, w, nil))
	var got []models.DashAnnotation
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, int64(2), got[0].Id)
}

func TestBoardGet_PrivateBoardRequiresMembershipOrAdmin(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		status int
	}{
		{name: "member", user: models.User{Id: 1, Username: "member"}, status: 200},
		{name: "outsider", user: models.User{Id: 2, Username: "outsider"}, status: 403},
		{name: "admin", user: models.User{Id: 3, Username: "admin", Roles: "Admin"}, status: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := boardOwnerRouter(t, &models.Board{}, &models.BoardPayload{}, &models.User{},
				&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
			seedBoardOwnerMembership(t, rt)
			rt.UserCache = &memsto.UserCacheType{}
			rt.HTTP.ProxyAuth.Enable = true
			rt.HTTP.ProxyAuth.HeaderUserNameKey = "X-Test-User"
			require.NoError(t, rt.Ctx.DB.Create(&tc.user).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, GroupId: 1, Name: "restricted"}).Error)
			require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"private":true}`))
			handler := func(c *gin.Context) { c.Request.Header.Set("X-Test-User", tc.user.Username); rt.boardGet(c) }

			w := boardOwnerRequest(t, handler, nil, "/groups/1/boards/1", nil)

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var got models.Board
				require.Empty(t, boardOwnerReply(t, w, &got))
				require.JSONEq(t, `{"private":true}`, got.Configs)
			} else {
				require.NotContains(t, w.Body.String(), "private")
			}
		})
	}
}

func TestBoardGet_GroupSharedBoardRequiresMatchingSharingGroup(t *testing.T) {
	cases := []struct {
		name        string
		userID      int64
		sharedGroup int64
		status      int
	}{
		{name: "shared with membership", userID: 1, sharedGroup: 1, status: 200},
		{name: "different sharing group", userID: 1, sharedGroup: 2, status: 403},
		{name: "no memberships", userID: 2, sharedGroup: 1, status: 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := boardOwnerRouter(t, &models.Board{}, &models.BoardPayload{}, &models.BoardBusigroup{}, &models.User{},
				&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
			seedBoardOwnerMembership(t, rt)
			rt.UserCache = &memsto.UserCacheType{}
			rt.HTTP.ProxyAuth.Enable = true
			rt.HTTP.ProxyAuth.HeaderUserNameKey = "X-Test-User"
			require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: tc.userID, Username: "viewer"}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.Board{
				Id: 1, GroupId: 2, Name: "shared", Public: 1, PublicCate: models.PublicBusi,
			}).Error)
			require.NoError(t, models.BoardBusigroupUpdate(rt.Ctx, 1, []int64{tc.sharedGroup}))
			require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"panels":[1]}`))
			handler := func(c *gin.Context) { c.Request.Header.Set("X-Test-User", "viewer"); rt.boardGet(c) }

			w := boardOwnerRequest(t, handler, nil, "/groups/1/boards/1", nil)

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var got models.Board
				require.Empty(t, boardOwnerReply(t, w, &got))
				require.Equal(t, int64(1), got.Id)
				require.JSONEq(t, `{"panels":[1]}`, got.Configs)
			} else {
				require.NotContains(t, w.Body.String(), "panels")
			}
		})
	}
}
