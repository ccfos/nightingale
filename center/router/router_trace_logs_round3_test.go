package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceLogsJSON_ReturnsLocalMatchingLines(t *testing.T) {
	rt := &Router{}
	rt.LogDir = logDetailRound2Dir(t, "trace_id=abc-123 matched\ntrace_id=other private\n")
	rt.Alert.Heartbeat.IP = "127.0.0.1"
	rt.HTTP.Port = 17000
	handler := func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "traceid", Value: "abc-123"})
		rt.traceLogsJSON(c)
	}

	w := identityRequest(t, rt, handler, nil, "0", nil)

	var got loggrep.EventDetailResp
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []string{"trace_id=abc-123 matched"}, got.Logs)
	require.Equal(t, "127.0.0.1:17000", got.Instance)
	require.False(t, got.Truncated)
}

func TestGetTraceLogs_QueriesActivePeerAfterLocalMiss(t *testing.T) {
	rt := identityRouter(t, &models.AlertingEngines{})
	rt.LogDir = logDetailRound2Dir(t, "unrelated\n")
	rt.Alert.Heartbeat.IP = "127.0.0.1"
	rt.HTTP.Port = 17000
	rt.Alert.Heartbeat.EngineName = "production"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/n9e/trace-logs/abc-123", r.URL.Path)
		fmt.Fprint(w, `{"dat":{"logs":["trace_id=abc-123 remote"],"truncated":false}}`)
	}))
	t.Cleanup(server.Close)
	node := strings.TrimPrefix(server.URL, "http://")
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertingEngines{
		{Instance: "127.0.0.1:17000", EngineCluster: "production", Clock: 1 << 62},
		{Instance: node, EngineCluster: "production", Clock: 1 << 62},
		{Instance: "never-contact.invalid", EngineCluster: "other", Clock: 1 << 62},
	}).Error)

	got, err := rt.getTraceLogs(context.Background(), "abc-123")

	require.NoError(t, err)
	require.Equal(t, []string{"trace_id=abc-123 remote"}, got.Logs)
	require.Equal(t, node, got.Instance)
}

func TestGetTraceLogs_ReportsFailedPeerAsIncomplete(t *testing.T) {
	rt := identityRouter(t, &models.AlertingEngines{})
	rt.LogDir = logDetailRound2Dir(t, "unrelated\n")
	rt.Alert.Heartbeat.EngineName = "production"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"err":"log storage unavailable"}`)
	}))
	t.Cleanup(server.Close)
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertingEngines{
		Instance: strings.TrimPrefix(server.URL, "http://"), EngineCluster: "production", Clock: 1 << 62,
	}).Error)

	got, err := rt.getTraceLogs(context.Background(), "abc-123")

	require.NoError(t, err)
	require.Empty(t, got.Logs)
	require.True(t, got.Truncated)
	require.Equal(t, loggrep.ReasonTimeout, got.Reason)
}

func TestTraceLogHandlers_ValidateTraceIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		id     string
		page   bool
		status int
	}{
		{name: "page", id: "abc-123", page: true, status: http.StatusOK},
		{name: "invalid page", id: "bad!", page: true, status: http.StatusBadRequest},
		{name: "invalid JSON request", id: "bad!", status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			handler := func(c *gin.Context) {
				c.Params = append(c.Params, gin.Param{Key: "traceid", Value: tc.id})
				if tc.page {
					rt.traceLogsPage(c)
				} else {
					rt.traceLogsJSON(c)
				}
			}

			w := identityRequest(t, rt, handler, nil, "0", nil)

			require.Equal(t, tc.status, w.Code)
			if tc.id == "bad!" {
				require.Contains(t, w.Body.String(), "invalid trace id format")
			} else {
				require.Contains(t, w.Header().Get("Content-Type"), "text/html")
				require.Contains(t, w.Body.String(), "abc-123")
			}
		})
	}
}
