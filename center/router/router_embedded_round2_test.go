package router

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedProductGets_FiltersPrivateProductsByTeam(t *testing.T) {
	cases := []struct {
		name     string
		user     models.User
		expected []int64
	}{
		{name: "member", user: models.User{Id: 1}, expected: []int64{2, 1}},
		{name: "outsider", user: models.User{Id: 2}, expected: []int64{1}},
		{name: "admin", user: models.User{RolesLst: []string{"Admin"}}, expected: []int64{2, 3, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.EmbeddedProduct{}, &models.User{}, &models.UserGroupMember{})
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&[]models.EmbeddedProduct{
				{ID: 1, Name: "public", Weight: 10},
				{ID: 2, Name: "team", IsPrivate: true, TeamIDs: []int64{10}, Weight: 1},
				{ID: 3, Name: "foreign", IsPrivate: true, TeamIDs: []int64{20}, Weight: 5},
			}).Error)

			w := identityRequest(t, rt, rt.embeddedProductGets, nil, "1", &tc.user)

			var got []models.EmbeddedProduct
			require.Empty(t, identityReply(t, w, &got))
			ids := make([]int64, 0, len(got))
			for _, product := range got {
				ids = append(ids, product.ID)
			}
			require.Equal(t, tc.expected, ids)
		})
	}
}

func TestEmbeddedProductGet_RequiresTeamMembershipForPrivateProducts(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		status int
	}{
		{name: "member", user: models.User{Id: 1}, status: 200},
		{name: "outsider", user: models.User{Id: 2}, status: 403},
		{name: "admin", user: models.User{RolesLst: []string{"Admin"}}, status: 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.EmbeddedProduct{}, &models.UserGroupMember{})
			require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{UserId: 1, GroupId: 10}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.EmbeddedProduct{
				ID: 1, Name: "private", URL: "https://example.invalid/private", IsPrivate: true, TeamIDs: []int64{10},
			}).Error)

			w := identityRequest(t, rt, rt.embeddedProductGet, nil, "1", &tc.user)

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				var got models.EmbeddedProduct
				require.Empty(t, identityReply(t, w, &got))
				require.Equal(t, "private", got.Name)
				require.Equal(t, []int64{10}, got.TeamIDs)
			} else {
				require.NotContains(t, w.Body.String(), "https://example.invalid/private")
			}
		})
	}
}

func TestEmbeddedProductAdd_UsesAuthenticatedCreator(t *testing.T) {
	rt := identityRouter(t, &models.EmbeddedProduct{})
	body := []models.EmbeddedProduct{
		{Name: "overview", URL: "https://example.invalid", CreateBy: "forged", UpdateBy: "forged"},
	}

	w := identityRequest(t, rt, rt.embeddedProductAdd, body, "1", &models.User{Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	var got models.EmbeddedProduct
	require.NoError(t, rt.Ctx.DB.First(&got).Error)
	require.Equal(t, "overview", got.Name)
	require.Equal(t, body[0].URL, got.URL)
	require.Equal(t, "alice", got.CreateBy)
	require.Equal(t, "alice", got.UpdateBy)
}

func TestEmbeddedProductAdd_InvalidBatchIsNotPartiallySaved(t *testing.T) {
	rt := identityRouter(t, &models.EmbeddedProduct{})
	body := []models.EmbeddedProduct{
		{Name: "valid", URL: "https://example.invalid"},
		{Name: "private", URL: "https://example.invalid", IsPrivate: true},
	}

	w := identityRequest(t, rt, rt.embeddedProductAdd, body, "1", &models.User{Username: "alice"})

	require.Contains(t, identityReply(t, w, nil), "TeamIDs is blank")
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.EmbeddedProduct{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestEmbeddedProductPut_UpdatesEditableFieldsAndPreservesCreation(t *testing.T) {
	rt := identityRouter(t, &models.EmbeddedProduct{})
	require.NoError(t, rt.Ctx.DB.Create(&models.EmbeddedProduct{
		ID: 1, Name: "old", URL: "https://example.invalid/old", CreateBy: "creator", CreateAt: 100,
	}).Error)
	form := models.EmbeddedProduct{
		ID: 99, Name: "updated", URL: "https://example.invalid/new", IsPrivate: true,
		Hide: true, TeamIDs: []int64{10}, Weight: 7, CreateBy: "forged",
	}

	w := identityRequest(t, rt, rt.embeddedProductPut, form, "1", &models.User{Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	var got models.EmbeddedProduct
	require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
	require.Equal(t, form.Name, got.Name)
	require.Equal(t, form.URL, got.URL)
	require.Equal(t, form.TeamIDs, got.TeamIDs)
	require.Equal(t, form.Weight, got.Weight)
	require.True(t, got.Hide)
	require.True(t, got.IsPrivate)
	require.Equal(t, "creator", got.CreateBy)
	require.Equal(t, int64(100), got.CreateAt)
	require.Equal(t, "alice", got.UpdateBy)
}

func TestEmbeddedProductHidePut_ChangesOnlyVisibilityAndUpdateAudit(t *testing.T) {
	rt := identityRouter(t, &models.EmbeddedProduct{})
	original := models.EmbeddedProduct{
		ID: 1, Name: "overview", URL: "https://example.invalid", TeamIDs: []int64{10}, Weight: 9, CreateBy: "creator",
	}
	require.NoError(t, rt.Ctx.DB.Create(&original).Error)
	for _, hide := range []bool{true, false} {
		w := identityRequest(t, rt, rt.embeddedProductHidePut,
			map[string]bool{"hide": hide}, "1", &models.User{Username: "alice"})

		require.Empty(t, identityReply(t, w, nil))
		var got models.EmbeddedProduct
		require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
		require.Equal(t, hide, got.Hide)
		require.Equal(t, "alice", got.UpdateBy)
		require.Equal(t, original.Name, got.Name)
		require.Equal(t, original.URL, got.URL)
		require.Equal(t, original.TeamIDs, got.TeamIDs)
		require.Equal(t, original.Weight, got.Weight)
		require.Equal(t, original.CreateBy, got.CreateBy)
	}
}

func TestEmbeddedProductWeightsPut_UpdatesOnlySelectedWeights(t *testing.T) {
	rt := identityRouter(t, &models.EmbeddedProduct{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.EmbeddedProduct{
		{ID: 1, Name: "first", Weight: 10}, {ID: 2, Name: "second", Weight: 20}, {ID: 3, Name: "third", Weight: 30},
	}).Error)
	body := []map[string]interface{}{{"id": 1, "weight": 0}, {"id": 2, "weight": 2}}

	w := identityRequest(t, rt, rt.embeddedProductWeightsPut, body, "1", &models.User{Username: "alice"})

	require.Empty(t, identityReply(t, w, nil))
	var got []models.EmbeddedProduct
	require.NoError(t, rt.Ctx.DB.Order("id").Find(&got).Error)
	require.Equal(t, 0, got[0].Weight)
	require.Equal(t, 2, got[1].Weight)
	require.Equal(t, 30, got[2].Weight)
	require.Equal(t, "alice", got[0].UpdateBy)
	require.Equal(t, "first", got[0].Name)
	require.Equal(t, "second", got[1].Name)
}

func TestEmbeddedProductWeightsPut_ValidatesEntireBatchBeforeWriting(t *testing.T) {
	cases := []struct {
		name string
		body []map[string]interface{}
	}{
		{name: "invalid id", body: []map[string]interface{}{{"id": 1, "weight": 0}, {"id": 0, "weight": 2}}},
		{name: "oversized batch", body: make([]map[string]interface{}, 1001)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := identityRouter(t, &models.EmbeddedProduct{})
			require.NoError(t, rt.Ctx.DB.Create(&models.EmbeddedProduct{ID: 1, Name: "original", Weight: 10}).Error)

			w := identityRequest(t, rt, rt.embeddedProductWeightsPut, tc.body, "1", &models.User{Username: "alice"})

			require.Equal(t, http.StatusBadRequest, w.Code)
			var got models.EmbeddedProduct
			require.NoError(t, rt.Ctx.DB.First(&got, 1).Error)
			require.Equal(t, 10, got.Weight)
			require.Empty(t, got.UpdateBy)
		})
	}
}

func TestEmbeddedProductDelete_PreservesUnselectedProducts(t *testing.T) {
	rt := identityRouter(t, &models.EmbeddedProduct{})
	require.NoError(t, rt.Ctx.DB.Create(&[]models.EmbeddedProduct{{ID: 1, Name: "first"}, {ID: 2, Name: "second"}}).Error)

	w := identityRequest(t, rt, rt.embeddedProductDelete, nil, "1", nil)

	require.Empty(t, identityReply(t, w, nil))
	var got []models.EmbeddedProduct
	require.NoError(t, rt.Ctx.DB.Find(&got).Error)
	require.Len(t, got, 1)
	require.Equal(t, "second", got[0].Name)
}

func TestEmbeddedProductHandlers_RejectNonPositiveIDs(t *testing.T) {
	rt := identityRouter(t, &models.EmbeddedProduct{})
	cases := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "get", handler: rt.embeddedProductGet},
		{name: "update", handler: rt.embeddedProductPut},
		{name: "hide", handler: rt.embeddedProductHidePut},
		{name: "delete", handler: rt.embeddedProductDelete},
	}
	for _, tc := range cases {
		for _, id := range []int64{0, -1} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, id), func(t *testing.T) {
				w := identityRequest(t, rt, tc.handler, models.EmbeddedProduct{}, fmt.Sprint(id), nil)

				require.Equal(t, http.StatusBadRequest, w.Code)
				require.Contains(t, w.Body.String(), "invalid id")
			})
		}
	}
}
