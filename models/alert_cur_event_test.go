package models

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// TestLoggableSnapshot 覆盖：nil 事件、剔除重字段、不篡改原事件。
func TestLoggableSnapshot(t *testing.T) {
	// nil 事件返回空串（约定为「本工作流没有事件」信号）
	var nilEvent *AlertCurEvent
	assert.Equal(t, "", nilEvent.LoggableSnapshot())

	e := &AlertCurEvent{
		Hash:            "h1",
		Severity:        2,
		TagsJSON:        []string{"a=b"},
		ShotImageBase64: map[string]string{"panel": "BASE64IMGDATA"},
		ExtraInfoMap:    []map[string]string{{"enrich_key": "enrich_val"}},
		NotifyRules:     []*EventNotifyRule{{Id: 1, Name: "rule-A"}},
	}
	snap := e.LoggableSnapshot()

	assert.NotEmpty(t, snap)
	// 唯一被剔除的超大字段
	assert.NotContains(t, snap, "BASE64IMGDATA", "base64 截图应被剔除")
	// 普通字段保留
	assert.Contains(t, snap, "\"severity\":2")
	// event_update 可改写的字段必须保留，否则真实改动会被漏记成 no-change
	assert.Contains(t, snap, "enrich_val", "ExtraInfoMap 应保留")
	assert.Contains(t, snap, "rule-A", "NotifyRules 应保留")

	// 原事件不被篡改
	assert.NotNil(t, e.ShotImageBase64)
	assert.NotNil(t, e.ExtraInfoMap)
}

// TestDiffEventSnapshot 表驱动覆盖三种空串语义与字段增删改。
func TestDiffEventSnapshot(t *testing.T) {
	base := &AlertCurEvent{Hash: "h1", Severity: 2}
	baseSnap := base.LoggableSnapshot()

	// 各构造一个「相对 base 改了某字段」的快照
	modified := &AlertCurEvent{Hash: "h1", Severity: 1} // severity 改
	added := &AlertCurEvent{Hash: "h1", Severity: 2, NotifyChannelsJSON: []string{"dingtalk"}}
	baseWithChan := &AlertCurEvent{Hash: "h1", Severity: 2, NotifyChannelsJSON: []string{"dingtalk"}}

	tests := []struct {
		name       string
		before     string
		after      string
		dropped    bool
		wantFields []string // 期望出现的 Field（顺序无关）
		wantFmt    string   // 非空时校验 FormatEventChanges 结果
	}{
		{
			name:    "no_event_workflow", // 两侧空串且未 drop：ai.agent 等非告警工作流
			before:  "", after: "", dropped: false,
			wantFields: nil,
			wantFmt:    "no-change",
		},
		{
			name:    "dropped_by_flag",
			before:  baseSnap, after: "", dropped: true,
			wantFields: []string{"_event"},
		},
		{
			name:    "drop_priority_over_empty", // dropped 优先于两侧空串短路
			before:  "", after: "", dropped: true,
			wantFields: []string{"_event"},
		},
		{
			name:    "marshal_error_sentinel", // 序列化失败(非空哨兵)：快照不可用，非丢弃
			before:  baseSnap, after: eventSnapshotMarshalErr, dropped: false,
			wantFields: []string{"_snapshot"},
		},
		{
			name:    "no_change",
			before:  baseSnap, after: baseSnap, dropped: false,
			wantFields: nil,
			wantFmt:    "no-change",
		},
		{
			name:    "field_modified",
			before:  baseSnap, after: modified.LoggableSnapshot(), dropped: false,
			wantFields: []string{"severity"},
		},
		{
			name:    "field_added", // omitempty 字段 nil→有值
			before:  baseSnap, after: added.LoggableSnapshot(), dropped: false,
			wantFields: []string{"notify_channels"},
		},
		{
			name:    "field_removed", // omitempty 字段 有值→nil
			before:  baseWithChan.LoggableSnapshot(), after: baseSnap, dropped: false,
			wantFields: []string{"notify_channels"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := DiffEventSnapshot(tt.before, tt.after, tt.dropped)

			gotFields := make([]string, 0, len(changes))
			for _, c := range changes {
				gotFields = append(gotFields, c.Field)
			}
			assert.ElementsMatch(t, tt.wantFields, gotFields)

			if tt.wantFmt != "" {
				assert.Equal(t, tt.wantFmt, FormatEventChanges(changes))
			}
		})
	}

	// 语义细节：dropped 与 marshal-error 的 marker 内容
	dropped := DiffEventSnapshot(baseSnap, "", true)
	assert.Equal(t, EventFieldChange{Field: "_event", Before: "present", After: "dropped"}, dropped[0])
	unavail := DiffEventSnapshot(baseSnap, eventSnapshotMarshalErr, false)
	assert.Equal(t, "unavailable", unavail[0].After)
}

// TestDiffEventSnapshot_StableOrder 校验输出按字段名排序、多次一致（防 map 迭代随机）。
func TestDiffEventSnapshot_StableOrder(t *testing.T) {
	before := (&AlertCurEvent{Severity: 2, TargetIdent: "a", RuleName: "r1", RunbookUrl: "u1"}).LoggableSnapshot()
	after := (&AlertCurEvent{Severity: 1, TargetIdent: "b", RuleName: "r2", RunbookUrl: "u2"}).LoggableSnapshot()

	first := FormatEventChanges(DiffEventSnapshot(before, after, false))
	for i := 0; i < 200; i++ {
		assert.Equal(t, first, FormatEventChanges(DiffEventSnapshot(before, after, false)))
	}

	// 排序确认：字段名升序
	changes := DiffEventSnapshot(before, after, false)
	for i := 1; i < len(changes); i++ {
		assert.LessOrEqual(t, changes[i-1].Field, changes[i].Field)
	}
}

// TestFormatEventChanges 覆盖空列表、格式化、超长截断且不切断多字节字符。
func TestFormatEventChanges(t *testing.T) {
	assert.Equal(t, "no-change", FormatEventChanges(nil))
	assert.Equal(t, "no-change", FormatEventChanges([]EventFieldChange{}))

	assert.Equal(t, "severity:2→1", FormatEventChanges([]EventFieldChange{{Field: "severity", Before: "2", After: "1"}}))

	// ai_summary 场景：整段中文摘要 → 截断、总长受限、UTF-8 合法
	huge := strings.Repeat("很长的AI摘要文本", 500)
	before := (&AlertCurEvent{AnnotationsJSON: map[string]string{"ai_summary": "short"}}).LoggableSnapshot()
	after := (&AlertCurEvent{AnnotationsJSON: map[string]string{"ai_summary": huge}}).LoggableSnapshot()
	s := FormatEventChanges(DiffEventSnapshot(before, after, false))
	assert.LessOrEqual(t, len(s), maxChangeStrLen+3, "整串应被截断到上限")
	assert.True(t, utf8.ValidString(s), "截断后应为合法 UTF-8")
}

// TestTruncateForLog 直接覆盖截断函数的边界与多字节安全。
func TestTruncateForLog(t *testing.T) {
	// 未超限：原样返回
	assert.Equal(t, "abc", truncateForLog("abc", 10))
	assert.Equal(t, "abcde", truncateForLog("abcde", 5))

	// 超限：加省略号
	got := truncateForLog("abcdefghij", 5)
	assert.Equal(t, "abcde...", got)

	// 多字节（每个「摘」3 字节）：在字节边界回退，不切断字符
	multi := strings.Repeat("摘", 100) // 300 字节
	tr := truncateForLog(multi, 256)   // 256 不是 3 的整数倍，会落在字符中间
	assert.True(t, utf8.ValidString(tr), "不应切断多字节字符")
	assert.True(t, strings.HasSuffix(tr, "..."))
	// 回退到 255 字节(85 个「摘」) + "..."
	assert.Equal(t, strings.Repeat("摘", 85)+"...", tr)
}
