package aisummary

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/callback"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newFakeOpenAIServer starts an httptest server that answers an OpenAI-style
// chat completion. It records the last request body so tests can assert what
// model/prompt was sent. summary is the content returned to the caller.
func newFakeOpenAIServer(t *testing.T, summary string, gotBody *[]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if gotBody != nil {
			*gotBody = body
		}
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"role": "assistant", "content": summary}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func newTestWfCtx() *models.WorkflowContext {
	return &models.WorkflowContext{
		Event: &models.AlertCurEvent{
			RuleName:        "Test Rule",
			Severity:        1,
			TagsMap:         map[string]string{"host": "test-host"},
			AnnotationsJSON: map[string]string{"description": "Test alert"},
		},
		Inputs: map[string]string{},
	}
}

// TestAISummary_InlineMode verifies the backward-compatible path: with
// LLMConfigId==0 the node uses the inline URL/model and produces a summary.
func TestAISummary_InlineMode(t *testing.T) {
	var gotBody []byte
	srv := newFakeOpenAIServer(t, "inline summary", &gotBody)
	defer srv.Close()

	config := &AISummaryConfig{
		HTTPConfig:     callback.HTTPConfig{URL: srv.URL, Timeout: 5000},
		ModelName:      "inline-model",
		APIKey:         "sk-inline",
		PromptTemplate: "rule: {{$event.RuleName}}",
	}

	result, _, err := config.Process(&ctx.Context{}, newTestWfCtx())
	assert.NoError(t, err)
	assert.Equal(t, "inline summary", result.Event.AnnotationsJSON["ai_summary"])
	assert.Contains(t, string(gotBody), "inline-model")
}

// TestAISummary_ReuseLLMConfig verifies that with LLMConfigId>0 the node loads
// the centralized ai_llm_config and calls its endpoint/model.
func TestAISummary_ReuseLLMConfig(t *testing.T) {
	var gotBody []byte
	srv := newFakeOpenAIServer(t, "centralized summary", &gotBody)
	defer srv.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.AILLMConfig{}))
	dbCtx := &ctx.Context{DB: db}

	llmCfg := &models.AILLMConfig{
		Name:    "central",
		APIType: "openai",
		APIURL:  srv.URL + "/chat/completions",
		APIKey:  "sk-central",
		Model:   "central-model",
		Enabled: true,
	}
	assert.NoError(t, db.Create(llmCfg).Error)
	assert.NotZero(t, llmCfg.Id)

	config := &AISummaryConfig{
		LLMConfigId:    llmCfg.Id,
		PromptTemplate: "rule: {{$event.RuleName}}",
	}

	result, _, err := config.Process(dbCtx, newTestWfCtx())
	assert.NoError(t, err)
	assert.Equal(t, "centralized summary", result.Event.AnnotationsJSON["ai_summary"])
	assert.Contains(t, string(gotBody), "central-model")
}

// TestAISummary_LLMConfigNotFoundOrDisabled verifies clear errors instead of panics.
func TestAISummary_LLMConfigNotFoundOrDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.AILLMConfig{}))
	dbCtx := &ctx.Context{DB: db}

	// not found
	cfgMissing := &AISummaryConfig{LLMConfigId: 999, PromptTemplate: "x"}
	_, _, err = cfgMissing.Process(dbCtx, newTestWfCtx())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	// disabled
	disabled := &models.AILLMConfig{Name: "off", APIType: "openai", APIURL: "http://x/chat/completions", APIKey: "k", Model: "m", Enabled: false}
	assert.NoError(t, db.Create(disabled).Error)
	cfgDisabled := &AISummaryConfig{LLMConfigId: disabled.Id, PromptTemplate: "x"}
	_, _, err = cfgDisabled.Process(dbCtx, newTestWfCtx())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestAISummaryConfig_Process(t *testing.T) {
	srv := newFakeOpenAIServer(t, "generated summary", nil)
	defer srv.Close()

	// 创建测试配置
	config := &AISummaryConfig{
		HTTPConfig: callback.HTTPConfig{
			URL:           srv.URL,
			Timeout:       30000,
			SkipSSLVerify: true,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
		ModelName:      "gemini-2.0-flash",
		APIKey:         "*",
		PromptTemplate: "告警规则：{{$event.RuleName}}\n严重程度：{{$event.Severity}}",
		CustomParams: map[string]interface{}{
			"temperature": 0.7,
			"max_tokens":  2000,
			"top_p":       0.9,
		},
	}

	// 创建测试事件
	event := &models.AlertCurEvent{
		RuleName: "Test Rule",
		Severity: 1,
		TagsMap: map[string]string{
			"host": "test-host",
		},
		AnnotationsJSON: map[string]string{
			"description": "Test alert",
		},
	}

	// 创建 WorkflowContext
	wfCtx := &models.WorkflowContext{
		Event:  event,
		Inputs: map[string]string{},
	}

	// 测试模板处理
	eventInfo, err := config.prepareEventInfo(wfCtx)
	assert.NoError(t, err)
	assert.Contains(t, eventInfo, "Test Rule")
	assert.Contains(t, eventInfo, "1")

	// 测试配置初始化
	processor, err := config.Init(config)
	assert.NoError(t, err)
	assert.NotNil(t, processor)

	// 测试处理函数
	result, _, err := processor.Process(&ctx.Context{}, wfCtx)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Event.AnnotationsJSON["ai_summary"])

	// 展示处理结果
	t.Log("\n=== 处理结果 ===")
	t.Logf("告警规则: %s", result.Event.RuleName)
	t.Logf("严重程度: %d", result.Event.Severity)
	t.Logf("标签: %v", result.Event.TagsMap)
	t.Logf("原始注释: %v", result.Event.AnnotationsJSON["description"])
	t.Logf("AI总结: %s", result.Event.AnnotationsJSON["ai_summary"])
}

func TestConvertCustomParam(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
		hasError bool
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: nil,
			hasError: false,
		},
		{
			name:     "string number to int64",
			input:    "123",
			expected: int64(123),
			hasError: false,
		},
		{
			name:     "string float to float64",
			input:    "123.45",
			expected: 123.45,
			hasError: false,
		},
		{
			name:     "string boolean to bool",
			input:    "true",
			expected: true,
			hasError: false,
		},
		{
			name:     "string false to bool",
			input:    "false",
			expected: false,
			hasError: false,
		},
		{
			name:     "JSON array string to slice",
			input:    `["a", "b", "c"]`,
			expected: []interface{}{"a", "b", "c"},
			hasError: false,
		},
		{
			name:     "JSON object string to map",
			input:    `{"key": "value", "num": 123}`,
			expected: map[string]interface{}{"key": "value", "num": float64(123)},
			hasError: false,
		},
		{
			name:     "plain string remains string",
			input:    "hello world",
			expected: "hello world",
			hasError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			converted, err := convertCustomParam(test.input)
			if test.hasError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.expected, converted)
		})
	}
}
