package router

import (
	"strings"
	"testing"
)

// feedAll drives the demuxer with a sequence of deltas and concatenates the
// reason / content outputs into two strings. This is the shape callers see —
// they don't care about per-delta segmentation, only the overall split.
func feedAll(d *reactStreamDemuxer, deltas []string) (reason, content string) {
	var r, c strings.Builder
	for _, delta := range deltas {
		rp, cp := d.Feed(delta)
		r.WriteString(rp)
		c.WriteString(cp)
	}
	return r.String(), c.String()
}

func TestDemuxNoMarkerFlowsToReason(t *testing.T) {
	d := &reactStreamDemuxer{}
	reason, content := feedAll(d, []string{
		"Thought: I should check X\n",
		"Action: query\n",
		"Action Input: {}\n",
	})
	if d.FinalSeen() {
		t.Fatal("FinalSeen=true with no marker in input")
	}
	if content != "" {
		t.Fatalf("content should be empty with no marker, got %q", content)
	}
	want := "Thought: I should check X\nAction: query\nAction Input: {}\n"
	if reason != want {
		t.Fatalf("reason mismatch:\n  got:  %q\n  want: %q", reason, want)
	}
}

func TestDemuxSingleDeltaMarkerSplitsCleanly(t *testing.T) {
	d := &reactStreamDemuxer{}
	reason, content := feedAll(d, []string{
		"Thought: I have enough info\nFinal Answer: The answer is 42",
	})
	if !d.FinalSeen() {
		t.Fatal("FinalSeen=false after seeing the marker")
	}
	if reason != "Thought: I have enough info\n" {
		t.Fatalf("reason should stop at the marker, got %q", reason)
	}
	if content != "The answer is 42" {
		t.Fatalf("content should be the post-marker body, got %q", content)
	}
}

// TestDemuxMarkerSpanningDeltaBoundary is the core regression: byte-by-byte
// streaming must still detect the marker even when individual chunks don't
// contain it.
func TestDemuxMarkerSpanningDeltaBoundary(t *testing.T) {
	d := &reactStreamDemuxer{}
	reason, content := feedAll(d, []string{
		"Thought: ok\n",
		"Final ",
		"Answ",
		"er:",
		" The answer is 42",
	})
	if !d.FinalSeen() {
		t.Fatal("FinalSeen=false despite marker spanning deltas")
	}
	if strings.Contains(reason, "Final") {
		t.Fatalf("marker prefix must not leak into reason, got %q", reason)
	}
	if reason != "Thought: ok\n" {
		t.Fatalf("reason should be only the pre-marker thought, got %q", reason)
	}
	if content != "The answer is 42" {
		t.Fatalf("content should be the post-marker body, got %q", content)
	}
}

// TestDemuxPostMarkerDeltasGoToContent covers the realistic case where the
// body itself arrives in many small chunks AFTER the marker — each must
// stream to content, not be buffered until Done.
func TestDemuxPostMarkerDeltasGoToContent(t *testing.T) {
	d := &reactStreamDemuxer{}
	deltas := []string{
		"Thought: done\nFinal Answer:",
		" The",
		" answer",
		" is",
		" 42",
	}
	var got strings.Builder
	for i, delta := range deltas {
		reason, content := d.Feed(delta)
		got.WriteString(content)
		// Only the first delta carries any reason; the rest must be empty.
		if i > 0 && reason != "" {
			t.Fatalf("delta[%d]: reason should be empty after marker, got %q", i, reason)
		}
	}
	if want := "The answer is 42"; got.String() != want {
		t.Fatalf("content stream mismatch:\n  got:  %q\n  want: %q", got.String(), want)
	}
}

// TestDemuxResetEnablesNextIteration mirrors the ReAct multi-iteration loop:
// after a tool_result the router calls Reset and the next iteration's
// Thought/marker pair must be detectable again. Without Reset, the second
// iteration's reasoning would be permanently muted (or routed to content).
func TestDemuxResetEnablesNextIteration(t *testing.T) {
	d := &reactStreamDemuxer{}

	// Iteration 1 hits the marker.
	d.Feed("Thought: one\nFinal Answer: body one")
	if !d.FinalSeen() {
		t.Fatal("iter1: FinalSeen=false")
	}

	// Tool boundary.
	d.Reset()
	if d.FinalSeen() {
		t.Fatal("Reset failed to clear FinalSeen")
	}

	// Iteration 2: regular thought, no marker yet.
	reason, content := d.Feed("Thought: two\nAction: x\n")
	if reason != "Thought: two\nAction: x\n" {
		t.Fatalf("iter2: reason mismatch, got %q", reason)
	}
	if content != "" {
		t.Fatalf("iter2: content should be empty without marker, got %q", content)
	}
}

// TestDemuxNoSpaceAfterMarker covers LLMs that emit "Final Answer:body"
// (no space). The single-space strip is a convenience, not a requirement —
// we must still extract the body unchanged here.
func TestDemuxNoSpaceAfterMarker(t *testing.T) {
	d := &reactStreamDemuxer{}
	_, content := feedAll(d, []string{
		"Thought: x\nFinal Answer:42",
	})
	if content != "42" {
		t.Fatalf("body without leading space should pass through, got %q", content)
	}
}

// TestDemuxMarkerAtVeryStart covers the edge case where the model skips the
// Thought and dives straight into "Final Answer:" as the very first bytes.
// The demuxer must not error or lose data.
func TestDemuxMarkerAtVeryStart(t *testing.T) {
	d := &reactStreamDemuxer{}
	reason, content := feedAll(d, []string{"Final Answer: hello"})
	if !d.FinalSeen() {
		t.Fatal("FinalSeen=false on leading-marker input")
	}
	if reason != "" {
		t.Fatalf("reason should be empty when marker leads, got %q", reason)
	}
	if content != "hello" {
		t.Fatalf("content should be the trimmed body, got %q", content)
	}
}

// TestDemuxByteByByte stresses the tail-retention logic: feeding the entire
// stream one byte at a time should produce the same split as feeding it as
// one chunk. This is the worst case for marker detection because tail must
// correctly accumulate up to len(marker)-1 bytes before a hit.
func TestDemuxByteByByte(t *testing.T) {
	full := "Thought: hmm\nFinal Answer: the body"
	d := &reactStreamDemuxer{}
	var r, c strings.Builder
	for i := 0; i < len(full); i++ {
		rp, cp := d.Feed(full[i : i+1])
		r.WriteString(rp)
		c.WriteString(cp)
	}
	if r.String() != "Thought: hmm\n" {
		t.Fatalf("reason mismatch on byte-by-byte feed, got %q", r.String())
	}
	if c.String() != "the body" {
		t.Fatalf("content mismatch on byte-by-byte feed, got %q", c.String())
	}
}

// TestDemuxCanonicalFormSingleDelta covers the canonical ReAct two-line form
// "Action: Final Answer\nAction Input: <body>" that many models prefer over
// the shorthand. Before canonical support, the demuxer's marker only matched
// the shorthand and this entire input would flow to reason — then Done would
// re-push the same body to content, producing the duplicate-final-answer bug.
func TestDemuxCanonicalFormSingleDelta(t *testing.T) {
	d := &reactStreamDemuxer{}
	reason, content := feedAll(d, []string{
		"Thought: ready to answer\nAction: Final Answer\nAction Input: The answer is 42",
	})
	if !d.FinalSeen() {
		t.Fatal("FinalSeen=false after canonical marker")
	}
	if reason != "Thought: ready to answer\n" {
		t.Fatalf("reason should stop before 'Action:', got %q", reason)
	}
	if content != "The answer is 42" {
		t.Fatalf("content should be the post-marker body, got %q", content)
	}
}

// TestDemuxCanonicalMarkerSpanningDeltas verifies the canonical marker is
// still detected when it arrives split across many small deltas — the
// canonical marker is 34 bytes long, so its prefix-suffix retention must
// hold across many chunks.
func TestDemuxCanonicalMarkerSpanningDeltas(t *testing.T) {
	d := &reactStreamDemuxer{}
	reason, content := feedAll(d, []string{
		"Thought: ok\n",
		"Action: ",
		"Final ",
		"Answer",
		"\nAction ",
		"Input:",
		" body here",
	})
	if !d.FinalSeen() {
		t.Fatal("FinalSeen=false despite canonical marker spanning deltas")
	}
	if strings.Contains(reason, "Action:") {
		t.Fatalf("marker prefix must not leak into reason, got %q", reason)
	}
	if reason != "Thought: ok\n" {
		t.Fatalf("reason should be only the pre-marker thought, got %q", reason)
	}
	if content != "body here" {
		t.Fatalf("content should be the post-marker body, got %q", content)
	}
}

// TestDemuxCanonicalByteByByte feeds the canonical form one byte at a time.
// This is the strictest test of tail retention because the 34-byte marker
// must be reconstructed across 34 separate Feed calls without leaking any
// of its prefix bytes into reason.
func TestDemuxCanonicalByteByByte(t *testing.T) {
	full := "Thought: x\nAction: Final Answer\nAction Input: the body"
	d := &reactStreamDemuxer{}
	var r, c strings.Builder
	for i := 0; i < len(full); i++ {
		rp, cp := d.Feed(full[i : i+1])
		r.WriteString(rp)
		c.WriteString(cp)
	}
	if r.String() != "Thought: x\n" {
		t.Fatalf("reason mismatch on byte-by-byte canonical, got %q", r.String())
	}
	if c.String() != "the body" {
		t.Fatalf("content mismatch on byte-by-byte canonical, got %q", c.String())
	}
}

// TestDemuxCanonicalMultilineBody mirrors the real production scenario from
// the screenshot bug: an ops-troubleshooting answer with multi-line markdown
// (headings, lists, blank lines) following the canonical marker. None of
// the inner whitespace should be touched — only the leading space after the
// marker's trailing colon.
func TestDemuxCanonicalMultilineBody(t *testing.T) {
	body := "## 故障分析报告\n\n1. 问题概述\n\n- 问题描述：磁盘 I/O 延迟\n"
	d := &reactStreamDemuxer{}
	reason, content := feedAll(d, []string{
		"Thought: 我将基于这些证据生成最终报告。\nAction: Final Answer\nAction Input: " + body,
	})
	if !d.FinalSeen() {
		t.Fatal("FinalSeen=false on multiline canonical body")
	}
	if content != body {
		t.Fatalf("multiline body must pass through unchanged:\n  got:  %q\n  want: %q", content, body)
	}
	if !strings.HasPrefix(reason, "Thought:") || strings.Contains(reason, "Action") {
		t.Fatalf("reason should hold only the thought, got %q", reason)
	}
}

// TestDemuxIntermediateActionNotConfusedWithFinal makes sure a non-final
// tool-call iteration ("Action: query_metrics\nAction Input: ...") does NOT
// trip the canonical marker. The marker requires the literal "Final Answer"
// between Action: and \n.
func TestDemuxIntermediateActionNotConfusedWithFinal(t *testing.T) {
	d := &reactStreamDemuxer{}
	reason, content := feedAll(d, []string{
		"Thought: I need data\nAction: query_metrics\nAction Input: {\"q\":\"cpu\"}\n",
	})
	if d.FinalSeen() {
		t.Fatalf("FinalSeen=true on intermediate Action — false positive; reason=%q content=%q", reason, content)
	}
	if content != "" {
		t.Fatalf("content should be empty on intermediate action, got %q", content)
	}
	want := "Thought: I need data\nAction: query_metrics\nAction Input: {\"q\":\"cpu\"}\n"
	if reason != want {
		t.Fatalf("reason mismatch on intermediate action:\n  got:  %q\n  want: %q", reason, want)
	}
}
