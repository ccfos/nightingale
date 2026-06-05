package models

import "testing"

// TestConversationRouteSurvivesEncodeDecode proves the persisted route state
// round-trips through the gzip+base64 row encoding, so a follow-up form
// submission really can inherit the previous turn's action.
func TestConversationRouteSurvivesEncodeDecode(t *testing.T) {
	msg := AssistantMessage{
		ChatID: "c1",
		SeqID:  3,
		Extra: AssistantMessageExtra{
			HistoryMessages: []byte(`{"schema_version":1,"messages":[]}`),
			Route:           &ConversationRoute{ActionKey: "creation", AwaitingForm: true},
		},
	}

	row, err := encodeMessage(msg)
	if err != nil {
		t.Fatalf("encodeMessage: %v", err)
	}
	back, err := decodeMessage(&row)
	if err != nil {
		t.Fatalf("decodeMessage: %v", err)
	}
	if back == nil || back.Extra.Route == nil {
		t.Fatalf("route lost in round-trip: %+v", back)
	}
	if back.Extra.Route.ActionKey != "creation" || !back.Extra.Route.AwaitingForm {
		t.Fatalf("route = %+v, want {creation awaiting_form}", back.Extra.Route)
	}
	if string(back.Extra.HistoryMessages) != `{"schema_version":1,"messages":[]}` {
		t.Fatalf("history blob corrupted: %s", back.Extra.HistoryMessages)
	}

	// A message without route stays nil (no spurious inheritance).
	msg2 := AssistantMessage{ChatID: "c1", SeqID: 4}
	row2, err := encodeMessage(msg2)
	if err != nil {
		t.Fatalf("encodeMessage(no route): %v", err)
	}
	back2, err := decodeMessage(&row2)
	if err != nil {
		t.Fatalf("decodeMessage(no route): %v", err)
	}
	if back2.Extra.Route != nil {
		t.Fatalf("expected nil route, got %+v", back2.Extra.Route)
	}
}
