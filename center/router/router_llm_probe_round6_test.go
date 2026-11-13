package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestAILLMConfigTest_UsesStoredKeyForMaskedInputWithoutSaving(t *testing.T) {
	requests := make(chan *http.Request, 1)
	bodies := make(chan map[string]interface{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- r.Clone(r.Context())
		bodies <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}]}`)
	}))
	t.Cleanup(server.Close)
	rt := identityRouter(t, &models.AILLMConfig{})
	stored := models.AILLMConfig{Id: 1, Name: "local", APIKey: "0123456789abcdef", Model: "original"}
	require.NoError(t, rt.Ctx.DB.Create(&stored).Error)
	form := models.AILLMConfig{
		Name: "local", APIType: "openai", APIURL: server.URL + "/v1", APIKey: "0123****cdef", Model: "probe-model",
	}

	w := identityRequest(t, rt, rt.aiLLMConfigTest, form, "0", nil)

	var got struct {
		Success  bool   `json:"success"`
		Duration int64  `json:"duration_ms"`
		Error    string `json:"error"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.True(t, got.Success)
	require.Empty(t, got.Error)
	require.GreaterOrEqual(t, got.Duration, int64(0))
	received, body := <-requests, <-bodies
	require.Equal(t, "/v1/chat/completions", received.URL.Path)
	require.Equal(t, "Bearer 0123456789abcdef", received.Header.Get("Authorization"))
	require.Equal(t, "probe-model", body["model"])
	require.Equal(t, []interface{}{map[string]interface{}{"role": "user", "content": "Hi"}}, body["messages"])
	var unchanged models.AILLMConfig
	require.NoError(t, rt.Ctx.DB.First(&unchanged, 1).Error)
	require.Equal(t, "original", unchanged.Model)
	require.Equal(t, stored.APIKey, unchanged.APIKey)
}

func TestAILLMConfigTest_ClassifiesAuthenticationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid key","type":"invalid_request_error"}}`)
	}))
	t.Cleanup(server.Close)
	rt := &Router{}

	w := identityRequest(t, rt, rt.aiLLMConfigTest, models.AILLMConfig{
		APIType: "openai", APIURL: server.URL + "/v1", APIKey: "wrong", Model: "test-model",
	}, "0", nil)

	var got struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.False(t, got.Success)
	require.Contains(t, got.Error, "401")
	require.Contains(t, got.Error, "invalid key")
}

func TestAILLMConfigTest_RejectsMissingRequiredFields(t *testing.T) {
	rt := &Router{}

	w := identityRequest(t, rt, rt.aiLLMConfigTest, models.AILLMConfig{APIType: "openai"}, "0", nil)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "api_type, api_url, api_key, model are required")
}
