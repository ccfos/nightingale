package callback

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

type HTTPConfig struct {
	URL           string            `json:"url"`
	Method        string            `json:"method,omitempty"`
	Body          string            `json:"body,omitempty"`
	Headers       map[string]string `json:"headers"`
	AuthUsername  string            `json:"auth_username"`
	AuthPassword  string            `json:"auth_password"`
	Timeout       int               `json:"timeout"` // 单位:ms
	SkipSSLVerify bool              `json:"skip_ssl_verify"`
	Proxy         string            `json:"proxy"`
	Client        *http.Client      `json:"-"`
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

func (c *CallbackConfig) Process(ctx *ctx.Context, event *models.AlertCurEvent) {
	if c.Client == nil {
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipSSLVerify},
			Proxy:           http.ProxyFromEnvironment,
		}

		c.Client = &http.Client{
			Timeout:   time.Duration(c.Timeout) * time.Millisecond,
			Transport: transport,
		}
	}

	// 设置 请求headers
	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"
	for k, v := range c.Headers {
		headers[k] = v
	}

	// 将 event 转换为 json
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

	// 设置 headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 设置 basic auth
	if c.AuthUsername != "" && c.AuthPassword != "" {
		req.SetBasicAuth(c.AuthUsername, c.AuthPassword)
	}

	// 发送 post 请求
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
}
