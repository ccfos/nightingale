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

func boardOwnerRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(tables...))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func seedBoardOwnerMembership(t *testing.T, rt *Router) {
	t.Helper()
	db := rt.Ctx.DB
	require.NoError(t, db.Create(&models.BusiGroup{Id: 1, Name: "production"}).Error)
	require.NoError(t, db.Create(&models.BusiGroup{Id: 2, Name: "restricted"}).Error)
	require.NoError(t, db.Create(&models.BusiGroupMember{BusiGroupId: 1, UserGroupId: 10, PermFlag: "rw"}).Error)
	require.NoError(t, db.Create(&models.UserGroupMember{GroupId: 10, UserId: 1}).Error)
}

func boardOwnerRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, path string,
	user *models.User) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Use(func(c *gin.Context) { c.Set("user", user) })
	engine.POST("/groups/:id/boards/:bid", handler)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func boardOwnerReply(t *testing.T, w *httptest.ResponseRecorder, dat interface{}) string {
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

func TestBoardAdd_SavesPayloadAndAuthenticatedCreator(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
	)
	form := boardForm{Name: "overview", Ident: "overview", Configs: `{"panels":[]}`}

	w := boardOwnerRequest(t, rt.boardAdd, form, "/groups/1/boards/0", &models.User{Username: "editor"})

	var board models.Board
	require.Empty(t, boardOwnerReply(t, w, &board))
	require.Equal(t, int64(1), board.GroupId)
	require.Equal(t, "editor", board.CreateBy)
	payload, err := models.BoardPayloadGet(rt.Ctx, board.Id)
	require.NoError(t, err)
	require.JSONEq(t, form.Configs, payload)
}

func TestBoardPut_EnforcesStoredGroupOwnership(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(fmt.Sprintf("allowed=%t", allowed), func(t *testing.T) {
			rt := boardOwnerRouter(t,
				&models.Board{},
				&models.BoardPayload{},
				&models.BusiGroup{},
				&models.BusiGroupMember{},
				&models.UserGroupMember{},
			)
			seedBoardOwnerMembership(t, rt)
			original := models.Board{Id: 1, Name: "original", Ident: "original", GroupId: 1, CreateBy: "creator"}
			require.NoError(t, rt.Ctx.DB.Create(&original).Error)
			user := &models.User{Id: 1, Username: "editor"}
			if !allowed {
				user.Id = 2
			}

			w := boardOwnerRequest(t, rt.boardPut, boardForm{Name: "updated", Ident: "updated"}, "/groups/2/boards/1", user)

			var saved models.Board
			require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
			require.Equal(t, original.GroupId, saved.GroupId)
			require.Equal(t, original.CreateBy, saved.CreateBy)
			if allowed {
				require.Empty(t, boardOwnerReply(t, w, nil))
				require.Equal(t, "updated", saved.Name)
				require.Equal(t, "editor", saved.UpdateBy)
			} else {
				require.Equal(t, http.StatusForbidden, w.Code)
				require.Equal(t, original.Name, saved.Name)
				require.Equal(t, original.Ident, saved.Ident)
			}
		})
	}
}

func TestBoardPut_RejectsDuplicateIdentWithoutWriting(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Board{
		{Id: 1, Name: "original", Ident: "original", GroupId: 1}, {Id: 2, Name: "other", Ident: "taken", GroupId: 2},
	}).Error)

	w := boardOwnerRequest(t, rt.boardPut, boardForm{
		Name:  "updated",
		Ident: "taken",
	}, "/groups/1/boards/1", &models.User{
		Id: 1,
	})

	require.Contains(t, boardOwnerReply(t, w, nil), "Ident duplicate")
	var saved models.Board
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "original", saved.Name)
}

func TestBoardPutConfigs_ResolvesIdentAndChecksOwnership(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "overview", Ident: "overview", GroupId: 1}).Error)
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"old":true}`))

	w := boardOwnerRequest(t, rt.boardPutConfigs, boardForm{Configs: `{"panels":[1]}`}, "/groups/2/boards/overview",
		&models.User{Id: 1, Username: "editor"})

	var got models.Board
	require.Empty(t, boardOwnerReply(t, w, &got))
	require.JSONEq(t, `{"panels":[1]}`, got.Configs)
	payload, err := models.BoardPayloadGet(rt.Ctx, 1)
	require.NoError(t, err)
	require.Equal(t, got.Configs, payload)
	var saved models.Board
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, "editor", saved.UpdateBy)
}

func TestBoardPutPublic_UpdatesAndClearsGroupSharing(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.BoardBusigroup{},
	)
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "overview", GroupId: 1}).Error)
	form := boardForm{Public: 1, PublicCate: models.PublicBusi, Bgids: []int64{2}}

	w := boardOwnerRequest(t, rt.boardPutPublic, form, "/groups/1/boards/1", &models.User{Id: 1, Username: "editor"})

	require.Empty(t, boardOwnerReply(t, w, nil))
	allowed, err := models.BoardBusigroupCheck(rt.Ctx, 1, []int64{2})
	require.NoError(t, err)
	require.True(t, allowed)

	w = boardOwnerRequest(t, rt.boardPutPublic, boardForm{Public: 1, PublicCate: models.PublicAnonymous},
		"/groups/1/boards/1", &models.User{Id: 1, Username: "editor"})

	require.Empty(t, boardOwnerReply(t, w, nil))
	allowed, err = models.BoardBusigroupCheck(rt.Ctx, 1, []int64{2})
	require.NoError(t, err)
	require.False(t, allowed)
	var saved models.Board
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	require.Equal(t, models.PublicAnonymous, saved.PublicCate)
}

func TestBoardDelete_RejectsForeignGroupWithoutDeletingPayload(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "restricted", GroupId: 2}).Error)
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"keep":true}`))

	w := boardOwnerRequest(t, rt.boardDel, idsForm{Ids: []int64{1}}, "/groups/1/boards/1", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	var saved models.Board
	require.NoError(t, rt.Ctx.DB.First(&saved, 1).Error)
	payload, err := models.BoardPayloadGet(rt.Ctx, 1)
	require.NoError(t, err)
	require.JSONEq(t, `{"keep":true}`, payload)
}

func TestBoardDelete_RemovesBoardAndPayloadAndSkipsMissingIDs(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "own", GroupId: 1}).Error)
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"keep":true}`))

	w := boardOwnerRequest(t, rt.boardDel, idsForm{Ids: []int64{404, 1}}, "/groups/1/boards/1", &models.User{Id: 1})

	require.Empty(t, boardOwnerReply(t, w, nil))
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Board{}).Count(&count).Error)
	require.Zero(t, count)
	payload, err := models.BoardPayloadGet(rt.Ctx, 1)
	require.NoError(t, err)
	require.Empty(t, payload)
}

func TestBoardClone_CopiesPayloadAndGeneratesDistinctIdent(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "source", Ident: "source", GroupId: 1}).Error)
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"panels":[1]}`))

	w := boardOwnerRequest(t, rt.boardClone, nil, "/groups/1/boards/1", &models.User{Username: "cloner"})

	require.Empty(t, boardOwnerReply(t, w, nil))
	var clone models.Board
	require.NoError(t, rt.Ctx.DB.Where("id <> ?", 1).First(&clone).Error)
	require.Contains(t, clone.Name, "Cloned")
	require.NotEqual(t, "source", clone.Ident)
	require.NotEmpty(t, clone.Ident)
	require.Equal(t, "cloner", clone.CreateBy)
	payload, err := models.BoardPayloadGet(rt.Ctx, clone.Id)
	require.NoError(t, err)
	require.JSONEq(t, `{"panels":[1]}`, payload)
}

func TestBoardBatchClone_RequiresWritableDestination(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
	)
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{Id: 1, Name: "source", GroupId: 1}).Error)

	w := boardOwnerRequest(t, rt.boardBatchClone, boardsForm{BoardIds: []int64{1}, Bgids: []int64{2}},
		"/groups/1/boards/1", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	var count int64
	require.NoError(t, rt.Ctx.DB.Model(&models.Board{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestBoardGetsByGids_FiltersMembershipAndIncludesSharingGroups(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
		&models.BusiGroup{},
		&models.BusiGroupMember{},
		&models.UserGroupMember{},
		&models.BoardBusigroup{},
		&models.User{},
	)
	seedBoardOwnerMembership(t, rt)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.Board{
		{Id: 1, Name: "own", GroupId: 1}, {Id: 2, Name: "foreign", GroupId: 2},
	}).Error)
	require.NoError(t, models.BoardBusigroupUpdate(rt.Ctx, 1, []int64{2}))

	w := boardOwnerRequest(t, rt.boardGetsByGids, nil, "/groups/1/boards/0", &models.User{Id: 1})

	var got []models.Board
	require.Empty(t, boardOwnerReply(t, w, &got))
	require.Len(t, got, 1)
	require.Equal(t, int64(1), got[0].Id)
	require.Equal(t, []int64{2}, got[0].Bgids)
}

func TestBoardPureGet_OmitsAuthorNames(t *testing.T) {
	rt := boardOwnerRouter(t,
		&models.Board{},
		&models.BoardPayload{},
	)
	require.NoError(t, rt.Ctx.DB.Create(&models.Board{
		Id:       1,
		Name:     "overview",
		CreateBy: "creator",
		UpdateBy: "editor",
	}).Error)
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 1, `{"panels":[]}`))

	w := boardOwnerRequest(t, rt.boardPureGet, nil, "/groups/1/boards/1", nil)

	var got models.Board
	require.Empty(t, boardOwnerReply(t, w, &got))
	require.Empty(t, got.CreateBy)
	require.Empty(t, got.UpdateBy)
	require.Equal(t, "overview", got.Name)
}
