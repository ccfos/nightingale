package aisummary

import (
	"testing"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/callback"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/assert"
)

func TestAISummaryConfig_Process(t *testing.T) {
	// 创建测试配置
	config := &AISummaryConfig{
		HTTPConfig: callback.HTTPConfig{
			URL:           "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
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
		Event: event,
		Env:   map[string]string{},
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
