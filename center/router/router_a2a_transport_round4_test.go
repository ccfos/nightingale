package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/aiagent/a2a"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCaptureA2ARequestBody_PreservesFullBodyWhenPreviewIsTruncated(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{name: "small", size: 32},
		{name: "exact limit", size: a2aLogBodyLimit},
		{name: "over limit", size: a2aLogBodyLimit + 4096},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := strings.Repeat("x", tc.size)
			req := httptest.NewRequest(http.MethodPost, "/a2a/message:send", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json; charset=utf-8")

			preview, truncated := captureA2ARequestBody(req)

			require.Equal(t, tc.size > a2aLogBodyLimit, truncated)
			if tc.size > a2aLogBodyLimit {
				require.Equal(t, body[:a2aLogBodyLimit], preview)
			} else {
				require.Equal(t, body, preview)
			}
			restored, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.Equal(t, body, string(restored))
			require.NoError(t, req.Body.Close())
		})
	}
}

func TestCaptureA2ARequestBody_SkipsBinaryBodyWithoutConsumingIt(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("binary payload"))
	req.Header.Set("Content-Type", "application/octet-stream")

	preview, truncated := captureA2ARequestBody(req)

	require.Equal(t, "<skipped non-json content-type=application/octet-stream>", preview)
	require.False(t, truncated)
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, "binary payload", string(body))
}

func TestCaptureA2ARequestBody_HandlesAbsentBodies(t *testing.T) {
	cases := []struct {
		name string
		req  *http.Request
	}{
		{name: "nil request"},
		{name: "nil body", req: &http.Request{}},
		{name: "no body", req: &http.Request{Body: http.NoBody}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			preview, truncated := captureA2ARequestBody(tc.req)

			require.Empty(t, preview)
			require.False(t, truncated)
		})
	}
}

func TestA2ARequestLog_ForwardsBodyStatusHeadersAndStreamingWrites(t *testing.T) {
	rt := &Router{}
	engine := gin.New()
	engine.Use(rt.a2aRequestLog("test"))
	engine.POST("/a2a/message:send", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		require.NoError(t, err)
		require.Equal(t, `{"message":"hello"}`, string(body))
		c.Set("user", &models.User{Username: "alice"})
		c.Header("X-Test-Response", "streaming")
		c.Status(http.StatusAccepted)
		_, err = c.Writer.WriteString("first:")
		require.NoError(t, err)
		_, err = c.Writer.Write([]byte("last"))
		require.NoError(t, err)
		c.Writer.Flush()
	})
	req := httptest.NewRequest(http.MethodPost, "/a2a/message:send", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusAccepted, w.Code)
	require.Equal(t, "first:last", w.Body.String())
	require.Equal(t, "streaming", w.Header().Get("X-Test-Response"))
	require.True(t, w.Flushed)
}

func TestInjectA2AUser_PropagatesVerifiedIdentityToRequestContext(t *testing.T) {
	rt := &Router{}
	user := &models.User{Id: 7, Username: "alice"}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("user", user) }, rt.injectA2AUser())
	engine.GET("/identity", func(c *gin.Context) {
		got := a2a.UserFromContext(c.Request.Context())
		require.Same(t, user, got)
		c.String(http.StatusOK, got.Username)
	})
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/identity", nil))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "alice", w.Body.String())
}

func TestInjectA2AUser_RejectsMissingOrMalformedMiddlewareIdentity(t *testing.T) {
	cases := []struct {
		name   string
		value  interface{}
		status int
	}{
		{name: "missing", status: http.StatusUnauthorized},
		{name: "wrong type", value: "alice", status: http.StatusInternalServerError},
		{name: "typed nil", value: (*models.User)(nil), status: http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			engine := gin.New()
			engine.Use(aop.Recovery(), func(c *gin.Context) {
				if tc.value != nil {
					c.Set("user", tc.value)
				}
			}, rt.injectA2AUser())
			engine.GET("/identity", func(c *gin.Context) { t.Error("invalid identity reached handler") })
			w := httptest.NewRecorder()

			engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/identity", nil))

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusUnauthorized {
				require.Contains(t, w.Body.String(), "unauthorized")
			} else {
				require.Contains(t, w.Body.String(), "user middleware returned wrong type")
			}
		})
	}
}

func TestA2ARootEndpointHint_ReturnsRESTEndpointsWithoutRedirect(t *testing.T) {
	rt := &Router{}
	engine := gin.New()
	engine.POST("/a2a", rt.a2aRootEndpointHint)
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/a2a", nil))

	require.Equal(t, http.StatusNotFound, w.Code)
	require.Empty(t, w.Header().Get("Location"))
	require.JSONEq(t, `{
		"error":"no A2A method at /a2a; this server speaks the A2A v0.3 HTTP+JSON (REST) binding, not legacy JSON-RPC (tasks/send)",
		"endpoints":{
			"send_message":"POST /a2a/message:send","stream_message":"POST /a2a/message:stream",
			"get_task":"GET /a2a/tasks/{id}","cancel_task":"POST /a2a/tasks/{id}:cancel"
		},
		"agent_card":"/.well-known/agent-card.json"
	}`, w.Body.String())
}
