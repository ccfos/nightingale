package eventupdate

import (
	"crypto/tls"
	"encoding/json"
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
	models.RegisterProcessor("eventupdate", &EventUpdateConfig{})
}

func (c *EventUpdateConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*EventUpdateConfig](settings)
	return result, err
}

func (c *EventUpdateConfig) Process(ctx *ctx.Context, event *models.AlertCurEvent) {
	if c.Client == nil {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipSSLVerify},
		}

		if c.Proxy != "" {
			proxyURL, err := url.Parse(c.Proxy)
			if err != nil {
				logger.Errorf("failed to parse proxy url: %v", err)
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
		logger.Errorf("failed to marshal event: %v", err)
		return
	}

	req, err := http.NewRequest("POST", c.URL, strings.NewReader(string(body)))
	if err != nil {
		logger.Errorf("failed to create request: %v event: %v", err, event)
		return
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if c.AuthUsername != "" && c.AuthPassword != "" {
		req.SetBasicAuth(c.AuthUsername, c.AuthPassword)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		logger.Errorf("failed to send request: %v event: %v", err, event)
		return
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Errorf("failed to read response body: %v event: %v", err, event)
		return
	}
	logger.Infof("response body: %s", string(b))

	json.Unmarshal(b, &event)
}
