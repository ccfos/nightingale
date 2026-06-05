package router

import "testing"

func TestParseChatIDFromStreamID(t *testing.T) {
	cases := []struct {
		name     string
		streamID string
		want     string
	}{
		{"new format", "chat-uuid-1234:7:f47ac10b-58cc-4372-a567-0e02b2c3d479", "chat-uuid-1234"},
		{"legacy format", "chat-uuid-1234:f47ac10b-58cc-4372-a567-0e02b2c3d479", "chat-uuid-1234"},
		{"empty", "", ""},
		{"no colon", "chatid-only", ""},
		{"leading colon", ":foo", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseChatIDFromStreamID(tc.streamID)
			if got != tc.want {
				t.Fatalf("parseChatIDFromStreamID(%q) = %q, want %q", tc.streamID, got, tc.want)
			}
		})
	}
}

func TestParseSeqIDFromStreamID(t *testing.T) {
	cases := []struct {
		name     string
		streamID string
		want     int64
	}{
		{"new format", "chat-uuid-1234:7:f47ac10b-58cc-4372-a567-0e02b2c3d479", 7},
		{"new format large seq", "chat-uuid-1234:9999999999:abc", 9999999999},
		{"legacy 2-segment", "chat-uuid-1234:f47ac10b-58cc-4372-a567-0e02b2c3d479", 0},
		{"non-numeric middle", "chat-uuid-1234:notanumber:abc", 0},
		{"empty", "", 0},
		{"only colons", "::", 0},
		{"chatid with no rest", "chat-uuid", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSeqIDFromStreamID(tc.streamID)
			if got != tc.want {
				t.Fatalf("parseSeqIDFromStreamID(%q) = %d, want %d", tc.streamID, got, tc.want)
			}
		})
	}
}

// ==================== Stream Segments ====================

// segOp 是驱动 segmentAccumulator 的测试指令：kind 非空 = append，空 = closeCurrent。
type segOp struct {
	kind  string
	delta string
}

func runSegOps(ops []segOp) *segmentAccumulator {
	acc := &segmentAccumulator{}
	for _, op := range ops {
		if op.kind == "" {
			acc.closeCurrent()
		} else {
			acc.append(op.kind, op.delta)
		}
	}
	return acc
}

func TestSegmentAccumulator(t *testing.T) {
	cases := []struct {
		name string
		ops  []segOp
		want []string // 每段 "kind:text"
	}{
		{
			name: "same kind merges",
			ops:  []segOp{{segmentKindReasoning, "a"}, {segmentKindReasoning, "b"}},
			want: []string{"reasoning:ab"},
		},
		{
			name: "kind switch opens new segment",
			ops:  []segOp{{segmentKindReasoning, "think"}, {segmentKindContent, "answer"}},
			want: []string{"reasoning:think", "content:answer"},
		},
		{
			name: "close forces new segment on same kind",
			ops:  []segOp{{segmentKindReasoning, "round1"}, {"", ""}, {segmentKindReasoning, "round2"}},
			want: []string{"reasoning:round1", "reasoning:round2"},
		},
		{
			name: "close on empty is noop",
			ops:  []segOp{{"", ""}, {segmentKindContent, "x"}},
			want: []string{"content:x"},
		},
		{
			name: "double close is noop",
			ops:  []segOp{{segmentKindContent, "x"}, {"", ""}, {"", ""}, {segmentKindContent, "y"}},
			want: []string{"content:x", "content:y"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acc := runSegOps(tc.ops)
			if len(acc.segments) != len(tc.want) {
				t.Fatalf("got %d segments, want %d", len(acc.segments), len(tc.want))
			}
			for i, seg := range acc.segments {
				got := seg.kind + ":" + seg.text.String()
				if got != tc.want[i] {
					t.Fatalf("segment[%d] = %q, want %q", i, got, tc.want[i])
				}
			}
		})
	}
}

func TestAssembleSegmentResponses(t *testing.T) {
	const sid = "chat-1:1:uuid"
	cases := []struct {
		name          string
		ops           []segOp
		finalMarkdown string
		streamed      bool
		want          []string // 每块 "content_type:content"
	}{
		{
			// 多轮工具调用正常收尾：r1 → 过渡语 → [tool_result] → r2 → 终答流式。
			// 终答原始段被权威 markdown 替代，其余按到达顺序成块。
			name:          "multi round streamed",
			ops:           []segOp{{segmentKindReasoning, "r1"}, {segmentKindContent, "我先查询。"}, {"", ""}, {segmentKindReasoning, "r2"}, {segmentKindContent, "raw final"}},
			finalMarkdown: "final",
			streamed:      true,
			want:          []string{"reasoning:r1", "markdown:我先查询。", "reasoning:r2", "markdown:final"},
		},
		{
			// 单轮（无工具）：思考 + 终答。
			name:          "single round",
			ops:           []segOp{{segmentKindReasoning, "think"}, {segmentKindContent, "raw"}},
			finalMarkdown: "final",
			streamed:      true,
			want:          []string{"reasoning:think", "markdown:final"},
		},
		{
			// 不出思考的模型：只有终答。
			name:          "no reasoning",
			ops:           []segOp{{segmentKindContent, "raw"}},
			finalMarkdown: "final",
			streamed:      true,
			want:          []string{"markdown:final"},
		},
		{
			// 空段列表（如 preflight 直接收尾）：保底一个 markdown 块。
			name:          "empty segments",
			ops:           nil,
			finalMarkdown: "",
			streamed:      false,
			want:          []string{"markdown:"},
		},
		{
			// 轮间 "\n\n" 分隔帧自成一段（上一段被 tool_result 收口后才到达），TrimSpace 后过滤。
			name:          "whitespace separator filtered",
			ops:           []segOp{{segmentKindReasoning, "r1"}, {"", ""}, {segmentKindContent, "\n\n"}, {segmentKindReasoning, "r2"}, {segmentKindContent, "raw"}},
			finalMarkdown: "final",
			streamed:      true,
			want:          []string{"reasoning:r1", "reasoning:r2", "markdown:final"},
		},
		{
			// 人在环中断：终答（确认文案）走非流式 Done，段里只有中间轮的思考/过渡语，
			// 全部保留——修复改造前过渡语丢失的问题。
			name:          "interrupt keeps all segments",
			ops:           []segOp{{segmentKindReasoning, "r1"}, {segmentKindContent, "先创建规则。"}, {"", ""}, {segmentKindReasoning, "r2"}},
			finalMarkdown: "请确认是否执行",
			streamed:      false,
			want:          []string{"reasoning:r1", "markdown:先创建规则。", "reasoning:r2", "markdown:请确认是否执行"},
		},
		{
			// 终答后尾随思考（少见 provider 时序）：从尾部搜索丢弃终答原始段，不误删、不重复。
			name:          "trailing reasoning after final body",
			ops:           []segOp{{segmentKindReasoning, "r1"}, {segmentKindContent, "raw final"}, {segmentKindReasoning, "trail"}},
			finalMarkdown: "final",
			streamed:      true,
			want:          []string{"reasoning:r1", "reasoning:trail", "markdown:final"},
		},
		{
			// 连续两轮纯思考（轮间无过渡语）：靠 tool_result 收口分成两段。
			name:          "consecutive reasoning rounds",
			ops:           []segOp{{segmentKindReasoning, "r1"}, {"", ""}, {segmentKindReasoning, "r2"}, {segmentKindContent, "raw"}},
			finalMarkdown: "final",
			streamed:      true,
			want:          []string{"reasoning:r1", "reasoning:r2", "markdown:final"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acc := runSegOps(tc.ops)
			got := assembleSegmentResponses(acc.segments, tc.finalMarkdown, sid, tc.streamed)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d blocks %+v, want %d", len(got), got, len(tc.want))
			}
			for i, r := range got {
				s := string(r.ContentType) + ":" + r.Content
				if s != tc.want[i] {
					t.Fatalf("block[%d] = %q, want %q", i, s, tc.want[i])
				}
				if !r.IsFinish || !r.IsFromAI {
					t.Fatalf("block[%d] flags = finish:%v fromAI:%v, want both true", i, r.IsFinish, r.IsFromAI)
				}
			}
			if got[0].StreamID != sid {
				t.Fatalf("StreamID anchor = %q on first block, want %q", got[0].StreamID, sid)
			}
			for i := 1; i < len(got); i++ {
				if got[i].StreamID != "" {
					t.Fatalf("block[%d] has unexpected StreamID %q", i, got[i].StreamID)
				}
			}
		})
	}
}
