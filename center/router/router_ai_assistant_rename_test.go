package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/errorx"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gin-gonic/gin"
)

func setupAssistantChatRenameTest(t *testing.T) (*Router, *ctx.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AssistantChatRow{}); err != nil {
		t.Fatalf("migrate assistant chat: %v", err)
	}
	c := &ctx.Context{DB: db}
	return &Router{Ctx: c}, c
}

func callAssistantChatRename(rt *Router, userID int64, body string) (*httptest.ResponseRecorder, any) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/n9e/assistant/chat/rename", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &models.User{Id: userID})

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		rt.assistantChatRename(c)
	}()
	return w, recovered
}

func TestAssistantChatRename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt, c := setupAssistantChatRenameTest(t)
	chat := models.AssistantChat{
		ChatID:     "chat-rename",
		Title:      "New Chat",
		LastUpdate: 1700000000,
		UserID:     100,
		IsNew:      true,
	}
	if err := models.AssistantChatSet(c, chat); err != nil {
		t.Fatalf("seed chat: %v", err)
	}

	w, recovered := callAssistantChatRename(rt, chat.UserID, `{"chat_id":"chat-rename","title":"生产环境告警排查"}`)
	if recovered != nil {
		t.Fatalf("rename panicked: %v", recovered)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	var response struct {
		Dat models.AssistantChat `json:"dat"`
		Err string               `json:"err"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Err != "" || response.Dat.Title != "生产环境告警排查" || !response.Dat.IsRenamed {
		t.Fatalf("unexpected response: %+v", response)
	}

	stored, err := models.AssistantChatGet(c, chat.ChatID)
	if err != nil {
		t.Fatalf("load renamed chat: %v", err)
	}
	if stored.Title != response.Dat.Title || !stored.IsRenamed {
		t.Fatalf("rename was not persisted: %+v", stored)
	}
	if stored.LastUpdate != chat.LastUpdate {
		t.Fatalf("last_update changed from %d to %d", chat.LastUpdate, stored.LastUpdate)
	}
}

func TestAssistantChatRenameRejectsInvalidOrForeignChat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rt, c := setupAssistantChatRenameTest(t)
	chat := models.AssistantChat{ChatID: "chat-owned", Title: "Original", UserID: 100}
	if err := models.AssistantChatSet(c, chat); err != nil {
		t.Fatalf("seed chat: %v", err)
	}

	for _, tc := range []struct {
		name     string
		user     int64
		body     string
		want     string
		wantCode int
	}{
		{name: "blank title", user: 100, body: `{"chat_id":"chat-owned","title":"  "}`, want: "title is required", wantCode: http.StatusBadRequest},
		{name: "missing chat id", user: 100, body: `{"title":"renamed"}`, want: "chat_id is required", wantCode: http.StatusBadRequest},
		{name: "foreign chat", user: 101, body: `{"chat_id":"chat-owned","title":"renamed"}`, want: "forbidden", wantCode: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, recovered := callAssistantChatRename(rt, tc.user, tc.body)
			pageErr, ok := recovered.(errorx.PageError)
			if !ok {
				t.Fatalf("panic = %#v, want errorx.PageError", recovered)
			}
			if pageErr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d", pageErr.Code, tc.wantCode)
			}
			if pageErr.Message != tc.want {
				t.Fatalf("message = %q, want %q", pageErr.Message, tc.want)
			}
		})
	}

	stored, err := models.AssistantChatGet(c, chat.ChatID)
	if err != nil {
		t.Fatalf("load chat: %v", err)
	}
	if stored.Title != chat.Title || stored.IsRenamed {
		t.Fatalf("invalid rename modified chat: %+v", stored)
	}
}
