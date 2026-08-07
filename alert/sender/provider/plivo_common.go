package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

const plivoAPIBase = "https://api.plivo.com/v1/Account"

// normalizePlivoNumber strips a single leading "+" so operators may enter a
// number in either form. Plivo accepts the bare-digit E.164 form, and building
// the request body in code (rather than through a template) means the "+" is
// never HTML-escaped the way the generic HTTP body renderer would escape it.
func normalizePlivoNumber(n string) string {
	return strings.TrimPrefix(strings.TrimSpace(n), "+")
}

// plivoContent extracts the rendered alert text from the notification template.
func plivoContent(tpl map[string]interface{}) string {
	if tpl == nil {
		return ""
	}
	if v, ok := tpl["content"]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// postPlivoJSON POSTs a JSON payload to a Plivo endpoint with HTTP Basic auth
// and returns a "status_code:.., response:.." summary. A non-2xx status is
// returned as an error alongside the summary. Network errors are retried.
func postPlivoJSON(ctx context.Context, client *http.Client, cfg *models.PlivoRequestConfig,
	endpoint string, payload map[string]interface{}) (string, error) {

	if client == nil {
		return "", fmt.Errorf("http client not found")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	retryTimes := 1
	if cfg.RetryTimes > 0 {
		retryTimes = cfg.RetryTimes
	}
	retrySleep := 200 * time.Millisecond
	if cfg.RetrySleep > 0 {
		retrySleep = time.Duration(cfg.RetrySleep) * time.Millisecond
	}

	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.AuthID+":"+cfg.AuthToken))

	var lastErr error
	for i := 0; i < retryTimes; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", auth)

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			logger.Errorf("send_plivo: http_call=fail url=%s error=%v times=%d", endpoint, err, i+1)
			time.Sleep(retrySleep)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		summary := fmt.Sprintf("status_code:%d, response:%s", resp.StatusCode, string(respBody))
		logger.Infof("send_plivo: http_call=succ url=%s response_code=%d body=%s", endpoint, resp.StatusCode, string(respBody))

		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			return summary, nil
		}
		return summary, fmt.Errorf("plivo returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return fmt.Sprintf("request failed: %v", lastErr), fmt.Errorf("all retries failed, last error: %v", lastErr)
}

// notifyPlivo sends the given payload to every destination in sendtos and
// aggregates the per-target responses and errors, matching the multi-target
// pattern used by the other providers.
func notifyPlivo(ctx context.Context, req *NotifyRequest, endpoint string,
	buildPayload func(dst string) map[string]interface{}) *NotifyResult {

	cfg := req.Config.RequestConfig.PlivoRequestConfig
	var responses, failed []string
	for _, to := range req.Sendtos {
		dst := normalizePlivoNumber(to)
		resp, err := postPlivoJSON(ctx, req.HttpClient, cfg, endpoint, buildPayload(dst))
		responses = append(responses, fmt.Sprintf("%s: %s", dst, resp))
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", dst, err))
		}
	}

	var aggErr error
	if len(failed) > 0 {
		aggErr = fmt.Errorf("%s", strings.Join(failed, " | "))
	}
	return &NotifyResult{
		Target:   strings.Join(req.Sendtos, ","),
		Response: strings.Join(responses, "; "),
		Err:      aggErr,
	}
}
