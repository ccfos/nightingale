package router

import (
	"net/http"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestSourceTokenAdd_RejectsInvalidBoardShareWithoutWriting(t *testing.T) {
	cases := []struct {
		name    string
		form    models.SourceToken
		status  int
		message string
	}{
		{name: "unsupported type", form: models.SourceToken{SourceType: "user"}, status: 400, message: "invalid source_type"},
		{
			name: "expired", form: models.SourceToken{SourceType: "board", SourceId: "1", ExpireAt: 1},
			status: 400, message: "expire time must be in the future",
		},
		{
			name: "invalid source id", form: models.SourceToken{SourceType: "board", SourceId: "invalid"},
			status: 400, message: "invalid source_id",
		},
		{
			name: "missing board", form: models.SourceToken{SourceType: "board", SourceId: "404"},
			status: 404, message: "No such dashboard",
		},
		{
			name: "missing expiry", form: models.SourceToken{SourceType: " Board ", SourceId: "1", Note: "share"},
			status: 400, message: "expire time is required",
		},
		{
			name: "excessive expiry",
			form: models.SourceToken{
				SourceType: "board",
				SourceId:   "1",
				ExpireAt:   time.Now().Unix() + 2*maxBoardTokenTTL,
				Note:       "share",
			},
			status: 400, message: "expire time is too far in the future",
		},
		{
			name: "blank note", form: models.SourceToken{SourceType: "board", SourceId: "1", ExpireAt: 1 << 40, Note: " "},
			status: 400, message: "note is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.SourceToken{}, &models.Board{}, &models.BoardPayload{})
			require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "overview"}).Error)
			if tc.name == "blank note" {
				tc.form.ExpireAt = time.Now().Add(24 * time.Hour).Unix()
			}

			w := identityRequest(t, rt, rt.sourceTokenAdd, tc.form, "1", &models.User{RolesLst: []string{"Admin"}})

			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.SourceToken{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestSourceTokenGets_RequiresDashboardOperationAndWritableMembership(t *testing.T) {
	cases := []struct {
		name       string
		user       models.User
		permission bool
		status     int
	}{
		{name: "writer", user: models.User{Id: 1, RolesLst: []string{"Editor"}}, permission: true, status: 200},
		{name: "reader", user: models.User{Id: 2, RolesLst: []string{"Editor"}}, permission: true, status: 403},
		{name: "unbound operation", user: models.User{Id: 1, RolesLst: []string{"Editor"}}, status: 403},
		{name: "admin", user: models.User{Id: 99, RolesLst: []string{"Admin"}}, status: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.SourceToken{}, &models.Board{}, &models.BoardPayload{},
				&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{}, &models.RoleOperation{})
			seedBoardOwnerMembership(t, rt)
			require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 20, PermFlag: "ro"}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 20, UserId: 2}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "overview", GroupId: 1}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&[]models.SourceToken{
				{Id: 1, SourceType: "board", SourceId: "1", Token: "old", CreateAt: 100},
				{Id: 2, SourceType: "board", SourceId: "1", Token: "new", CreateAt: 200},
				{Id: 3, SourceType: "board", SourceId: "2", Token: "foreign"},
			}).Error)
			if tc.permission {
				require.NoError(t, rt.Ctx.DB.Create(&models.RoleOperation{RoleName: "Editor", Operation: "/dashboards/put"}).Error)
			}

			w := identityRequest(t, rt, rt.sourceTokenGets, nil, "1?source_type=Board&source_id=01", &tc.user)

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var got []models.SourceToken
				require.Empty(t, identityReply(t, w, &got))
				require.Len(t, got, 2)
				require.Equal(t, "new", got[0].Token)
				require.Equal(t, "old", got[1].Token)
			} else {
				require.NotContains(t, w.Body.String(), `"token"`)
			}
			var count int64
			require.NoError(t, rt.Ctx.DB.Model(&models.SourceToken{}).Count(&count).Error)
			require.Equal(t, int64(3), count)
		})
	}
}

func TestSourceTokenDel_RevokesBoardShareAndPreservesOtherTokens(t *testing.T) {
	rt := identityRouter(t, &models.SourceToken{}, &models.Board{}, &models.BoardPayload{})
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "overview"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.SourceToken{
		{Id: 1, SourceType: "board", SourceId: "1", Token: "revoked"},
		{Id: 2, SourceType: "event", SourceId: "1", Token: "preserved"},
	}).Error)

	w := identityRequest(t, rt, rt.sourceTokenDel, nil, "1", &models.User{RolesLst: []string{"Admin"}})

	require.Empty(t, identityReply(t, w, nil))
	var got []models.SourceToken
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, "preserved", got[0].Token)
	token, err := models.GetSourceTokenByToken(rt.Ctx, "board", "revoked")
	require.NoError(t, err)
	require.Nil(t, token)
}

func TestSourceTokenDel_DeniesUnownedBoardWithoutRevoking(t *testing.T) {
	rt := identityRouter(t, &models.SourceToken{}, &models.Board{}, &models.BoardPayload{}, &models.RoleOperation{})
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "overview"}).Error)
	require.NoError(t,
		rt.Ctx.DB.Create(&models.SourceToken{Id: 1, SourceType: "board", SourceId: "1", Token: "original"}).Error)

	w := identityRequest(t, rt, rt.sourceTokenDel, nil, "1", &models.User{Id: 2})

	require.Equal(t, http.StatusForbidden, w.Code)
	var saved models.SourceToken
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "original", saved.Token)
}

func TestSourceTokenDel_RejectsEventTokensAndIgnoresMissingIDs(t *testing.T) {
	rt := identityRouter(t, &models.SourceToken{})
	require.NoError(t, rt.Ctx.DB.Create(&models.SourceToken{Id: 1, SourceType: "event", Token: "preserved"}).Error)

	w := identityRequest(t, rt, rt.sourceTokenDel, nil, "1", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "invalid source_type")
	var saved models.SourceToken
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "preserved", saved.Token)
	missing := identityRequest(t, rt, rt.sourceTokenDel, nil, "404", nil)
	require.Empty(t, identityReply(t, missing, nil))
}
