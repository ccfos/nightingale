package aisummary

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/callback"
	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
)

const (
	HTTP_STATUS_SUCCESS_MAX = 299
)

// AISummaryConfig 配置结构体
type AISummaryConfig struct {
	callback.HTTPConfig
	ModelName      string                 `json:"model_name"`
	APIKey         string                 `json:"api_key"`
	PromptTemplate string                 `json:"prompt_template"`
	CustomParams   map[string]interface{} `json:"custom_params"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func init() {
	models.RegisterProcessor("ai_summary", &AISummaryConfig{})
}

func (c *AISummaryConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*AISummaryConfig](settings)
	return result, err
}

func (c *AISummaryConfig) Process(ctx *ctx.Context, wfCtx *models.WorkflowContext) (*models.WorkflowContext, string, error) {
	event := wfCtx.Event
	if c.Client == nil {
		if err := c.initHTTPClient(); err != nil {
			return wfCtx, "", fmt.Errorf("failed to initialize HTTP client: %v processor: %v", err, c)
		}
	}

	// 准备告警事件信息
	eventInfo, err := c.prepareEventInfo(wfCtx)
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to prepare event info: %v processor: %v", err, c)
	}

	// 调用AI模型生成总结
	summary, err := c.generateAISummary(eventInfo)
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to generate AI summary: %v processor: %v", err, c)
	}

	// 将总结添加到annotations字段
	if event.AnnotationsJSON == nil {
		event.AnnotationsJSON = make(map[string]string)
	}
	event.AnnotationsJSON["ai_summary"] = summary

	// 更新Annotations字段
	b, err := json.Marshal(event.AnnotationsJSON)
	if err != nil {
		return wfCtx, "", fmt.Errorf("failed to marshal annotations: %v processor: %v", err, c)
	}
	event.Annotations = string(b)

	return wfCtx, "", nil
}

func (c *AISummaryConfig) initHTTPClient() error {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipSSLVerify},
	}

	if c.Proxy != "" {
		proxyURL, err := url.Parse(c.Proxy)
		if err != nil {
			return fmt.Errorf("failed to parse proxy url: %v", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	c.Client = &http.Client{
		Timeout:   time.Duration(c.Timeout) * time.Millisecond,
		Transport: transport,
	}
	return nil
}

func (c *AISummaryConfig) prepareEventInfo(wfCtx *models.WorkflowContext) (string, error) {
	var defs = []string{
		"{{$event := .Event}}",
		"{{$env := .Env}}",
	}

	text := strings.Join(append(defs, c.PromptTemplate), "")
	t, err := template.New("prompt").Funcs(template.FuncMap(tplx.TemplateFuncMap)).Parse(text)
	if err != nil {
		return "", fmt.Errorf("failed to parse prompt template: %v", err)
	}

	var body bytes.Buffer
	err = t.Execute(&body, wfCtx)
	if err != nil {
		return "", fmt.Errorf("failed to execute prompt template: %v", err)
	}

	return body.String(), nil
}

func (c *AISummaryConfig) generateAISummary(eventInfo string) (string, error) {
	// 构建基础请求参数
	reqParams := map[string]interface{}{
		"model": c.ModelName,
		"messages": []Message{
			{
				Role:    "user",
				Content: eventInfo,
			},
		},
	}

	// 合并自定义参数
	for k, v := range c.CustomParams {
		converted, err := convertCustomParam(v)
		if err != nil {
			return "", fmt.Errorf("failed to convert custom param %s: %v", k, err)
		}
		reqParams[k] = converted
	}

	// 序列化请求体
	jsonData, err := json.Marshal(reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %v", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", c.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	// 发送请求
	resp, err := c.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode > HTTP_STATUS_SUCCESS_MAX {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	// 解析响应
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %v", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from AI model")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// convertCustomParam 将前端传入的参数转换为正确的类型
func convertCustomParam(value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	// 如果是字符串，尝试转换为其他类型
	if str, ok := value.(string); ok {
		// 尝试转换为数字
		if f, err := strconv.ParseFloat(str, 64); err == nil {
			// 检查是否为整数
			if f == float64(int64(f)) {
				return int64(f), nil
			}
			return f, nil
		}

		// 尝试转换为布尔值
		if b, err := strconv.ParseBool(str); err == nil {
			return b, nil
		}

		// 尝试解析为JSON数组
		if strings.HasPrefix(strings.TrimSpace(str), "[") {
			var arr []interface{}
			if err := json.Unmarshal([]byte(str), &arr); err == nil {
				return arr, nil
			}
		}

		// 尝试解析为JSON对象
		if strings.HasPrefix(strings.TrimSpace(str), "{") {
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(str), &obj); err == nil {
				return obj, nil
			}
		}
	}
	return value, nil
}
