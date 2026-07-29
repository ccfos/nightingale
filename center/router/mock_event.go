package router

import (
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

// mockEventSpec 描述一条「样例事件」中随调用方变化的部分。
// 其余字段（业务组、注解、时间戳、通知计数等）由 newMockEvent 统一填充，
// 保证通知规则测试与工作流试跑看到的样例事件形状一致。
type mockEventSpec struct {
	RuleName     string
	RuleNote     string
	Hash         string
	Severity     int
	IsRecovered  bool
	PromQL       string
	TriggerValue string
	// ExtraTags 不需要包含 rulename，newMockEvent 会按事件的通用约定补在最前面
	ExtraTags []string
}

// newMockEvent 构造各处「测试 / 试跑」共用的模拟告警事件。
// 该事件只在内存里流转、不落库，仅用于在没有真实历史事件的环境里验证配置。
func newMockEvent(spec mockEventSpec) *models.AlertCurEvent {
	now := time.Now().Unix()

	// 调用方给的级别可能来自用户输入，越界时回落到 Warning
	severity := spec.Severity
	if severity < 1 || severity > 3 {
		severity = 2
	}

	tags := append([]string{"rulename=" + spec.RuleName}, spec.ExtraTags...)

	event := &models.AlertCurEvent{
		Cate:             "prometheus",
		GroupName:        "Default Busi Group",
		Hash:             spec.Hash,
		RuleName:         spec.RuleName,
		RuleNote:         spec.RuleNote,
		Severity:         severity,
		PromQl:           spec.PromQL,
		TriggerTime:      now,
		TriggerValue:     spec.TriggerValue,
		TriggerValues:    spec.TriggerValue,
		Tags:             strings.Join(tags, ",,"),
		TagsJSON:         tags,
		Annotations:      "{}",
		AnnotationsJSON:  map[string]string{},
		FirstTriggerTime: now,
		LastEvalTime:     now,
		NotifyCurNumber:  1,
		IsRecovered:      spec.IsRecovered,
	}
	// 恢复事件必须带上恢复时间，否则前端事件详情会把零值渲染成 1970-01-01
	if spec.IsRecovered {
		event.RecoverTime = now
	}
	event.SetTagsMap()
	return event
}
