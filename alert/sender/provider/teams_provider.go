package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

// TeamsProvider sends Adaptive Cards to Teams Workflows webhooks.
// https://learn.microsoft.com/microsoftteams/platform/webhooks-and-connectors/how-to/connectors-using
// A successful workflow response acknowledges submission, not delivery to Teams.
type TeamsProvider struct{}

func (*TeamsProvider) Ident() string { return models.Teams }
func (*TeamsProvider) Check(c *models.NotifyChannelConfig) error {
	if c == nil || c.RequestType != models.RequestTypeTeams {
		return errors.New("teams provider requires request_type: teams")
	}
	if n := c.RequestConfig.NativeNetwork(models.RequestTypeTeams); n != nil && (n.RetryTimes < 0 || n.RetrySleep < 0) {
		return errors.New("Teams retry settings must not be negative")
	}
	return nil
}

func (*TeamsProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req == nil {
		return &NotifyResult{Err: errors.New("Teams notify request is required")}
	}
	webhookURL, err := parseWebhookURL(req.CustomParams["webhook_url"], "", "Teams", "https://<workflow-host>/workflows/<id>/triggers/manual/paths/invoke?...")
	if err != nil {
		return &NotifyResult{Err: err}
	}
	u, _ := url.Parse(webhookURL)
	if u.User != nil || u.Fragment != "" {
		return &NotifyResult{Err: errors.New("Teams webhook URL must not contain user credentials or a fragment")}
	}
	target := strings.TrimSpace(req.CustomParams["bot_name"])
	if target == "" {
		target = u.Scheme + "://" + u.Host + "/***"
	}
	var cfg *models.TeamsRequestConfig
	if req.Config != nil && req.Config.RequestConfig != nil && req.Config.RequestConfig.TeamsRequestConfig != nil {
		cfg = req.Config.RequestConfig.TeamsRequestConfig
	}
	card := buildTeamsCard(req)
	body, err := json.Marshal(map[string]interface{}{"type": "message", "attachments": []interface{}{map[string]interface{}{
		"contentType": "application/vnd.microsoft.card.adaptive", "contentUrl": nil, "content": card,
	}}})
	if err != nil {
		return &NotifyResult{Target: target, Err: err}
	}
	if len(body) > 28*1024 {
		return &NotifyResult{Target: target, Err: errors.New("Teams card exceeds 28 KB; shorten the message template")}
	}
	retries, sleep := nativeRetrySettings(nil)
	if cfg != nil {
		_, sleep = nativeRetrySettings(&cfg.NativeNetworkConfig)
		retries = cfg.RetryTimes // An explicit zero disables retries.
	}
	_, err = doWithRetry(ctx, req.HttpClient, retries, sleep, func(ctx context.Context) (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", "application/json")
		return r, nil
	}, checkTeamsResponse)
	if err != nil {
		return &NotifyResult{Target: target, Err: errors.New(maskTeamsError(err.Error(), webhookURL))}
	}
	return &NotifyResult{Target: target, Response: "accepted by Teams webhook"}
}

func checkTeamsResponse(r *nativeResponse) (bool, error) {
	// Legacy connectors sometimes return HTTP 200 with a throttling error in the body.
	if strings.Contains(string(r.Body), "Microsoft Teams endpoint returned HTTP error 429") {
		return true, &HTTPStatusError{StatusCode: http.StatusTooManyRequests, Detail: "Teams webhook rate limited"}
	}
	if r.StatusCode/100 == 2 {
		body := strings.TrimSpace(string(r.Body))
		if body == "" || body == "1" {
			return false, nil
		}
		return false, &HTTPStatusError{StatusCode: r.StatusCode, Detail: truncateBody(r.Body)}
	}
	return checkStatus(r, nil)
}

// URL parts that are structure rather than credentials; redacting them would
// only garble the provider's error text.
var teamsURLWords = map[string]bool{
	"webhookb2": true, "incomingwebhook": true, "workflows": true, "triggers": true,
	"powerautomate": true, "automations": true,
}
var teamsPublicQueryKeys = map[string]bool{"api-version": true, "sp": true, "sv": true}

func maskTeamsError(msg, raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "Teams webhook request failed"
	}
	msg = strings.ReplaceAll(msg, raw, u.Scheme+"://"+u.Host+"/***")
	// Workflows credentials are in the query; legacy webhook credentials can span multiple path segments.
	for _, part := range strings.Split(u.Path, "/") {
		if len(part) >= 8 && !teamsURLWords[strings.ToLower(part)] {
			msg = strings.ReplaceAll(msg, part, redactedMark)
		}
	}
	for key, values := range u.Query() {
		if teamsPublicQueryKeys[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			// Short values of unknown keys would match unrelated text such as status codes.
			if value == "" || (len(value) < 8 && !strings.EqualFold(key, "sig")) {
				continue
			}
			msg = strings.ReplaceAll(msg, url.QueryEscape(value), redactedMark)
			msg = strings.ReplaceAll(msg, value, redactedMark)
		}
	}
	return msg
}
