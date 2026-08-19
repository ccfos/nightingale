package callback

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/common"
	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/utils"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

// HTTPHeaders accepts both {"X-Token":"v"} and the UI form [{"key":"X-Token","value":"v"}].
type HTTPHeaders map[string]string

func (h *HTTPHeaders) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		return nil
	}

	switch data[0] {
	case '{':
		var asMap map[string]string
		if err := json.Unmarshal(data, &asMap); err != nil {
			return err
		}
		*h = asMap
		return nil
	case '[':
		var pairs []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(data, &pairs); err != nil {
			return err
		}
		out := make(HTTPHeaders, len(pairs))
		for _, p := range pairs {
			if p.Key == "" {
				continue
			}
			out[p.Key] = p.Value
		}
		*h = out
		return nil
	default:
		return fmt.Errorf("header: expected object or array")
	}
}

type HTTPConfig struct {
	URL           string       `json:"url"`
	Method        string       `json:"method,omitempty"`
	Body          string       `json:"body,omitempty"`
	Headers       HTTPHeaders  `json:"header"`
	AuthUsername  string       `json:"auth_username"`
	AuthPassword  string       `json:"auth_password"`
	Timeout       int          `json:"timeout"` // 单位:ms
	SkipSSLVerify bool         `json:"skip_ssl_verify"`
	Proxy         string       `json:"proxy"`
	Client        *http.Client `json:"-"`
}

// RelabelConfig
type CallbackConfig struct {
	HTTPConfig
}

func init() {
	models.RegisterProcessor("callback", &CallbackConfig{})
}

func (c *CallbackConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*CallbackConfig](settings)
	return result, err
}

func (c *CallbackConfig) Process(ctx *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	event := wfCtx.Event
	if c.Client == nil {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipSSLVerify},
		}

		if c.Proxy != "" {
			proxyURL, err := url.Parse(c.Proxy)
			if err != nil {
				return wfCtx, "", fmt.Errorf("failed to parse proxy url: %v processor: %v", err, c)
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

	url, err := utils.TplRender(wfCtx, c.URL)
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to render url template: %v processor: %v", err, c)
	}

	body, err := json.Marshal(event)
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to marshal event: %v processor: %v", err, c)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to create request: %v processor: %v", err, c)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if c.AuthUsername != "" && c.AuthPassword != "" {
		req.SetBasicAuth(c.AuthUsername, c.AuthPassword)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to send request: %v processor: %v", err, c)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to read response body: %v processor: %v", err, c)
	}

	logger.Debugf("callback processor response body: %s", string(b))
	return wfCtx, "callback success", nil
}
