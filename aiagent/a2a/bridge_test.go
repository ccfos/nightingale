package a2a

import (
	"strconv"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
)

// collectBridge constructs a streamBridge whose yield channel records every
// emitted event into events, and returns the slice so tests can inspect what
// the bridge produced.
func collectBridge(t *testing.T) (*streamBridge, *[]a2a.Event) {
	t.Helper()
	var events []a2a.Event
	ec := &a2asrv.ExecutorContext{TaskID: "task-1", ContextID: "ctx-1"}
	b := newBridge(ec, func(ev a2a.Event, err error) bool {
		if err != nil {
			t.Fatalf("yield got error: %v", err)
		}
		events = append(events, ev)
		return true
	})
	return b, &events
}

// concatArtifactText reassembles the text content emitted to a single
// artifact ID. Useful for asserting on the full body without caring about
// which call was the create-vs-update event.
func concatArtifactText(events []a2a.Event, id a2a.ArtifactID) string {
	var sb strings.Builder
	for _, ev := range events {
		up, ok := ev.(*a2a.TaskArtifactUpdateEvent)
		if !ok || up.Artifact == nil || up.Artifact.ID != id {
			continue
		}
		for _, p := range up.Artifact.Parts {
			sb.WriteString(p.Text())
		}
	}
	return sb.String()
}

func TestForwardContentNotMarkedAsThought(t *testing.T) {
	b, events := collectBridge(t)

	if !b.Forward(aiagent.StreamMessage{P: "content", V: "hello "}) {
		t.Fatal("Forward returned false unexpectedly")
	}
	if !b.Forward(aiagent.StreamMessage{P: "content", V: "world"}) {
		t.Fatal("Forward returned false unexpectedly")
	}

	if len(*events) != 2 {
		t.Fatalf("expected 2 artifact events, got %d", len(*events))
	}
	for i, ev := range *events {
		up, ok := ev.(*a2a.TaskArtifactUpdateEvent)
		if !ok {
			t.Fatalf("event[%d] is not an artifact event: %T", i, ev)
		}
		for j, p := range up.Artifact.Parts {
			if _, ok := p.Metadata[thoughtMetadataKey]; ok {
				t.Fatalf("event[%d].part[%d]: content must NOT carry %q metadata (got %v)",
					i, j, thoughtMetadataKey, p.Metadata)
			}
		}
	}
}

func TestForwardReasonMarkedAsThought(t *testing.T) {
	b, events := collectBridge(t)

	if !b.Forward(aiagent.StreamMessage{P: "reason", V: "Thought: I need to check X\n"}) {
		t.Fatal("Forward returned false unexpectedly")
	}

	if len(*events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(*events))
	}
	up := (*events)[0].(*a2a.TaskArtifactUpdateEvent)
	if got, _ := up.Artifact.Parts[0].Metadata[thoughtMetadataKey].(bool); !got {
		t.Fatalf("reason delta must carry %q=true, got %v", thoughtMetadataKey, up.Artifact.Parts[0].Metadata)
	}
}

// TestReasonDeltasAppendToSameArtifact verifies that successive reason deltas
// land on the same artifact ID — that's what gives clients an incrementally-
// growing thought block instead of one artifact per token.
func TestReasonDeltasAppendToSameArtifact(t *testing.T) {
	b, events := collectBridge(t)

	deltas := []string{"Thought:", " step", " one"}
	for _, d := range deltas {
		if !b.Forward(aiagent.StreamMessage{P: "reason", V: d}) {
			t.Fatal("Forward returned false unexpectedly")
		}
	}

	if len(*events) != len(deltas) {
		t.Fatalf("expected %d artifact events, got %d", len(deltas), len(*events))
	}
	firstID := (*events)[0].(*a2a.TaskArtifactUpdateEvent).Artifact.ID
	for i, ev := range *events {
		up := ev.(*a2a.TaskArtifactUpdateEvent)
		if up.Artifact.ID != firstID {
			t.Fatalf("event[%d]: artifact ID drifted (%q != %q)", i, up.Artifact.ID, firstID)
		}
	}
	got := concatArtifactText(*events, firstID)
	want := strings.Join(deltas, "")
	if got != want {
		t.Fatalf("concatenated body mismatch:\n  got:  %q\n  want: %q", got, want)
	}
}

// TestStepBoundaryStartsFreshReasoningArtifact covers the multi-iteration
// ReAct case: when the router emits a P:"step" frame (tool result boundary),
// the next reason delta should land on a NEW artifact ID so clients can
// delimit thoughts per iteration instead of rendering them as one blob.
func TestStepBoundaryStartsFreshReasoningArtifact(t *testing.T) {
	b, events := collectBridge(t)

	b.Forward(aiagent.StreamMessage{P: "reason", V: "Thought: one"})
	b.Forward(aiagent.StreamMessage{P: "step", V: "tool_result:query"})
	b.Forward(aiagent.StreamMessage{P: "reason", V: "Thought: two"})

	var artifactIDs []a2a.ArtifactID
	var statusEvents int
	for _, ev := range *events {
		switch e := ev.(type) {
		case *a2a.TaskArtifactUpdateEvent:
			artifactIDs = append(artifactIDs, e.Artifact.ID)
		case *a2a.TaskStatusUpdateEvent:
			statusEvents++
		}
	}
	if len(artifactIDs) != 2 {
		t.Fatalf("expected 2 artifact events (one per iteration), got %d", len(artifactIDs))
	}
	if statusEvents != 1 {
		t.Fatalf("expected 1 status event for the step boundary, got %d", statusEvents)
	}
	if artifactIDs[0] == artifactIDs[1] {
		t.Fatalf("iteration 2 reasoning must allocate a new artifact ID (both were %q)", artifactIDs[0])
	}
}

// A card frame (dashboard/alert_rule) must surface as an artifact carrying the
// card JSON plus the n9e content-type tag, so A2A callers can render the widget.
func TestForwardResponseEmitsCardArtifact(t *testing.T) {
	b, events := collectBridge(t)

	card := `{"id":889,"name":"MySQL 监控","group_name":"AIDev","panels_count":20}`
	// Shape matches aiagent.ResponseFrame as PublishResponse serializes it.
	frame := `{"content_type":"dashboard","content":` + strconv.Quote(card) + `}`

	if !b.Forward(aiagent.StreamMessage{P: aiagent.PhaseResponse, V: frame}) {
		t.Fatal("Forward returned false unexpectedly")
	}

	if len(*events) != 1 {
		t.Fatalf("expected 1 artifact event, got %d", len(*events))
	}
	up, ok := (*events)[0].(*a2a.TaskArtifactUpdateEvent)
	if !ok {
		t.Fatalf("event is not an artifact event: %T", (*events)[0])
	}
	if len(up.Artifact.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(up.Artifact.Parts))
	}
	part := up.Artifact.Parts[0]
	if part.Text() != card {
		t.Fatalf("part text = %q, want card JSON %q", part.Text(), card)
	}
	if got := part.Metadata[n9eContentTypeMetadataKey]; got != "dashboard" {
		t.Fatalf("part metadata[%s] = %v, want \"dashboard\"", n9eContentTypeMetadataKey, got)
	}
}

// Empty-content or malformed frames are dropped silently (Forward returns true)
// rather than aborting the stream.
func TestForwardResponseDropsEmptyAndBadFrames(t *testing.T) {
	b, events := collectBridge(t)

	if !b.Forward(aiagent.StreamMessage{P: aiagent.PhaseResponse, V: `{"content_type":"dashboard","content":""}`}) {
		t.Fatal("empty-content frame should be dropped, not abort the stream")
	}
	if !b.Forward(aiagent.StreamMessage{P: aiagent.PhaseResponse, V: `not json`}) {
		t.Fatal("malformed frame should be dropped, not abort the stream")
	}
	if len(*events) != 0 {
		t.Fatalf("dropped frames must emit no events, got %d", len(*events))
	}
}
