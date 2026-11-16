package router

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func r8cRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	if len(tables) > 0 {
		require.NoError(t, db.AutoMigrate(tables...))
	}
	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}

func r8cRequest(t *testing.T, handler gin.HandlerFunc, body interface{}, path string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	e := gin.New()
	e.Use(aop.Recovery())
	e.Use(func(c *gin.Context) {
		c.Set("user", &models.User{Id: 7, Username: "editor", Roles: "Admin", RolesLst: []string{models.AdminRole}})
		c.Set("username", "editor")
	})
	e.Any("/:id", handler)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", path, bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Language", "en_US")
	e.ServeHTTP(w, r)
	return w
}

func r8cData(t *testing.T, w *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	require.Equal(t, 200, w.Code, w.Body.String())
	var reply struct {
		Dat json.RawMessage `json:"dat"`
		Err string          `json:"err"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &reply))
	require.Empty(t, reply.Err, w.Body.String())
	if out != nil {
		require.NoError(t, json.Unmarshal(reply.Dat, out))
	}
}
