package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errorx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSelfTokenTest(t *testing.T) (*Router, *ctx.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.UserToken{}); err != nil {
		t.Fatalf("migrate user_token: %v", err)
	}
	c := &ctx.Context{DB: db, IsCenter: true}
	return &Router{Ctx: c}, c
}

// callDeleteToken 调用 deleteToken，返回响应和 ginx.Bomb 抛出的 panic（如有）。
func callDeleteToken(rt *Router, username string, id int64) (*httptest.ResponseRecorder, any) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	idStr := strconv.FormatInt(id, 10)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/n9e/self/token/"+idStr, nil)
	c.Params = gin.Params{{Key: "id", Value: idStr}}
	c.Set("username", username)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		rt.deleteToken(c)
	}()
	return w, recovered
}

func tokenExists(t *testing.T, c *ctx.Context, id int64) bool {
	t.Helper()
	var count int64
	if err := c.DB.Model(&models.UserToken{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatalf("count token: %v", err)
	}
	return count > 0
}

func TestSelfTokenDeleteOwnToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt, c := setupSelfTokenTest(t)

	// 预置两个 token，保证删除时不是最后一个
	if _, err := models.AddToken(c, "alice", "token-keep", "keep"); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	tok, err := models.AddToken(c, "alice", "token-del", "del")
	if err != nil {
		t.Fatalf("seed token: %v", err)
	}

	w, rec := callDeleteToken(rt, "alice", tok.Id)
	if rec != nil || w.Code != http.StatusOK {
		t.Fatalf("deleteToken: code=%d recovered=%v body=%s", w.Code, rec, w.Body.String())
	}
	if tokenExists(t, c, tok.Id) {
		t.Fatalf("token should be deleted")
	}
}

func TestSelfTokenDeleteRejectsOtherUsersToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt, c := setupSelfTokenTest(t)

	for _, name := range []string{"a1", "a2"} {
		if _, err := models.AddToken(c, "alice", "token-"+name, name); err != nil {
			t.Fatalf("seed alice token: %v", err)
		}
	}
	bobToken, err := models.AddToken(c, "bob", "token-bob", "bob")
	if err != nil {
		t.Fatalf("seed bob token: %v", err)
	}

	_, rec := callDeleteToken(rt, "alice", bobToken.Id)
	bomb, ok := rec.(errorx.PageError)
	if !ok || bomb.Code != http.StatusNotFound {
		t.Fatalf("deleting another user's token should return 404, recovered=%v", rec)
	}
	if !tokenExists(t, c, bobToken.Id) {
		t.Fatalf("bob's token was deleted by alice")
	}
}
