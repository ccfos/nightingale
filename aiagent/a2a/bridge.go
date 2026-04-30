package a2a

import (
	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
)

// thoughtMetadataKey is recognised by Google ADK A2A clients to render content
// as the agent's chain-of-thought rather than a regular response. Other clients
// see it as plain text — no harm done.
const thoughtMetadataKey = "adk_thought"

// streamBridge translates aiagent.StreamMessage frames produced by the existing
// ReAct pipeline into A2A events. It maintains separate "in-flight" artifact
// IDs for the message body and the reasoning trace so updates accumulate into
// the same artifact rather than creating one event per delta.
type streamBridge struct {
	execCtx *a2asrv.ExecutorContext
	yield   func(a2a.Event, error) bool

	// artifact IDs allocated lazily on first delta of each kind.
	contentArtifactID a2a.ArtifactID
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
	case "step":
		// Status text describing a tool call or workflow step. Surface as a
		// transient working update so clients can render progress.
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

// End marks the in-flight artifacts as last-chunk and emits a terminal status
// update. err == nil means success → TaskStateCompleted; otherwise failed.
func (b *streamBridge) End(err error) bool {
	// Mark accumulated artifacts as final so clients can free buffers.
	if b.contentArtifactID != "" {
		ev := a2a.NewArtifactUpdateEvent(b.execCtx, b.contentArtifactID)
		ev.LastChunk = true
		if !b.yield(ev, nil) {
			return false
		}
	}
	if b.reasoningArtifactID != "" {
		ev := a2a.NewArtifactUpdateEvent(b.execCtx, b.reasoningArtifactID)
		ev.LastChunk = true
		if !b.yield(ev, nil) {
			return false
		}
	}

	if err != nil {
		return b.yield(a2a.NewStatusUpdateEvent(b.execCtx, a2a.TaskStateFailed,
			a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(err.Error()))), nil)
	}
	return b.yield(a2a.NewStatusUpdateEvent(b.execCtx, a2a.TaskStateCompleted, nil), nil)
}
