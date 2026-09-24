package a2a

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
)

// thoughtMetadataKey is recognised by Google ADK A2A clients to render content
// as the agent's chain-of-thought rather than a regular response. Other clients
// see it as plain text — no harm done.
//
// The router routes thinking deltas into P:"reason" (thoughts) and body
// deltas into P:"content" (final answer body) before frames reach this
// bridge — so the rule here is simply: reason → mark thought, content →
// don't. No marker detection lives in this file.
const thoughtMetadataKey = "adk_thought"

// n9eContentTypeMetadataKey tags an A2A part whose body is a structured n9e
// payload (e.g. form_select JSON). n9e clients render the widget; generic
// clients see opaque JSON — degraded but not lost.
const n9eContentTypeMetadataKey = "n9e_content_type"

// bridgeFlushInterval 是 content/reason delta 的合并窗口。LLM 流式输出时每秒
// 产生几十条 delta，逐条转发会让下游（SDK processor → store 全量写）背压拖死
// executor（长输出轮次数千事件 × 全量 Task 写，可达数百秒）。窗口内累积、到点
// 合并成一条 artifact update，事件量可降一个数量级。0 = 关闭节流（逐 delta
// 转发），单测用。
const bridgeFlushInterval = 200 * time.Millisecond

// streamBridge translates aiagent.StreamMessage frames produced by the existing
// agent pipeline into A2A events. It maintains separate "in-flight" artifact
// IDs for the message body and the reasoning trace so updates accumulate into
// the same artifact rather than creating one event per delta.
type streamBridge struct {
	execCtx *a2asrv.ExecutorContext
	yield   func(a2a.Event, error) bool

	// artifact IDs allocated lazily on first delta of each kind.
	contentArtifactID   a2a.ArtifactID
	reasoningArtifactID a2a.ArtifactID

	// input_required 帧的记录位：本轮以人在环中断收尾，prompt 是确认问题文本。
	// bridge 不在此处发终态——终态由 executor 统一收口（cancel/failed 优先级
	// 高于 input-required，见 executor.Execute 尾部）。
	inputRequired       bool
	inputRequiredPrompt string

	// flushInterval + 累积缓冲：content/reason delta 先写 buffer，到点/流结束
	// 才合并转发（见 bridgeFlushInterval）。step/response 帧不过节流，立即转发。
	flushInterval     time.Duration
	contentBuf        strings.Builder
	reasonBuf         strings.Builder
	lastContentFlush  time.Time
	lastReasonFlush   time.Time
	contentDeltaCount int
	contentEvents     int
	reasonDeltaCount  int
	reasonEvents      int
}

func newBridge(ec *a2asrv.ExecutorContext, yield func(a2a.Event, error) bool) *streamBridge {
	return &streamBridge{execCtx: ec, yield: yield, flushInterval: bridgeFlushInterval}
}

// Flush 兜底把流结束时还留在缓冲里的 delta 合并发出（LLM 停止输出后不会再
// 有 Forward 触发窗口到期）。executor 在 stream 关闭后、Finalize 前调用。
// 返回 false 表示下游已取消。
func (b *streamBridge) Flush() bool {
	ok := true
	if b.contentBuf.Len() > 0 {
		ok = b.flushContent() && ok
	}
	if b.reasonBuf.Len() > 0 {
		ok = b.flushReason() && ok
	}
	return ok
}

// coalesceStats 返回 (delta 数, 实际事件数)——合并比，供可观测性/排查。
func (b *streamBridge) coalesceStats() (contentDelta, contentEvents, reasonDelta, reasonEvents int) {
	return b.contentDeltaCount, b.contentEvents, b.reasonDeltaCount, b.reasonEvents
}

// Forward emits A2A events for one StreamMessage. Returns false when the
// downstream consumer has cancelled (yield returned false), in which case
// callers should bail out of their read loop.
func (b *streamBridge) Forward(msg aiagent.StreamMessage) bool {
	switch msg.P {
	case "content":
		return b.forwardContent(msg.V)
	case "reason":
		return b.forwardReason(msg.V)
	case aiagent.PhaseResponse:
		return b.forwardResponse(msg.V)
	case aiagent.PhaseInputRequired:
		// 只记录不转发：确认问题文本已经走 content 通道流出过了，这里再发一遍
		// 会让客户端渲染两份。终态映射在 executor 里做。
		b.inputRequired = true
		b.inputRequiredPrompt = msg.V
		return true
	case "step":
		// Status text describing a tool call or workflow step. Surface as a
		// transient working update so clients can render progress.
		//
		// A step frame is also the natural boundary between two agent
		// reasoning passes — the next "reason" delta after a tool call is a
		// fresh thought, not a continuation. Flush the buffered reasoning
		// deltas first (so they land on the OLD artifact), then reset
		// reasoningArtifactID so the next reason delta allocates a new
		// artifact; otherwise coalescing would merge cross-step thoughts into
		// one undelimited blob.
		if b.reasonBuf.Len() > 0 {
			if !b.flushReason() {
				return false
			}
		}
		b.reasoningArtifactID = ""
		return b.yield(a2a.NewStatusUpdateEvent(b.execCtx, a2a.TaskStateWorking,
			a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(msg.V))), nil)
	}
	// Unknown phase — ignore rather than fail.
	return true
}

func (b *streamBridge) forwardContent(delta string) bool {
	b.contentDeltaCount++
	b.contentBuf.WriteString(delta)
	if b.flushInterval <= 0 || time.Since(b.lastContentFlush) >= b.flushInterval {
		return b.flushContent()
	}
	return true
}

// flushContent 把累积的 content delta 合并成一条 artifact 事件发出。
func (b *streamBridge) flushContent() bool {
	if b.contentBuf.Len() == 0 {
		b.lastContentFlush = time.Now()
		return true
	}
	delta := b.contentBuf.String()
	b.contentBuf.Reset()
	b.lastContentFlush = time.Now()
	b.contentEvents++
	if b.contentArtifactID == "" {
		ev := a2a.NewArtifactEvent(b.execCtx, a2a.NewTextPart(delta))
		b.contentArtifactID = ev.Artifact.ID
		return b.yield(ev, nil)
	}
	return b.yield(a2a.NewArtifactUpdateEvent(b.execCtx, b.contentArtifactID, a2a.NewTextPart(delta)), nil)
}

// forwardResponse emits a structured response as its own artifact. Each frame
// is a complete payload (not a delta), so allocate a fresh artifact rather
// than reusing contentArtifactID. Bad JSON / empty content are dropped so a
// single malformed frame can't take down the stream.
func (b *streamBridge) forwardResponse(body string) bool {
	var frame aiagent.ResponseFrame
	if err := json.Unmarshal([]byte(body), &frame); err != nil {
		return true
	}
	if frame.Content == "" {
		return true
	}
	part := a2a.NewTextPart(frame.Content)
	// Tag only machine-readable JSON payloads: the tag is a contract that the
	// part body parses as JSON. Plain-text frames (markdown — e.g. the resume
	// path's "已取消本次改动" notice) must stay untagged or A2A clients
	// json.Unmarshal them and fail on the first multi-byte character.
	if frame.ContentType.IsStructuredPayload() {
		part.SetMeta(n9eContentTypeMetadataKey, string(frame.ContentType))
	}
	return b.yield(a2a.NewArtifactEvent(b.execCtx, part), nil)
}

// InputRequiredPrompt 返回流中 input_required 帧携带的确认问题文本。
// ok=false 表示本轮没有人在环中断。
func (b *streamBridge) InputRequiredPrompt() (string, bool) {
	return b.inputRequiredPrompt, b.inputRequired
}

func (b *streamBridge) forwardReason(delta string) bool {
	b.reasonDeltaCount++
	b.reasonBuf.WriteString(delta)
	if b.flushInterval <= 0 || time.Since(b.lastReasonFlush) >= b.flushInterval {
		return b.flushReason()
	}
	return true
}

// flushReason 把累积的 reason delta 合并成一条带 thought 标记的 artifact 事件发出。
func (b *streamBridge) flushReason() bool {
	if b.reasonBuf.Len() == 0 {
		b.lastReasonFlush = time.Now()
		return true
	}
	delta := b.reasonBuf.String()
	b.reasonBuf.Reset()
	b.lastReasonFlush = time.Now()
	b.reasonEvents++
	part := a2a.NewTextPart(delta)
	part.SetMeta(thoughtMetadataKey, true)
	if b.reasoningArtifactID == "" {
		ev := a2a.NewArtifactEvent(b.execCtx, part)
		b.reasoningArtifactID = ev.Artifact.ID
		return b.yield(ev, nil)
	}
	return b.yield(a2a.NewArtifactUpdateEvent(b.execCtx, b.reasoningArtifactID, part), nil)
}

// Finalize emits the terminal status update for the task, optionally
// attaching a human-readable message (used to surface ErrMsg on
// failed/canceled terminals, and the confirmation prompt on input-required).
// An empty msg keeps the status update payload-free.
//
// We deliberately do NOT emit a separate LastChunk artifact-update event:
// the SDK's taskupdate.Manager rejects ArtifactUpdate events with empty
// Parts (a2a.ErrInvalidAgentResponse "artifact cannot be empty"), which
// would flip the task state to failed even though the conversation
// succeeded. The terminal status update alone is the canonical end-of-
// stream signal in this SDK; clients still know the artifact is final
// because no further deltas arrive after a terminal state.
func (b *streamBridge) Finalize(state a2a.TaskState, msg string) bool {
	var payload *a2a.Message
	if msg != "" {
		payload = a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(msg))
	}
	return b.yield(a2a.NewStatusUpdateEvent(b.execCtx, state, payload), nil)
}
