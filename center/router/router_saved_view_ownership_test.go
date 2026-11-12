package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func savedViewOwnerRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func savedViewOwnerRequest(t *testing.T, handler gin.HandlerFunc, user *models.User,
	body interface{}, query string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", user) })
	engine.POST("/views/:id", handler)
	req := httptest.NewRequest(http.MethodPost, "/views/1"+query, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func savedViewOwnerReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
	t.Helper()
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var response struct {
		Err string          `json:"err"`
		Dat json.RawMessage `json:"dat"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	if dat != nil && len(response.Dat) > 0 {
		require.NoError(t, json.Unmarshal(response.Dat, dat))
	}
	return response.Err
}

func TestSavedViewGets_RestrictsVisibilityAndPlacesFavoritesFirst(t *testing.T) {
	rt := savedViewOwnerRouter(t,
		&models.SavedView{},
		&models.UserViewFavorite{},
		&models.UserGroupMember{},
		&models.User{},
	)
	views := []models.SavedView{
		{Id: 1, Name: "mine", Page: "targets", CreateBy: "alice", UpdateAt: 500},
		{Id: 2, Name: "public favorite", Page: "targets", PublicCate: 2, UpdateAt: 100},
		{Id: 3, Name: "team", Page: "targets", PublicCate: 1, Gids: []int64{10}, UpdateAt: 300},
		{Id: 4, Name: "private", Page: "targets", CreateBy: "bob", UpdateAt: 400},
		{Id: 5, Name: "other page", Page: "events", PublicCate: 2, UpdateAt: 600},
	}
	require.NoError(t, rt.Ctx.DB.Create(&views).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserViewFavorite{ViewId: 2, UserId: 1}).Error)

	w := savedViewOwnerRequest(t, rt.savedViewGets, &models.User{Id: 1, Username: "alice"}, nil, "?page=targets")

	var got []models.SavedView
	require.Empty(t, savedViewOwnerReply(t, w, &got))
	require.Len(t, got, 3)
	require.Equal(t, []int64{2, 1, 3}, []int64{got[0].Id, got[1].Id, got[2].Id})
	require.True(t, got[0].IsFavorite)
	require.False(t, got[1].IsFavorite)
}

func TestSavedViewAdd_OverridesCallerIdentity(t *testing.T) {
	rt := savedViewOwnerRouter(t,
		&models.SavedView{},
	)
	view := models.SavedView{Id: 99, Name: " mine ", Page: "targets", CreateBy: "spoofed", Filter: `{"env":"prod"}`}

	w := savedViewOwnerRequest(t, rt.savedViewAdd, &models.User{Username: "alice"}, view, "")

	var id int64
	require.Empty(t, savedViewOwnerReply(t, w, &id))
	var saved models.SavedView
	require.NoError(t, rt.Ctx.DB.First(&saved, id).Error)
	require.NotEqual(t, int64(99), id)
	require.Equal(t, "mine", saved.Name)
	require.Equal(t, "alice", saved.CreateBy)
	require.Equal(t, "alice", saved.UpdateBy)
	require.Equal(t, view.Filter, saved.Filter)
}

func TestSavedViewPut_RequiresCreatorOrAdmin(t *testing.T) {
	cases := []struct {
		name   string
		user   models.User
		status int
	}{
		{name: "creator", user: models.User{Username: "alice"}, status: http.StatusOK},
		{name: "admin", user: models.User{Username: "admin", RolesLst: []string{models.AdminRole}}, status: http.StatusOK},
		{name: "outsider", user: models.User{Username: "bob"}, status: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := savedViewOwnerRouter(t,
				&models.SavedView{},
			)
			original := models.SavedView{Id: 1, Name: "original", Page: "targets", CreateBy: "alice"}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)

			w := savedViewOwnerRequest(t, rt.savedViewPut, &tc.user,
				models.SavedView{Name: "updated", Page: "events", Filter: "new", CreateBy: "spoofed"}, "")

			require.Equal(t, tc.status, w.Code)
			var saved models.SavedView
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, original.Page, saved.Page)
			require.Equal(t, original.CreateBy, saved.CreateBy)
			if tc.status == http.StatusOK {
				require.Empty(t, savedViewOwnerReply(t, w, nil))
				require.Equal(t, "updated", saved.Name)
				require.Equal(t, tc.user.Username, saved.UpdateBy)
			} else {
				require.Equal(t, original.Name, saved.Name)
				require.Empty(t, saved.Filter)
			}
		})
	}
}

func TestSavedViewDelete_EnforcesOwnerAndRemovesFavorites(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		t.Run(fmt.Sprintf("allowed=%t", allowed), func(t *testing.T) {
			rt := savedViewOwnerRouter(t,
				&models.SavedView{},
				&models.UserViewFavorite{},
			)
			require.NoError(t, rt.Ctx.DB.Create(&models.SavedView{
				Id:       1,
				Name:     "mine",
				Page:     "targets",
				CreateBy: "alice",
			}).Error)
			require.NoError(t, rt.Ctx.DB.Create(&models.UserViewFavorite{ViewId: 1, UserId: 2}).Error)
			user := &models.User{Username: "bob"}
			if allowed {
				user.Username = "alice"
			}

			w := savedViewOwnerRequest(t, rt.savedViewDel, user, nil, "")

			var views, favorites int64
			require.NoError(t, rt.Ctx.DB.Model(&models.SavedView{}).Count(&views).Error)
			require.NoError(t, rt.Ctx.DB.Model(&models.UserViewFavorite{}).Count(&favorites).Error)
			if allowed {
				require.Empty(t, savedViewOwnerReply(t, w, nil))
				require.Zero(t, views)
				require.Zero(t, favorites)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Equal(t, int64(1), views)
				require.Equal(t, int64(1), favorites)
			}
		})
	}
}

func TestSavedViewFavoriteDelete_PreservesOtherUsersFavorites(t *testing.T) {
	rt := savedViewOwnerRouter(t,
		&models.SavedView{},
		&models.UserViewFavorite{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.SavedView{Id: 1, Name: "public", Page: "targets", PublicCate: 2}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.UserViewFavorite{ViewId: 1, UserId: 2}).Error)

	w := savedViewOwnerRequest(t, rt.savedViewFavoriteAdd, &models.User{Id: 1}, nil, "")
	require.Empty(t, savedViewOwnerReply(t, w, nil))
	w = savedViewOwnerRequest(t, rt.savedViewFavoriteAdd, &models.User{Id: 1}, nil, "")
	require.Empty(t, savedViewOwnerReply(t, w, nil))
	w = savedViewOwnerRequest(t, rt.savedViewFavoriteDel, &models.User{Id: 1}, nil, "")

	require.Empty(t, savedViewOwnerReply(t, w, nil))
	var favorites []models.UserViewFavorite
	require.NoError(t, rt.Ctx.DB.Find(&favorites).Error)
	require.Len(t, favorites, 1)
	require.Equal(t, int64(2), favorites[0].UserId)
}
