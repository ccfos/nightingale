package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestNewMockEventFillsSkeleton(t *testing.T) {
	e := newMockEvent(mockEventSpec{
		RuleName:     "rn",
		RuleNote:     "note",
		Hash:         "h",
		Severity:     1,
		PromQL:       "up == 0",
		TriggerValue: "1",
		ExtraTags:    []string{"ident=mock-host-01"},
	})

	if e.Cate != "prometheus" || e.GroupName != "Default Busi Group" {
		t.Fatalf("unexpected skeleton: cate=%q group=%q", e.Cate, e.GroupName)
	}
	// rulename 必须自动补在最前面，调用方不需要自己拼
	if got, want := e.TagsJSON[0], "rulename=rn"; got != want {
		t.Fatalf("first tag = %q, want %q", got, want)
	}
	if e.TagsMap["ident"] != "mock-host-01" {
		t.Fatalf("TagsMap not populated: %v", e.TagsMap)
	}
	if e.TriggerValue != "1" || e.TriggerValues != "1" {
		t.Fatalf("trigger value not mirrored: %q / %q", e.TriggerValue, e.TriggerValues)
	}
	if e.TriggerTime == 0 || e.FirstTriggerTime == 0 || e.LastEvalTime == 0 {
		t.Fatal("timestamps should be filled")
	}
	// 非恢复事件不能带恢复时间，否则前端会渲染出一个莫名其妙的恢复时刻
	if e.RecoverTime != 0 {
		t.Fatalf("RecoverTime = %d, want 0 for a firing event", e.RecoverTime)
	}
}

func TestNewMockEventRecoveredHasRecoverTime(t *testing.T) {
	e := newMockEvent(mockEventSpec{RuleName: "rn", IsRecovered: true})
	if !e.IsRecovered || e.RecoverTime == 0 {
		t.Fatalf("recovered mock event must carry RecoverTime, got %d", e.RecoverTime)
	}
}

func TestNewMockEventSeverityFallback(t *testing.T) {
	// 越界或缺省的级别一律回落到 Warning，避免把非法值透传给处理器
	for _, in := range []int{0, -1, 4, 99} {
		if got := newMockEvent(mockEventSpec{RuleName: "rn", Severity: in}).Severity; got != 2 {
			t.Fatalf("severity %d => %d, want 2", in, got)
		}
	}
	for _, in := range []int{1, 2, 3} {
		if got := newMockEvent(mockEventSpec{RuleName: "rn", Severity: in}).Severity; got != in {
			t.Fatalf("severity %d => %d, want unchanged", in, got)
		}
	}
}

// 通知规则的样例事件是已发布行为，抽公共骨架后这几项必须保持原样
func TestBuildNotifyTestMockEventKeepsContract(t *testing.T) {
	e := buildNotifyTestMockEvent("en_US", models.NotifyConfig{Severities: []int{3, 1, 2}})

	if e.Hash != "notify-rule-test-mock-event" {
		t.Fatalf("hash = %q", e.Hash)
	}
	// severity 取勾选级别里最严重的那个
	if e.Severity != 1 {
		t.Fatalf("severity = %d, want 1 (min of 3,1,2)", e.Severity)
	}
	if e.PromQl != "cpu_usage_active > 80" || e.TriggerValue != "81.5" {
		t.Fatalf("unexpected promql/value: %q / %q", e.PromQl, e.TriggerValue)
	}
	want := []string{"rulename=" + e.RuleName, "ident=mock-host-01", "source=notify-rule-test"}
	if len(e.TagsJSON) != len(want) {
		t.Fatalf("tags = %v, want %v", e.TagsJSON, want)
	}
	for i := range want {
		if e.TagsJSON[i] != want[i] {
			t.Fatalf("tags = %v, want %v", e.TagsJSON, want)
		}
	}
	if e.IsRecovered || e.RecoverTime != 0 {
		t.Fatal("notify mock event should be a firing event")
	}
}

func TestBuildNotifyTestMockEventDefaultSeverity(t *testing.T) {
	if got := buildNotifyTestMockEvent("en_US", models.NotifyConfig{}).Severity; got != 2 {
		t.Fatalf("severity = %d, want 2 when no severities configured", got)
	}
}

// 工作流试跑的样例事件必须带上处理器常用的标签，否则文档里的示例片段跑不出结果
func TestBuildPipelineTestMockEventTagsCoverProcessorKeys(t *testing.T) {
	e := buildPipelineTestMockEvent("en_US", MockEventForm{MockSeverity: 3, MockIsRecovered: true})

	if e.Hash != "event-pipeline-test-mock-event" {
		t.Fatalf("hash = %q", e.Hash)
	}
	if e.Severity != 3 || !e.IsRecovered || e.RecoverTime == 0 {
		t.Fatalf("overrides not applied: severity=%d recovered=%v recoverTime=%d", e.Severity, e.IsRecovered, e.RecoverTime)
	}
	for _, key := range []string{"ident", "env", "service", "__name__", "rulename"} {
		if _, ok := e.TagsMap[key]; !ok {
			t.Fatalf("mock event missing tag %q, TagsMap=%v", key, e.TagsMap)
		}
	}
}

func TestBuildPipelineTestMockEventDefaults(t *testing.T) {
	e := buildPipelineTestMockEvent("en_US", MockEventForm{})
	if e.Severity != 2 {
		t.Fatalf("severity = %d, want 2 by default", e.Severity)
	}
	if e.IsRecovered || e.RecoverTime != 0 {
		t.Fatal("default mock event should be a firing event")
	}
}

// 媒介测试的样例事件：级别与恢复态由用户在弹窗里调，其余字段固定
func TestBuildChannelTestMockEvent(t *testing.T) {
	e := buildChannelTestMockEvent("en_US", MockEventForm{MockSeverity: 1, MockIsRecovered: true})

	if e.Hash != "notify-channel-test-mock-event" {
		t.Fatalf("hash = %q", e.Hash)
	}
	if e.Severity != 1 || !e.IsRecovered || e.RecoverTime == 0 {
		t.Fatalf("overrides not applied: severity=%d recovered=%v recoverTime=%d", e.Severity, e.IsRecovered, e.RecoverTime)
	}
	// 来源标签用于在收到的消息里区分是哪个入口发的测试
	if got := e.TagsMap["source"]; got != "notify-channel-test" {
		t.Fatalf("source tag = %q, want notify-channel-test", got)
	}
}

func TestBuildChannelTestMockEventDefaults(t *testing.T) {
	e := buildChannelTestMockEvent("en_US", MockEventForm{})
	if e.Severity != 2 {
		t.Fatalf("severity = %d, want 2 by default", e.Severity)
	}
	if e.IsRecovered || e.RecoverTime != 0 {
		t.Fatal("default mock event should be a firing event")
	}
}

// 模板预览的样例事件必须让 $labels（来自 TagsMap）可用，否则文档里的
// {{$event.TagsMap.instance}} 这类示例在预览里会直接报 nil map
func TestBuildTemplatePreviewMockEventHasTagsMap(t *testing.T) {
	e := buildTemplatePreviewMockEvent("en_US", MockEventForm{MockSeverity: 3})

	if e.Hash != "message-template-preview-mock-event" {
		t.Fatalf("hash = %q", e.Hash)
	}
	if e.Severity != 3 {
		t.Fatalf("severity = %d, want 3", e.Severity)
	}
	if len(e.TagsMap) == 0 {
		t.Fatal("TagsMap is empty, $labels would be unusable in preview")
	}
	for _, key := range []string{"ident", "rulename", "source"} {
		if _, ok := e.TagsMap[key]; !ok {
			t.Fatalf("mock event missing tag %q, TagsMap=%v", key, e.TagsMap)
		}
	}
}
