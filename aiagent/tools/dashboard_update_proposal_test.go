package tools

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/redis/go-redis/v9"
)

// bg is a tiny helper so the proposal-store calls read cleanly.
func bg() context.Context { return context.Background() }

// The in-memory fallback (rds == nil) and the Redis path must behave
// identically for the propose→confirm flow, so the store tests run against the
// fallback; a dedicated Redis test below covers the GETDEL path end-to-end.

func TestProposalStore_PutTakeSingleUse(t *testing.T) {
	s := &proposalStore{items: map[string]*dashboardProposal{}}
	p := &dashboardProposal{ID: "p1", BoardID: 7, CreatedAt: time.Now()}
	if err := s.put(bg(), nil, p); err != nil {
		t.Fatalf("put: %v", err)
	}

	got := s.take(bg(), nil, "p1")
	if got == nil || got.BoardID != 7 {
		t.Fatalf("take returned %#v, want the stored proposal", got)
	}
	// take consumes: a second take must miss (proposal_id is single-use, so a
	// confirm can't be replayed).
	if again := s.take(bg(), nil, "p1"); again != nil {
		t.Fatal("take must consume the proposal; second take should be nil")
	}
}

func TestProposalStore_TakeMissing(t *testing.T) {
	s := &proposalStore{items: map[string]*dashboardProposal{}}
	if got := s.take(bg(), nil, "nope"); got != nil {
		t.Fatalf("take of unknown id = %#v, want nil", got)
	}
}

func TestProposalStore_Expiry(t *testing.T) {
	s := &proposalStore{items: map[string]*dashboardProposal{}}
	// Stamp the proposal older than the TTL; take must treat it as gone.
	if err := s.put(bg(), nil, &dashboardProposal{ID: "old", CreatedAt: time.Now().Add(-proposalTTL - time.Minute)}); err != nil {
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
	s := &proposalStore{items: map[string]*dashboardProposal{}}

	p := &dashboardProposal{
		ID:           "dbprop_abc",
		ChatID:       "c1",
		SeqID:        3,
		BoardID:      9,
		BaselineHash: "deadbeef",
		NewConfigs:   `{"var":[]}`,
		Changes:      []string{"更新变量 \"ident\""},
		CreatedAt:    time.Now(),
	}
	if err := s.put(bg(), rds, p); err != nil {
		t.Fatalf("put to redis: %v", err)
	}

	got := s.take(bg(), rds, "dbprop_abc")
	if got == nil {
		t.Fatal("take from redis returned nil, want the stored proposal")
	}
	if got.BoardID != 9 || got.SeqID != 3 || got.ChatID != "c1" ||
		got.BaselineHash != "deadbeef" || got.NewConfigs != `{"var":[]}` ||
		len(got.Changes) != 1 {
		t.Fatalf("proposal did not round-trip through redis: %#v", got)
	}

	// Single-use: the GETDEL must have removed it.
	if again := s.take(bg(), rds, "dbprop_abc"); again != nil {
		t.Fatal("redis take must consume the proposal; second take should be nil")
	}

	// TTL must be set on the key (short, not persistent).
	if err := s.put(bg(), rds, &dashboardProposal{ID: "ttl", CreatedAt: time.Now()}); err != nil {
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
		if len(id) < len("dbprop_")+8 {
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
