package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/gin-gonic/gin"
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
	return &Router{Ctx: c, UserTokenCache: &memsto.UserTokenCacheType{}}, c
}

func callSelfToken(handler gin.HandlerFunc, user *models.User, method, path string, params gin.Params, body string) (*httptest.ResponseRecorder, any) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	c.Set("user", user)
	c.Set("username", user.Username)

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		handler(c)
	}()
	return w, recovered
}

func TestSelfTokenAddAndDeleteUpdateCacheImmediately(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt, c := setupSelfTokenTest(t)
	alice := &models.User{Id: 1, Username: "alice"}

	// 预置一个 token，保证删除时不是最后一个
	if _, err := models.AddToken(c, "alice", "token-keep", "keep"); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	w, rec := callSelfToken(rt.addToken, alice, http.MethodPost, "/api/n9e/self/token", nil, `{"token_name":"new"}`)
	if rec != nil || w.Code != http.StatusOK {
		t.Fatalf("addToken: code=%d recovered=%v body=%s", w.Code, rec, w.Body.String())
	}
	var resp struct {
		Dat models.UserToken `json:"dat"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Dat.Token == "" {
		t.Fatalf("decode addToken response: %v, body=%s", err, w.Body.String())
	}
	if u := rt.UserTokenCache.GetByToken(resp.Dat.Token); u == nil || u.Username != "alice" {
		t.Fatalf("new token should authenticate without waiting for sync, got %+v", u)
	}

	id := strconv.FormatInt(resp.Dat.Id, 10)
	w, rec = callSelfToken(rt.deleteToken, alice, http.MethodDelete, "/api/n9e/self/token/"+id, gin.Params{{Key: "id", Value: id}}, "")
	if rec != nil || w.Code != http.StatusOK {
		t.Fatalf("deleteToken: code=%d recovered=%v body=%s", w.Code, rec, w.Body.String())
	}
	if u := rt.UserTokenCache.GetByToken(resp.Dat.Token); u != nil {
		t.Fatalf("deleted token should stop authenticating immediately, got %+v", u)
	}
}

func TestSelfTokenDeleteRejectsOtherUsersToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt, c := setupSelfTokenTest(t)
	alice := &models.User{Id: 1, Username: "alice"}

	for _, name := range []string{"a1", "a2"} {
		if _, err := models.AddToken(c, "alice", "token-"+name, name); err != nil {
			t.Fatalf("seed alice token: %v", err)
		}
	}
	bobToken, err := models.AddToken(c, "bob", "token-bob", "bob")
	if err != nil {
		t.Fatalf("seed bob token: %v", err)
	}

	id := strconv.FormatInt(bobToken.Id, 10)
	_, rec := callSelfToken(rt.deleteToken, alice, http.MethodDelete, "/api/n9e/self/token/"+id, gin.Params{{Key: "id", Value: id}}, "")
	if rec == nil {
		t.Fatalf("deleting another user's token should be rejected")
	}

	var count int64
	c.DB.Model(&models.UserToken{}).Where("id = ?", bobToken.Id).Count(&count)
	if count != 1 {
		t.Fatalf("bob's token was deleted by alice")
	}
}
