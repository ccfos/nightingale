package a2a

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/models"
)

// recordingYield captures every (event, err) pair the bridge yields so tests
// can assert on event types and structured part contents.
type recordingYield struct {
	events []a2a.Event
	errs   []error
}

func (r *recordingYield) yield(ev a2a.Event, err error) bool {
	r.events = append(r.events, ev)
	r.errs = append(r.errs, err)
	return true
}

func newTestBridge() (*streamBridge, *recordingYield) {
	rec := &recordingYield{}
	ec := &a2asrv.ExecutorContext{ContextID: "ctx-1", TaskID: "task-1"}
	return newBridge(ec, rec.yield), rec
}

// collectBridge constructs a streamBridge whose yield channel records every
// emitted event into events, and returns the slice so tests can inspect what
// the bridge produced. Unlike recordingYield it fails the test on yield error.
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

// firstDataPart pulls the first Data-typed Part out of a
// TaskArtifactUpdateEvent. Returns nil for non-artifact events or artifacts
// without a Data part.
func firstDataPart(ev a2a.Event) *a2a.Part {
	upd, ok := ev.(*a2a.TaskArtifactUpdateEvent)
	if !ok {
		return nil
	}
	for _, p := range upd.Artifact.Parts {
		if _, ok := p.Content.(a2a.Data); ok {
			return p
		}
	}
	return nil
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

// ----- P2: forwardArtifact -----

func TestForwardArtifact_emitsDataPartWithVendorMIME(t *testing.T) {
	br, rec := newTestBridge()

	envelope, _ := json.Marshal(map[string]any{
		"kind":    "alert_rule",
		"mime":    "application/vnd.n9e.alert-rule+json",
		"content": json.RawMessage(`{"id":42,"name":"prod.cpu.high"}`),
	})

	if !br.Forward(aiagent.StreamMessage{P: "artifact", V: string(envelope)}) {
		t.Fatal("Forward returned false on a clean artifact")
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	part := firstDataPart(rec.events[0])
	if part == nil {
		t.Fatalf("expected a Data part, got %#v", rec.events[0])
	}
	if part.MediaType != "application/vnd.n9e.alert-rule+json" {
		t.Errorf("MediaType = %q, want application/vnd.n9e.alert-rule+json", part.MediaType)
	}
	if got := part.Meta()[kindMetadataKey]; got != "alert_rule" {
		t.Errorf("kind metadata = %v, want alert_rule", got)
	}
	// The Data should round-trip back to the structured object, not stay a string.
	d := part.Data()
	m, ok := d.(map[string]any)
	if !ok {
		t.Fatalf("Data is not map[string]any: %T (%v)", d, d)
	}
	if m["name"] != "prod.cpu.high" {
		t.Errorf("decoded name = %v, want prod.cpu.high", m["name"])
	}
}

func TestForwardArtifact_malformedEnvelopeIsDropped(t *testing.T) {
	br, rec := newTestBridge()

	// Not JSON at all — should be silently ignored, return true so the loop
	// keeps going.
	if !br.Forward(aiagent.StreamMessage{P: "artifact", V: "not json"}) {
		t.Fatal("Forward returned false on malformed envelope (should drop silently)")
	}
	if len(rec.events) != 0 {
		t.Errorf("expected zero events on malformed envelope, got %d", len(rec.events))
	}
}

func TestForwardArtifact_recordsKindForFinalizeDedup(t *testing.T) {
	br, _ := newTestBridge()

	envelope, _ := json.Marshal(artifactEnvelope{
		Kind:    "alert_rule",
		Mime:    "application/vnd.n9e.alert-rule+json",
		Content: json.RawMessage(`{"id":1}`),
	})
	br.Forward(aiagent.StreamMessage{P: "artifact", V: string(envelope)})

	if got := br.emittedKinds[kindAlertRule]; got != 1 {
		t.Fatalf("emittedKinds[alert_rule] = %d, want 1 (must be tracked for Finalize dedup)", got)
	}
}

// ----- P1: tool_result phase -----

func TestForward_toolResultYieldsWorkingStatusUpdate(t *testing.T) {
	br, rec := newTestBridge()

	if !br.Forward(aiagent.StreamMessage{P: "tool_result", V: "Created alert rule prod.cpu"}) {
		t.Fatal("Forward returned false")
	}
	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(rec.events))
	}
	upd, ok := rec.events[0].(*a2a.TaskStatusUpdateEvent)
	if !ok {
		t.Fatalf("expected TaskStatusUpdateEvent, got %T", rec.events[0])
	}
	if upd.Status.State != a2a.TaskStateWorking {
		t.Errorf("state = %s, want working", upd.Status.State)
	}
}

// ----- P3: Finalize safety net -----

func TestFinalize_replaysMissingFormSelectFromSnapshot(t *testing.T) {
	br, rec := newTestBridge()

	// Snapshot pretends the halted-flow path produced a form_select but never
	// pushed it through streamBus.
	snap := &models.AssistantMessage{
		Response: []models.AssistantMessageResponse{
			{ContentType: models.ContentTypeFormSelect, Content: `{"fields":[{"name":"busi_group"}]}`},
		},
	}
	if !br.Finalize(a2a.TaskStateCompleted, "", snap) {
		t.Fatal("Finalize returned false")
	}

	// Expect: 1 ArtifactEvent (replay) + 1 StatusUpdateEvent (terminal).
	if len(rec.events) != 2 {
		t.Fatalf("expected 2 events (artifact + terminal), got %d", len(rec.events))
	}
	part := firstDataPart(rec.events[0])
	if part == nil {
		t.Fatalf("first event has no Data part: %#v", rec.events[0])
	}
	if part.MediaType != "application/vnd.n9e.form-select+json" {
		t.Errorf("MediaType = %q, want vendor form-select MIME", part.MediaType)
	}
	if got := part.Meta()[kindMetadataKey]; got != "form_select" {
		t.Errorf("kind = %v, want form_select", got)
	}
	final, ok := rec.events[1].(*a2a.TaskStatusUpdateEvent)
	if !ok || final.Status.State != a2a.TaskStateCompleted {
		t.Errorf("trailing event = %#v, want StatusUpdate(Completed)", rec.events[1])
	}
}

func TestFinalize_skipsArtifactsAlreadyStreamed(t *testing.T) {
	br, rec := newTestBridge()

	// Realtime path emitted an alert_rule artifact (mid-stream).
	envelope, _ := json.Marshal(artifactEnvelope{
		Kind:    "alert_rule",
		Mime:    "application/vnd.n9e.alert-rule+json",
		Content: json.RawMessage(`{"id":7}`),
	})
	br.Forward(aiagent.StreamMessage{P: "artifact", V: string(envelope)})

	// Snapshot also has the alert_rule (as the post-loop persisted view).
	// Finalize must NOT re-emit it — emittedKinds counter already covers it.
	snap := &models.AssistantMessage{
		Response: []models.AssistantMessageResponse{
			{ContentType: models.ContentTypeAlertRule, Content: `{"id":7}`},
		},
	}
	br.Finalize(a2a.TaskStateCompleted, "", snap)

	// Expected event sequence: 1 realtime artifact, 1 terminal status. No
	// duplicate artifact from Finalize.
	if len(rec.events) != 2 {
		t.Fatalf("expected 2 events (realtime + terminal), got %d", len(rec.events))
	}
	if _, ok := rec.events[0].(*a2a.TaskArtifactUpdateEvent); !ok {
		t.Errorf("event[0] = %#v, want TaskArtifactUpdateEvent (realtime)", rec.events[0])
	}
	if upd, ok := rec.events[1].(*a2a.TaskStatusUpdateEvent); !ok || upd.Status.State != a2a.TaskStateCompleted {
		t.Errorf("event[1] = %#v, want StatusUpdate(Completed)", rec.events[1])
	}
}

func TestFinalize_replaysOnlyExtraOnPartialOverlap(t *testing.T) {
	br, rec := newTestBridge()

	// Realtime emitted ONE alert_rule.
	envelope, _ := json.Marshal(artifactEnvelope{
		Kind: "alert_rule", Mime: "application/vnd.n9e.alert-rule+json",
		Content: json.RawMessage(`{"id":1}`),
	})
	br.Forward(aiagent.StreamMessage{P: "artifact", V: string(envelope)})

	// Snapshot ended up with TWO alert_rules — the second one bypassed the
	// realtime path. Finalize should replay exactly that second one.
	snap := &models.AssistantMessage{
		Response: []models.AssistantMessageResponse{
			{ContentType: models.ContentTypeAlertRule, Content: `{"id":1}`},
			{ContentType: models.ContentTypeAlertRule, Content: `{"id":2}`},
		},
	}
	br.Finalize(a2a.TaskStateCompleted, "", snap)

	// Realtime: 1 artifact. Finalize replay: 1 artifact (the second). Terminal: 1.
	if len(rec.events) != 3 {
		t.Fatalf("expected 3 events (realtime + replay + terminal), got %d", len(rec.events))
	}
	replay := firstDataPart(rec.events[1])
	if replay == nil {
		t.Fatalf("replay event has no Data part: %#v", rec.events[1])
	}
	m, ok := replay.Data().(map[string]any)
	if !ok {
		t.Fatalf("replay Data not a map: %T", replay.Data())
	}
	if m["id"].(float64) != 2 {
		t.Errorf("replayed wrong rule (id=%v), want id=2", m["id"])
	}
}

func TestFinalize_nilSnapStillTerminates(t *testing.T) {
	br, rec := newTestBridge()

	br.Finalize(a2a.TaskStateCompleted, "", nil)

	if len(rec.events) != 1 {
		t.Fatalf("expected 1 event (terminal), got %d", len(rec.events))
	}
	if _, ok := rec.events[0].(*a2a.TaskStatusUpdateEvent); !ok {
		t.Errorf("got %#v, want StatusUpdate", rec.events[0])
	}
}

func TestFinalize_textyContentTypesAreNotReplayed(t *testing.T) {
	br, rec := newTestBridge()

	snap := &models.AssistantMessage{
		Response: []models.AssistantMessageResponse{
			{ContentType: models.ContentTypeMarkdown, Content: "regular markdown body"},
			{ContentType: models.ContentTypeReasoning, Content: "trace"},
		},
	}
	br.Finalize(a2a.TaskStateCompleted, "", snap)

	if len(rec.events) != 1 {
		t.Fatalf("expected only the terminal status event, got %d", len(rec.events))
	}
}

// ----- content / reason / step streaming -----

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
