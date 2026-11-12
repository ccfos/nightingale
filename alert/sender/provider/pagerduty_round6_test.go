package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestPagerDutyNotify_SendsEachRoutingKeyWithEventLifecycle(t *testing.T) {
	server, requests := newCaptureServer(t, always(http.StatusAccepted, `{"status":"success"}`))
	localURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) {
		// Keep the production path and headers while directing the fixed endpoint to the local server.
		request := r.Clone(r.Context())
		request.URL.Scheme, request.URL.Host = localURL.Scheme, localURL.Host
		return transport.RoundTrip(request)
	})}
	events := []*models.AlertCurEvent{
		{Id: 11, Hash: "critical", RuleName: "CPU high", Severity: 1, Cluster: "prod", RuleId: 7},
		{Id: 12, Hash: "error", RuleName: "Disk full", Severity: 2},
		{Id: 13, Hash: "warning", RuleName: "Memory recovered", Severity: 3, IsRecovered: true},
	}
	req := &NotifyRequest{
		Config: &models.NotifyChannelConfig{RequestConfig: &models.RequestConfig{
			PagerDutyRequestConfig: &models.PagerDutyRequestConfig{},
		}},
		Events: events, PagerDutyRoutingKeys: []string{"key-a", "key-b"}, SiteUrl: "https://n9e.example.test",
		HttpClient: client,
	}

	result := new(PagerDutyProvider).Notify(context.Background(), req)

	require.NoError(t, result.Err)
	require.Equal(t, "key-a,key-b", result.Target)
	require.Contains(t, result.Response, `event 11: status_code:202, response:{"status":"success"}`)
	require.Len(t, requests(), 6)
	for i, sent := range requests() {
		event := events[i%len(events)]
		var body struct {
			RoutingKey string `json:"routing_key"`
			Action     string `json:"event_action"`
			DedupKey   string `json:"dedup_key"`
			Payload    struct {
				Summary  string `json:"summary"`
				Severity string `json:"severity"`
			} `json:"payload"`
			Links []map[string]string `json:"links"`
		}
		require.NoError(t, json.Unmarshal(sent.Body, &body))
		require.Equal(t, http.MethodPost, sent.Method)
		require.Equal(t, "/v2/enqueue", sent.Path)
		require.Equal(t, "application/json", sent.Header.Get("Content-Type"))
		require.Equal(t, req.PagerDutyRoutingKeys[i/len(events)], body.RoutingKey)
		require.Equal(t, event.Hash, body.DedupKey)
		require.Equal(t, event.RuleName, body.Payload.Summary)
		require.Equal(t, []string{"critical", "error", "warning"}[i%len(events)], body.Payload.Severity)
		if event.IsRecovered {
			require.Equal(t, "resolve", body.Action)
		} else {
			require.Equal(t, "trigger", body.Action)
		}
		require.Len(t, body.Links, 2)
		require.Contains(t, body.Links[0]["href"], req.SiteUrl+"/alert-his-events/")
	}
}
