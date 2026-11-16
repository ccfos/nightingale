package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/llm"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func r8bRouter(t *testing.T, tables ...interface{}) *Router {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tables...))
	return &Router{Ctx: &ctx.Context{DB: db, IsCenter: true}}
}
func r8bExecutionRouter(t *testing.T, url string) *Router {
	t.Helper()
	rt := r8bRouter(t, &models.AssistantChatRow{}, &models.AssistantMessageRow{}, &models.AIAgent{}, &models.AILLMConfig{}, &models.User{}, &models.AISkill{}, &models.UserGroupMember{})
	targetReadRedis(t, rt)
	rt.pubsubBus = storage.NewPubsubBus(rt.Redis)
	rt.streamBus = aiagent.NewStreamBus(rt.Redis)
	rt.llmClientCache = &llm.ClientCache{}
	rt.DatasourceCache = &memsto.DatasourceCacheType{}
	require.NoError(t, rt.Ctx.DB.Create(&models.User{Id: 1, Username: "alice", Roles: "Admin"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AIAgent{Name: "chat", UseCase: "chat", Enabled: true, LLMConfigId: 1}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AILLMConfig{Id: 1, Name: "local", Enabled: true, APIType: "openai", APIURL: url + "/v1", APIKey: "local", Model: "test"}).Error)
	return rt
}
func r8bRun(t *testing.T, rt *Router, msg *models.AssistantMessage) {
	t.Helper()
	lock, err := models.AcquireChatLock(context.Background(), rt.Redis, msg.ChatID)
	require.NoError(t, err)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	rt.processAssistantMessage(parent, cancel, lock, NewMessageState(rt.Redis, msg), "stream", 1, "en_US")
	held, err := models.ChatLockHeld(context.Background(), rt.Redis, msg.ChatID)
	require.NoError(t, err)
	require.False(t, held)
	require.True(t, msg.IsFinish, "%+v", msg)
}
func TestR8bAssistantExecutionTextAndHistory(t *testing.T) {
	requests := make(chan map[string]interface{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- body
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"checking\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"The answer is 42.\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	rt := r8bExecutionRouter(t, server.URL)
	msg := &models.AssistantMessage{ChatID: "text", SeqID: 1}
	msg.Query.Content = "Explain monitoring {{ .Name }}"
	r8bRun(t, rt, msg)
	require.Zero(t, msg.ErrCode, msg.ErrMsg)
	require.Equal(t, "general_chat", msg.Extra.Route.ActionKey)
	require.Contains(t, fmt.Sprint(msg.Response), "The answer is 42.")
	var env aiagent.TranscriptEnvelope
	require.NoError(t, json.Unmarshal(msg.Extra.HistoryMessages, &env))
	require.Equal(t, "The answer is 42.", env.Messages[len(env.Messages)-1].Content)
	body := <-requests
	require.Equal(t, true, body["stream"])
	require.Equal(t, "test", body["model"])
	stored, err := models.AssistantMessageGet(rt.Ctx, msg.ChatID, 1)
	require.NoError(t, err)
	require.Equal(t, msg.Response, stored.Response)
	// A second turn must replay the structured first turn without parsing user templates.
	next := &models.AssistantMessage{ChatID: "text", SeqID: 2}
	next.Query.Content = "confirm"
	r8bRun(t, rt, next)
	require.Zero(t, next.ErrCode, next.ErrMsg)
	body = <-requests
	require.Contains(t, fmt.Sprint(body["messages"]), "no pending change proposal")
	require.Contains(t, fmt.Sprint(body["messages"]), "The answer is 42.")
}
func TestR8bAssistantExecutionFailures(t *testing.T) {
	for _, mode := range []string{"no agent", "no llm", "invalid provider", "missing user", "upstream error", "creation form"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(401)
				fmt.Fprint(w, `{"error":{"message":"bad local key","type":"invalid_request_error"}}`)
			}))
			defer server.Close()
			rt := r8bExecutionRouter(t, server.URL)
			msg := &models.AssistantMessage{ChatID: "failure", SeqID: 1}
			msg.Query.Content = "hello"
			switch mode {
			case "no agent":
				require.NoError(t, rt.Ctx.DB.Where("1=1").Delete(&models.AIAgent{}).Error)
			case "no llm":
				require.NoError(t, rt.Ctx.DB.Model(&models.AILLMConfig{}).Where("id=1").Update("enabled", false).Error)
			case "invalid provider":
				require.NoError(t, rt.Ctx.DB.Model(&models.AILLMConfig{}).Where("id=1").Update("api_type", "unsupported").Error)
			case "missing user":
				require.NoError(t, rt.Ctx.DB.Where("id=1").Delete(&models.User{}).Error)
			case "creation form":
				msg.Query.Content = "创建告警规则"
				require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.BusiGroup{}, &models.UserGroup{}, &models.BusiGroupMember{}))
			}
			r8bRun(t, rt, msg)
			switch mode {
			case "no agent":
				require.Contains(t, msg.ErrMsg, "no AI agent")
			case "no llm":
				require.Equal(t, 409, msg.ErrCode)
			case "invalid provider":
				require.Contains(t, msg.ErrMsg, "failed to create LLM")
			case "missing user":
				require.Contains(t, msg.ErrMsg, "failed to resolve user")
			case "upstream error":
				require.NotZero(t, msg.ErrCode)
				require.Contains(t, msg.ErrMsg, "401")
			case "creation form":
				require.Zero(t, msg.ErrCode)
				require.NotEmpty(t, msg.Response)
			}
		})
	}
}
func TestR8bInterruptEffectLedger(t *testing.T) {
	rt := r8bExecutionRouter(t, "http://127.0.0.1:1")
	for _, mode := range []string{"reject", "cached", "continue", "unknown", "unclear"} {
		t.Run(mode, func(t *testing.T) {
			msg := &models.AssistantMessage{ChatID: mode, SeqID: 2}
			msg.Query.Content = "approve"
			pending := &models.PendingInterrupt{Kind: aiagent.InterruptKindApproval, Tool: "missing_tool", SeqID: 1, ResumeArgs: `{}`}
			if mode == "reject" {
				msg.Query.Content = "reject"
			}
			if mode == "unclear" {
				msg.Query.Content = ""
			}
			if mode == "cached" || mode == "continue" {
				rt.putResumeEffect(resumeEffectKey(mode, 1, pending.Tool, pending.ResumeArgs), `{"applied":true,"name":"CPU"}`)
			}
			pending.ResumeAfterConfirm = mode == "continue"
			handled, cont := rt.tryResumePending(context.Background(), NewMessageState(rt.Redis, msg), "stream", pending, nil, nil, "en_US")
			if mode == "unclear" {
				require.False(t, handled)
				require.Empty(t, cont)
				return
			}
			if mode == "continue" {
				require.False(t, handled)
				require.Contains(t, cont, "CPU")
				return
			}
			require.True(t, handled)
			require.True(t, msg.IsFinish)
			text := fmt.Sprint(msg.Response)
			switch mode {
			case "reject":
				require.Contains(t, text, "cancelled")
			case "cached":
				require.Contains(t, text, "Confirmed and applied: CPU")
			case "unknown":
				require.Contains(t, text, "unavailable")
			}
		})
	}
	require.NoError(t, rt.Ctx.DB.Model(&models.AILLMConfig{}).Where("id=1").Update("enabled", false).Error)
	require.Equal(t, approvalUnclear, rt.classifyApprovalLLM("apply?", "new requirements"))
	require.True(t, strings.Contains(formatResumeResult("raw", ""), "raw"))
}

func TestR8bAssistantToolCallsAndPageActions(t *testing.T) {
	for _, mode := range []string{"tool", "page", "input"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "text/event-stream")
				if calls == 1 {
					name, args := "list_busi_groups", `{}`
					if mode == "page" {
						name = "page_action"
						args = `{"name":"set_query","args":{"query":"up"}}`
					}
					if mode == "input" {
						name = "create_skill"
						args = `{"name":"my-skill","description":"demo","instructions":"Do useful work"}`
					}
					delta := map[string]interface{}{"content": "Checking now.", "tool_calls": []interface{}{map[string]interface{}{"index": 0, "id": "call-1", "type": "function", "function": map[string]interface{}{"name": name, "arguments": args}}}}
					frame, _ := json.Marshal(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"delta": delta, "finish_reason": "tool_calls"}}})
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", frame)
				} else {
					fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Operations is available.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
				}
			}))
			defer server.Close()
			rt := r8bExecutionRouter(t, server.URL)
			require.NoError(t, rt.Ctx.DB.AutoMigrate(&models.BusiGroup{}, &models.UserGroup{}, &models.BusiGroupMember{}, &models.Datasource{}))
			require.NoError(t, rt.Ctx.DB.Create(&models.BusiGroup{Id: 1, Name: "Operations"}).Error)
			msg := &models.AssistantMessage{ChatID: mode, SeqID: 1}
			msg.Query.Content = "help with monitoring"
			if mode == "page" {
				msg.Query.PageActions = []models.AssistantPageAction{{Name: "set_query", Description: "Set query", InputSchema: json.RawMessage(`{"type":"object"}`)}}
			}
			r8bRun(t, rt, msg)
			require.Zero(t, msg.ErrCode, msg.ErrMsg)
			switch mode {
			case "page":
				require.Equal(t, models.ContentTypePageAction, msg.Response[0].ContentType)
				require.Contains(t, string(msg.Extra.HistoryMessages), "set_query")
				require.NotContains(t, string(msg.Extra.HistoryMessages), "call-1")
			case "input":
				require.NotNil(t, msg.Extra.Pending)
				require.Equal(t, aiagent.InterruptKindInput, msg.Extra.Pending.Kind)
				require.True(t, msg.Extra.Route.AwaitingForm)
				require.Equal(t, models.ContentTypeFormSelect, msg.Response[len(msg.Response)-1].ContentType)
			case "tool":
				require.True(t, msg.ExecutedTools)
				require.Contains(t, string(msg.Extra.HistoryMessages), "Operations")
				require.Contains(t, fmt.Sprint(msg.Response), "Operations is available.")
			}
		})
	}
}
