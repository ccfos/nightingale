package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// 账号被禁用后，刷新 token 必须失败并清掉会话。否则前端的重试链是
// 接口 401 → 刷新成功 → 整页重载 → 再 401，页面永远停在白屏，等不到登录页。
func TestRefreshPostRejectsDisabledUser(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	user := &models.User{Username: "ui5326", Contacts: []byte("{}")}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	rt := &Router{
		Ctx:   &ctx.Context{DB: db},
		Redis: redis.NewClient(&redis.Options{Addr: mr.Addr()}),
	}
	rt.HTTP.JWTAuth = httpx.JWTAuth{SigningKey: "session-signing-key", AccessExpired: 10, RefreshExpired: 60}

	userIdentity := "1-ui5326"
	ts, err := rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
	if err != nil {
		t.Fatalf("create tokens: %v", err)
	}
	if err := rt.createAuth(newTestCtx().Request.Context(), userIdentity, ts); err != nil {
		t.Fatalf("create auth: %v", err)
	}

	refresh := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		gin.SetMode(gin.TestMode)
		c, _ := gin.CreateTestContext(w)
		body, _ := json.Marshal(refreshForm{RefreshToken: ts.RefreshToken})
		c.Request = httptest.NewRequest(http.MethodPost, "/api/n9e/auth/refresh", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		rt.refreshPost(c)
		return w
	}

	// 正常账号刷新得通
	if w := refresh(); w.Code != http.StatusOK {
		t.Fatalf("enabled user refresh: code=%d body=%q, want 200", w.Code, w.Body.String())
	}

	// 重新签一套会话再禁用，验证禁用后刷新被拒
	ts, err = rt.createTokens(rt.HTTP.JWTAuth.SigningKey, userIdentity)
	if err != nil {
		t.Fatalf("create tokens: %v", err)
	}
	if err := rt.createAuth(newTestCtx().Request.Context(), userIdentity, ts); err != nil {
		t.Fatalf("create auth: %v", err)
	}
	if err := user.UpdateDisabled(rt.Ctx, models.UserDisabled, "root"); err != nil {
		t.Fatalf("disable user: %v", err)
	}

	w := refresh()
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user refresh: code=%d body=%q, want 401", w.Code, w.Body.String())
	}

	// 会话已经清掉，再刷一次也只能是 401，不会出现「刷新成功→重载→再 401」的死循环
	if w := refresh(); w.Code != http.StatusUnauthorized {
		t.Fatalf("disabled user refresh again: code=%d, want 401", w.Code)
	}
}

func newTestCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c
}
