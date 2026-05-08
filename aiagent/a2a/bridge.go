package a2a

import (
	"encoding/json"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// thoughtMetadataKey is recognised by Google ADK A2A clients to render content
// as the agent's chain-of-thought rather than a regular response. Other clients
// see it as plain text — no harm done.
const thoughtMetadataKey = "adk_thought"

// kindMetadataKey tags an artifact Part with the n9e artifact kind tag (e.g.
// "alert_rule"). Lets clients route to the right card renderer without
// re-parsing the MIME string.
const kindMetadataKey = "n9e_kind"

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

	// emittedKinds counts how many structured artifacts of each kind have
	// already been yielded via the realtime "artifact" phase. Finalize uses
	// this to decide which Response items in the final snapshot still need
	// to be replayed as a safety-net (mode 2): skip kinds we've already
	// emitted enough of, replay the remainder.
	emittedKinds map[artifactKind]int
}

func newBridge(ec *a2asrv.ExecutorContext, yield func(a2a.Event, error) bool) *streamBridge {
	return &streamBridge{
		execCtx:      ec,
		yield:        yield,
		emittedKinds: map[artifactKind]int{},
	}
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
	case "tool_result":
		// One-line summary of a finished tool call, e.g. "Created alert rule
		// prod.cpu.high". Different from "step" only in semantic intent (post
		// vs pre tool) — but kept distinct so future producers can attach
		// extra metadata without overloading "step".
		return b.yield(a2a.NewStatusUpdateEvent(b.execCtx, a2a.TaskStateWorking,
			a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(msg.V))), nil)
	case "artifact":
		return b.forwardArtifact(msg.V)
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

// artifactEnvelope is the on-wire shape of a P:"artifact" StreamMessage.V.
// Producers (router_ai_assistant.go) marshal one of these per structured
// tool result; the bridge unmarshals here and translates to an A2A
// ArtifactEvent carrying a Data part.
type artifactEnvelope struct {
	Kind    string          `json:"kind"`
	Mime    string          `json:"mime"`
	Content json.RawMessage `json:"content"`
}

// forwardArtifact translates a P:"artifact" StreamMessage into an A2A
// ArtifactEvent with a Data part. Each artifact gets a fresh artifact ID so
// clients render structured cards independently of the markdown answer
// stream. Malformed envelopes are dropped silently — the producer is
// authoritative for the wire schema and bad payloads shouldn't fail the
// whole task.
func (b *streamBridge) forwardArtifact(envelope string) bool {
	var env artifactEnvelope
	if err := json.Unmarshal([]byte(envelope), &env); err != nil || env.Kind == "" {
		return true
	}

	// Track the kind so Finalize's safety-net pass knows not to re-emit it.
	b.emittedKinds[artifactKind(env.Kind)]++

	return b.yield(b.buildArtifactEvent(env), nil)
}

// buildArtifactEvent constructs the ArtifactEvent for a parsed envelope. Used
// by both forwardArtifact (realtime) and Finalize (safety-net) so the wire
// shape is identical regardless of which path emitted it.
func (b *streamBridge) buildArtifactEvent(env artifactEnvelope) a2a.Event {
	// Best-effort decode the inner JSON so the Data part carries the typed
	// object, not a string. If decode fails (rare — content was already
	// JSON-marshaled by the tool), fall back to the raw string so the part
	// is at least transmittable.
	var content any
	if err := json.Unmarshal(env.Content, &content); err != nil || content == nil {
		content = string(env.Content)
	}
	part := a2a.NewDataPart(content)
	if env.Mime != "" {
		part.MediaType = env.Mime
	}
	part.SetMeta(kindMetadataKey, env.Kind)
	return a2a.NewArtifactEvent(b.execCtx, part)
}

// Finalize emits the terminal status update for the task, optionally
// attaching a human-readable message (used to surface ErrMsg on
// failed/canceled terminals). An empty msg keeps the status update
// payload-free.
//
// Before yielding the terminal status, Finalize sweeps the persisted
// AssistantMessage snapshot for structured Response items that did NOT come
// through the realtime stream — the "mode 2" safety net. Two cases this
// rescues:
//
//   - Halted-flow responses (form_select / hint produced by preflight hooks
//     via finishHaltedMessage) never invoke the agent loop and never push
//     anything onto streamBus. Without this sweep they're invisible to A2A.
//   - Future tools whose ToolResult-time emission was forgotten or lost in
//     transit. Costs nothing when the realtime path worked (dedup via
//     emittedKinds counters).
//
// snap may be nil — happens when the snapshot has TTL'd out, or for the
// terminal-state cancel path that never persisted. In that case we just
// emit the status update as before.
//
// We deliberately do NOT emit a separate LastChunk artifact-update event:
// the SDK's taskupdate.Manager rejects ArtifactUpdate events with empty
// Parts (a2a.ErrInvalidAgentResponse "artifact cannot be empty"), which
// would flip the task state to failed even though the conversation
// succeeded. The terminal status update alone is the canonical end-of-
// stream signal in this SDK; clients still know the artifact is final
// because no further deltas arrive after a terminal state.
func (b *streamBridge) Finalize(state a2a.TaskState, msg string, snap *models.AssistantMessage) bool {
	if snap != nil {
		// Mutate a copy of emittedKinds so the safety-net pass doesn't
		// confuse a future Forward call (in practice Finalize is the last
		// thing called, but keeping the original counters intact is cheap
		// defensive code).
		remaining := make(map[artifactKind]int, len(b.emittedKinds))
		for k, v := range b.emittedKinds {
			remaining[k] = v
		}
		for _, r := range snap.Response {
			kind := kindForContentType(r.ContentType)
			if kind == "" {
				continue // text-y type — already covered by P:content stream
			}
			if remaining[kind] > 0 {
				remaining[kind]--
				continue // already streamed mid-flight
			}
			ev := b.buildArtifactEvent(artifactEnvelope{
				Kind:    string(kind),
				Mime:    vendorMime(kind),
				Content: json.RawMessage(r.Content),
			})
			if !b.yield(ev, nil) {
				return false
			}
		}
	}

	var payload *a2a.Message
	if msg != "" {
		payload = a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(msg))
	}
	return b.yield(a2a.NewStatusUpdateEvent(b.execCtx, state, payload), nil)
}
