package a2a

import (
	"encoding/json"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
)

// thoughtMetadataKey is recognised by Google ADK A2A clients to render content
// as the agent's chain-of-thought rather than a regular response. Other clients
// see it as plain text — no harm done.
//
// The router demultiplexes ReAct raw output into P:"reason" (thoughts) and
// P:"content" (final answer body) before frames reach this bridge — so the
// rule here is simply: reason → mark thought, content → don't. No marker
// detection lives in this file.
const thoughtMetadataKey = "adk_thought"

// n9eContentTypeMetadataKey tags an A2A part whose body is a structured n9e
// payload (e.g. form_select JSON). n9e clients render the widget; generic
// clients see opaque JSON — degraded but not lost.
const n9eContentTypeMetadataKey = "n9e_content_type"

// streamBridge translates aiagent.StreamMessage frames produced by the existing
// ReAct pipeline into A2A events. It maintains separate "in-flight" artifact
// IDs for the message body and the reasoning trace so updates accumulate into
// the same artifact rather than creating one event per delta.
type streamBridge struct {
	execCtx *a2asrv.ExecutorContext
	yield   func(a2a.Event, error) bool

	// artifact IDs allocated lazily on first delta of each kind.
	contentArtifactID   a2a.ArtifactID
	reasoningArtifactID a2a.ArtifactID
}

func newBridge(ec *a2asrv.ExecutorContext, yield func(a2a.Event, error) bool) *streamBridge {
	return &streamBridge{execCtx: ec, yield: yield}
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
	case "step":
		// Status text describing a tool call or workflow step. Surface as a
		// transient working update so clients can render progress.
		//
		// A step frame is also the natural boundary between two ReAct
		// reasoning passes — the next "reason" delta after a tool call is a
		// fresh thought, not a continuation. Reset reasoningArtifactID so
		// forwardReason allocates a new artifact for it; otherwise multi-step
		// thoughts would be appended to a single artifact and clients render
		// them as one undelimited blob.
		b.reasoningArtifactID = ""
		return b.yield(a2a.NewStatusUpdateEvent(b.execCtx, a2a.TaskStateWorking,
			a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(msg.V))), nil)
	}
	// Unknown phase — ignore rather than fail.
	return true
}

func (b *streamBridge) forwardContent(delta string) bool {
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
	if frame.ContentType != "" {
		part.SetMeta(n9eContentTypeMetadataKey, string(frame.ContentType))
	}
	return b.yield(a2a.NewArtifactEvent(b.execCtx, part), nil)
}

func (b *streamBridge) forwardReason(delta string) bool {
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
// failed/canceled terminals). An empty msg keeps the status update
// payload-free.
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
