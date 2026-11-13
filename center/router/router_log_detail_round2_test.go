package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/loggrep"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func logDetailRound2Dir(t *testing.T, content string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "router-log-detail-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	require.NoError(t, os.WriteFile(filepath.Join(dir, "INFO.log"), []byte(content), 0600))
	return dir
}

func TestEventDetailJSON_ReturnsMatchingLocalLogsForAuthorizedGroup(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{}, &models.AlertingEngines{},
		&models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	hash := "0123456789abcdef0123456789abcdef"
	rt.LogDir = logDetailRound2Dir(t, "unrelated\nprocessing "+hash+" dispatched\n")
	rt.Alert.Heartbeat.IP = "127.0.0.1"
	rt.HTTP.Port = 17000
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{Id: 1, Hash: hash, GroupId: 1, DatasourceId: 99}).Error)
	handler := func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "hash", Value: hash})
		rt.eventDetailJSON(c)
	}

	w := identityRequest(t, rt, handler, nil, "1", &models.User{Id: 1})

	var got loggrep.EventDetailResp
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, "127.0.0.1:17000", got.Instance)
	require.Equal(t, []string{"processing " + hash + " dispatched"}, got.Logs)
	require.False(t, got.Truncated)
}

func TestEventDetailJSON_RejectsUnrelatedGroupBeforeReadingLogs(t *testing.T) {
	rt := identityRouter(t, &models.AlertHisEvent{}, &models.BusiGroup{},
		&models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	hash := "0123456789abcdef0123456789abcdef"
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertHisEvent{Id: 1, Hash: hash, GroupId: 2}).Error)
	handler := func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "hash", Value: hash})
		rt.eventDetailJSON(c)
	}

	w := identityRequest(t, rt, handler, nil, "1", &models.User{Id: 1})

	require.Equal(t, http.StatusForbidden, w.Code)
	require.NotContains(t, w.Body.String(), `"logs"`)
}

func TestAlertEvalDetailJSON_ReturnsOnlySelectedRulesLocalLogs(t *testing.T) {
	rt := identityRouter(t, &models.AlertRule{}, &models.BusiGroup{}, &models.BusiGroupMember{}, &models.UserGroupMember{})
	seedBoardOwnerMembership(t, rt)
	rt.DatasourceCache = &memsto.DatasourceCacheType{}
	rt.Alert.Heartbeat.IP = "127.0.0.1"
	rt.HTTP.Port = 17000
	rt.LogDir = logDetailRound2Dir(t, "alert_eval_1 evaluated\nalert_eval_2 private\n")
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 1, GroupId: 1}).Error)

	w := identityRequest(t, rt, rt.alertEvalDetailJSON, nil, "1", &models.User{Id: 1})

	var got loggrep.EventDetailResp
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, []string{"alert_eval_1 evaluated"}, got.Logs)
	require.Equal(t, "127.0.0.1:17000", got.Instance)
	require.False(t, got.Truncated)
}

func TestLogDetailPages_ValidateIdentifiersBeforeRenderingShell(t *testing.T) {
	cases := []struct {
		name   string
		event  bool
		id     string
		status int
	}{
		{name: "valid event", event: true, id: "0123456789abcdef0123456789abcdef", status: 200},
		{name: "invalid event", event: true, id: "invalid!", status: 400},
		{name: "valid rule", id: "7", status: 200},
		{name: "invalid rule", id: "abc", status: 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}
			handler := func(c *gin.Context) {
				if tc.event {
					c.Params = append(c.Params, gin.Param{Key: "hash", Value: tc.id})
					rt.eventDetailPage(c)
				} else {
					rt.alertEvalDetailPage(c)
				}
			}

			w := identityRequest(t, rt, handler, nil, tc.id, nil)

			require.Equal(t, tc.status, w.Code)
			if tc.status == http.StatusOK {
				require.Contains(t, w.Header().Get("Content-Type"), "text/html")
				require.Contains(t, w.Body.String(), tc.id)
				require.Contains(t, strings.ToLower(w.Body.String()), "<!doctype html>")
			} else {
				require.Contains(t, w.Body.String(), "invalid")
			}
		})
	}
}

func TestForwardEventDetail_BuildsAuthenticatedRequestAndPreservesTruncation(t *testing.T) {
	rt := &Router{}
	rt.HTTP.APIForService.BasicAuth = map[string]string{"service": "secret"}
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"dat":{"logs":["matched"],"truncated":true,"reason":"timeout"},"err":""}`))
	}))
	defer server.Close()
	node := strings.TrimPrefix(server.URL, "http://")

	got, err := rt.forwardEventDetail(context.Background(), node, "hash-1", time.Unix(123, 0))

	require.NoError(t, err)
	request := <-requests
	username, password, ok := request.BasicAuth()
	require.True(t, ok)
	require.Equal(t, http.MethodGet, request.Method)
	require.Equal(t, "/v1/n9e/event-detail/hash-1", request.URL.Path)
	require.NotEmpty(t, request.URL.Query().Get("timeout"))
	require.Equal(t, "123", request.URL.Query().Get("since"))
	require.Equal(t, "service", username)
	require.Equal(t, "secret", password)
	require.Equal(t, []string{"matched"}, got.Logs)
	require.Equal(t, node, got.Instance)
	require.True(t, got.Truncated)
	require.Equal(t, loggrep.ReasonTimeout, truncatedReason(got))
}

func TestForwardLogQuery_PropagatesRemoteErrors(t *testing.T) {
	rt := &Router{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"err":"remote log unavailable"}`))
	}))
	defer server.Close()
	node := strings.TrimPrefix(server.URL, "http://")

	_, err := rt.forwardLogQuery(context.Background(), node, "/event-detail/hash-1", time.Time{}, time.Second)

	require.EqualError(t, err, "remote log unavailable")
}

func TestEventLogsSince_UsesEarliestTriggerWithClockSkewAllowance(t *testing.T) {
	cases := []struct {
		name     string
		event    models.AlertHisEvent
		expected time.Time
	}{
		{name: "unknown trigger"},
		{name: "first trigger only", event: models.AlertHisEvent{FirstTriggerTime: 10000}, expected: time.Unix(6400, 0)},
		{name: "trigger only", event: models.AlertHisEvent{TriggerTime: 10000}, expected: time.Unix(6400, 0)},
		{
			name: "trigger before first", event: models.AlertHisEvent{FirstTriggerTime: 12000, TriggerTime: 10000},
			expected: time.Unix(6400, 0),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := eventLogsSince(&tc.event)

			require.Equal(t, tc.expected, got)
		})
	}
}
