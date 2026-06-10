package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/redis/go-redis/v9"
)

// bg is a tiny helper so the proposal-store calls read cleanly.
func bg() context.Context { return context.Background() }

// The in-memory fallback (rds == nil) and the Redis path must behave
// identically for the propose→confirm flow, so the store tests run against the
// fallback; a dedicated Redis test below covers the GETDEL path end-to-end.

func TestProposalStore_PutTakeSingleUse(t *testing.T) {
	s := &proposalStore{items: map[string]*updateProposal{}}
	p := &updateProposal{ID: "p1", TargetID: 7, CreatedAt: time.Now()}
	if err := s.put(bg(), nil, p); err != nil {
		t.Fatalf("put: %v", err)
	}

	got := s.take(bg(), nil, "p1")
	if got == nil || got.TargetID != 7 {
		t.Fatalf("take returned %#v, want the stored proposal", got)
	}
	// take consumes: a second take must miss (proposal_id is single-use, so a
	// confirm can't be replayed).
	if again := s.take(bg(), nil, "p1"); again != nil {
		t.Fatal("take must consume the proposal; second take should be nil")
	}
}

func TestProposalStore_TakeMissing(t *testing.T) {
	s := &proposalStore{items: map[string]*updateProposal{}}
	if got := s.take(bg(), nil, "nope"); got != nil {
		t.Fatalf("take of unknown id = %#v, want nil", got)
	}
}

func TestProposalStore_Expiry(t *testing.T) {
	s := &proposalStore{items: map[string]*updateProposal{}}
	// Stamp the proposal older than the TTL; take must treat it as gone.
	if err := s.put(bg(), nil, &updateProposal{ID: "old", CreatedAt: time.Now().Add(-proposalTTL - time.Minute)}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if got := s.take(bg(), nil, "old"); got != nil {
		t.Fatal("expired proposal must not be returned")
	}
}

// TestProposalStore_Redis exercises the Redis-backed path against miniredis:
// a proposal survives put→take, round-trips its fields, is single-use (atomic
// GETDEL), and honors the Redis key TTL.
func TestProposalStore_Redis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	var rds storage.Redis = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	s := &proposalStore{items: map[string]*updateProposal{}}

	p := &updateProposal{
		ID:           "uprop_abc",
		ChatID:       "c1",
		SeqID:        3,
		TargetID:     9,
		BaselineHash: "deadbeef",
		Payload:      `{"var":[]}`,
		Changes:      []string{"更新变量 \"ident\""},
		CreatedAt:    time.Now(),
	}
	if err := s.put(bg(), rds, p); err != nil {
		t.Fatalf("put to redis: %v", err)
	}

	got := s.take(bg(), rds, "uprop_abc")
	if got == nil {
		t.Fatal("take from redis returned nil, want the stored proposal")
	}
	if got.TargetID != 9 || got.SeqID != 3 || got.ChatID != "c1" ||
		got.BaselineHash != "deadbeef" || got.Payload != `{"var":[]}` ||
		len(got.Changes) != 1 {
		t.Fatalf("proposal did not round-trip through redis: %#v", got)
	}

	// Single-use: the GETDEL must have removed it.
	if again := s.take(bg(), rds, "uprop_abc"); again != nil {
		t.Fatal("redis take must consume the proposal; second take should be nil")
	}

	// TTL must be set on the key (short, not persistent).
	if err := s.put(bg(), rds, &updateProposal{ID: "ttl", CreatedAt: time.Now()}); err != nil {
		t.Fatalf("put: %v", err)
	}
	ttl := mr.TTL(proposalKey("ttl"))
	if ttl <= 0 || ttl > proposalTTL {
		t.Fatalf("redis key TTL = %v, want (0, %v]", ttl, proposalTTL)
	}
	// Fast-forward past the TTL: the proposal must be gone.
	mr.FastForward(proposalTTL + time.Minute)
	if got := s.take(bg(), rds, "ttl"); got != nil {
		t.Fatal("expired redis proposal must not be returned")
	}
}

func TestNewProposalID_UnguessableAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := newProposalID()
		if err != nil {
			t.Fatalf("newProposalID: %v", err)
		}
		if len(id) < len("uprop_")+8 {
			t.Fatalf("proposal id too short to be unguessable: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate proposal id generated: %q", id)
		}
		seen[id] = true
	}
}

func TestHashConfigs_DetectsChange(t *testing.T) {
	a := hashConfigs(`{"x":1}`)
	if a != hashConfigs(`{"x":1}`) {
		t.Fatal("hashConfigs must be stable for identical input")
	}
	if a == hashConfigs(`{"x":2}`) {
		t.Fatal("hashConfigs must differ when the payload changes (conflict guard)")
	}
}

// gateDeps returns minimal ToolDeps for the gate tests: only deps.Redis is
// touched by the store, and nil falls back to the in-memory map.
func gateDeps() *aiagent.ToolDeps { return &aiagent.ToolDeps{} }

// stashProposal puts a proposal directly into the (fresh) global store and
// returns its id. Tests reset the store first so they don't leak into each other.
func stashProposal(t *testing.T, p *updateProposal) string {
	t.Helper()
	if p.ID == "" {
		p.ID = "uprop_test_" + t.Name()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if err := updateProposals.put(bg(), nil, p); err != nil {
		t.Fatalf("put: %v", err)
	}
	return p.ID
}

func resetProposalStore() {
	updateProposals.mu.Lock()
	updateProposals.items = map[string]*updateProposal{}
	updateProposals.mu.Unlock()
}

func TestConfirmUpdateGate(t *testing.T) {
	const (
		kind     = "alert_mute"
		tool     = "update_alert_mute"
		target   = int64(7)
		baseline = "base-hash"
	)
	mk := func() *updateProposal {
		return &updateProposal{Kind: kind, TargetID: target, BaselineHash: baseline, ChatID: "c1", SeqID: 3}
	}
	confirmParams := map[string]string{"chat_id": "c1", "seq_id": "4"}

	t.Run("happy path consumes the proposal", func(t *testing.T) {
		resetProposalStore()
		id := stashProposal(t, mk())
		p, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, kind, target, id, true, baseline)
		if err != nil || p == nil {
			t.Fatalf("expected success, got p=%v err=%v", p, err)
		}
		// Single-use: the same confirm must not pass twice.
		if _, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, kind, target, id, true, baseline); err == nil {
			t.Fatal("second confirm with the same proposal_id must fail")
		}
	})

	t.Run("same-turn self-confirm is refused and does not burn the proposal", func(t *testing.T) {
		resetProposalStore()
		id := stashProposal(t, mk())
		sameTurn := map[string]string{"chat_id": "c1", "seq_id": "3"}
		if _, err := confirmUpdateGate(bg(), gateDeps(), sameTurn, tool, kind, target, id, true, baseline); err == nil {
			t.Fatal("confirm in the proposing turn must be refused")
		}
		// The genuine confirm one turn later must still find the proposal.
		if _, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, kind, target, id, true, baseline); err != nil {
			t.Fatalf("later-turn confirm after a rejected one should succeed, got %v", err)
		}
	})

	t.Run("cross-chat confirm is refused", func(t *testing.T) {
		resetProposalStore()
		id := stashProposal(t, mk())
		other := map[string]string{"chat_id": "c2", "seq_id": "9"}
		if _, err := confirmUpdateGate(bg(), gateDeps(), other, tool, kind, target, id, true, baseline); err == nil {
			t.Fatal("confirm from another chat must be refused")
		}
	})

	t.Run("kind or target mismatch is refused", func(t *testing.T) {
		resetProposalStore()
		id := stashProposal(t, mk())
		if _, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, "notify_rule", target, id, true, baseline); err == nil {
			t.Fatal("kind mismatch must be refused")
		}
		if _, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, kind, target+1, id, true, baseline); err == nil {
			t.Fatal("target mismatch must be refused")
		}
	})

	t.Run("stale baseline is refused without burning the proposal", func(t *testing.T) {
		resetProposalStore()
		id := stashProposal(t, mk())
		if _, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, kind, target, id, true, "drifted"); err == nil {
			t.Fatal("stale baseline must be refused")
		}
		if _, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, kind, target, id, true, baseline); err != nil {
			t.Fatalf("confirm with the original baseline should still succeed, got %v", err)
		}
	})

	t.Run("confirmed without proposal_id and vice versa are refused", func(t *testing.T) {
		resetProposalStore()
		id := stashProposal(t, mk())
		if _, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, kind, target, "", true, baseline); err == nil {
			t.Fatal("confirmed=true without proposal_id must be refused")
		}
		if _, err := confirmUpdateGate(bg(), gateDeps(), confirmParams, tool, kind, target, id, false, baseline); err == nil {
			t.Fatal("proposal_id without confirmed=true must be refused")
		}
	})
}

func TestProposeUpdate_InterruptShape(t *testing.T) {
	resetProposalStore()
	res, err := proposeUpdate(bg(), gateDeps(), map[string]string{"chat_id": "c9", "seq_id": "2"}, &updateProposal{
		Kind:         "alert_subscribe",
		TargetID:     11,
		BaselineHash: "h",
		Changes:      []string{"`disabled` → 1"},
	}, "确认文案", map[string]interface{}{"id": int64(11), "config": `{"disabled":1}`})
	if res != "" {
		t.Fatalf("propose must not return a result, got %q", res)
	}
	intr, ok := err.(*aiagent.ToolInterrupt)
	if !ok {
		t.Fatalf("propose must return a *aiagent.ToolInterrupt, got %T: %v", err, err)
	}
	if intr.Kind != aiagent.InterruptKindApproval || intr.Prompt != "确认文案" {
		t.Fatalf("unexpected interrupt: %+v", intr)
	}
	var replay map[string]interface{}
	if uerr := json.Unmarshal([]byte(intr.ResumeArgs), &replay); uerr != nil {
		t.Fatalf("resume args not valid JSON: %v", uerr)
	}
	pid, _ := replay["proposal_id"].(string)
	if pid == "" || replay["confirmed"] != true || replay["config"] != `{"disabled":1}` {
		t.Fatalf("resume args must carry proposal_id/confirmed/original args: %v", replay)
	}
	// The stashed proposal must be confirmable in a later turn of the same chat.
	if _, gerr := confirmUpdateGate(bg(), gateDeps(), map[string]string{"chat_id": "c9", "seq_id": "3"}, "update_alert_subscribe", "alert_subscribe", 11, pid, true, "h"); gerr != nil {
		t.Fatalf("replayed confirm should pass the gate, got %v", gerr)
	}
}
