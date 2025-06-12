package eventupdate

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/callback"
	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

// RelabelConfig
type EventUpdateConfig struct {
	callback.HTTPConfig
}

func init() {
	models.RegisterProcessor("event_update", &EventUpdateConfig{})
}

func (c *EventUpdateConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*EventUpdateConfig](settings)
	return result, err
}

func (c *EventUpdateConfig) Process(ctx *ctx.Context, event *models.AlertCurEvent) (*models.AlertCurEvent, string, error) {
	if c.Client == nil {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipSSLVerify},
		}

		if c.Proxy != "" {
			proxyURL, err := url.Parse(c.Proxy)
			if err != nil {
				return event, "", fmt.Errorf("failed to parse proxy url: %v processor: %v", err, c)
			} else {
				transport.Proxy = http.ProxyURL(proxyURL)
			}
		}

		c.Client = &http.Client{
			Timeout:   time.Duration(c.Timeout) * time.Millisecond,
			Transport: transport,
		}
	}

	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"
	for k, v := range c.Headers {
		headers[k] = v
	}

	body, err := json.Marshal(event)
	if err != nil {
		return event, "", fmt.Errorf("failed to marshal event: %v processor: %v", err, c)
	}

	req, err := http.NewRequest("POST", c.URL, strings.NewReader(string(body)))
	if err != nil {
		return event, "", fmt.Errorf("failed to create request: %v processor: %v", err, c)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if c.AuthUsername != "" && c.AuthPassword != "" {
		req.SetBasicAuth(c.AuthUsername, c.AuthPassword)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return event, "", fmt.Errorf("failed to send request: %v processor: %v", err, c)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response body: %v processor: %v", err, c)
	}
	logger.Debugf("event update processor response body: %s", string(b))

	err = json.Unmarshal(b, &event)
	if err != nil {
		return event, "", fmt.Errorf("failed to unmarshal response body: %v processor: %v", err, c)
	}

	return event, "", nil
}
