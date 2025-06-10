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
		Temperature:    0.7,
		CustomParams: map[string]interface{}{
			"max_tokens": 2000,
			"top_p":      0.9,
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

	// 测试模板处理
	eventInfo, err := config.prepareEventInfo(event)
	assert.NoError(t, err)
	assert.Contains(t, eventInfo, "Test Rule")
	assert.Contains(t, eventInfo, "1")

	// 测试配置初始化
	processor, err := config.Init(config)
	assert.NoError(t, err)
	assert.NotNil(t, processor)

	// 测试处理函数
	result := processor.Process(&ctx.Context{}, event)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.AnnotationsJSON["ai_summary"])

	// 展示处理结果
	t.Log("\n=== 处理结果 ===")
	t.Logf("告警规则: %s", result.RuleName)
	t.Logf("严重程度: %d", result.Severity)
	t.Logf("标签: %v", result.TagsMap)
	t.Logf("原始注释: %v", result.AnnotationsJSON["description"])
	t.Logf("AI总结: %s", result.AnnotationsJSON["ai_summary"])
}

func TestAISummaryConfig_Init(t *testing.T) {
	// 测试无效配置
	invalidConfig := &AISummaryConfig{}
	_, err := invalidConfig.Init("invalid")
	assert.Error(t, err)

	// 测试有效配置
	validConfig := &AISummaryConfig{
		HTTPConfig: callback.HTTPConfig{
			URL: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		},
		ModelName:      "gemini-2.0-flash",
		APIKey:         "AIzaSyB-6GAAu9-oKdtH2nvSXdkDd2-HsHNJp_A",
		PromptTemplate: "Test template",
		Temperature:    0.7,
	}

	processor, err := validConfig.Init(validConfig)
	assert.NoError(t, err)
	assert.NotNil(t, processor)
}

func TestAISummaryConfig_GenerateAISummary(t *testing.T) {
	config := &AISummaryConfig{
		HTTPConfig: callback.HTTPConfig{
			URL: "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		},
		ModelName:      "gemini-2.0-flash",
		APIKey:         "AIzaSyB-6GAAu9-oKdtH2nvSXdkDd2-HsHNJp_A",
		PromptTemplate: "Test template",
		Temperature:    0.7,
		CustomParams: map[string]interface{}{
			"max_tokens": 2000,
		},
	}

	// 测试生成总结
	summary, err := config.generateAISummary("Test event info")
	assert.NoError(t, err)
	assert.NotEmpty(t, summary)

	// 展示生成的总结
	t.Log("\n=== 生成的总结 ===")
	t.Logf("总结内容: %s", summary)
}
