package tools

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/storage"

	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// Two-phase write confirmation for update_dashboard.
//
// The "show the diff, wait for the user to confirm" rule used to live only in
// the prompt. Nothing on the server stopped the model — in the same turn, or
// when steered by an injected prompt — from calling update_dashboard and
// writing straight to the DB. This adds a server-side gate the prompt can't
// bypass:
//
//   1. The first (propose) call computes the change set and the resulting
//      payload, stashes them under a freshly minted random proposal_id, and
//      returns WITHOUT writing.
//   2. A second (confirm) call must carry that proposal_id + confirmed=true.
//      It is honored only if it arrives in a LATER chat turn (so one model turn
//      can't both propose and confirm) and the dashboard's current payload still
//      matches the baseline captured at propose time (so a concurrent edit can't
//      be silently clobbered).
//
// Storage: proposals live in Redis (SET with a short TTL, consumed via atomic
// GETDEL) so a propose served by one center instance can be confirmed on
// another — every center shares the same Redis, so multi-instance deployments
// behind a round-robin load balancer work. Redis EX enforces the TTL; the
// atomic GETDEL makes a proposal_id single-use even under concurrent confirms.
// When no Redis client is wired (CLI / unit tests / single-instance dev), it
// falls back to a process-local TTL-bounded map.
// ============================================================================

// proposalTTL bounds how long a generated proposal stays confirmable. Kept short
// — a proposal only has to survive the user reading the diff and replying, not a
// whole session — so a never-confirmed proposal can't linger in Redis.
const proposalTTL = 30 * time.Minute

// proposalKeyPrefix namespaces proposal keys in the shared Redis.
const proposalKeyPrefix = "n9e:ai:dashboard_proposal:"

func proposalKey(id string) string { return proposalKeyPrefix + id }

// dashboardProposal is a pending, not-yet-applied update_dashboard change set.
type dashboardProposal struct {
	ID           string    `json:"id"`
	ChatID       string    `json:"chat_id"`       // conversation the proposal was made in ("" when unknown)
	SeqID        int64     `json:"seq_id"`        // chat turn the proposal was made in (0 when unknown)
	BoardID      int64     `json:"board_id"`      // dashboard the proposal targets
	BaselineHash string    `json:"baseline_hash"` // sha256 of board.Configs at propose time (conflict guard)
	NewConfigs   string    `json:"new_configs"`   // the payload to persist on confirm
	Changes      []string  `json:"changes"`
	CreatedAt    time.Time `json:"created_at"`
}

// proposalStore persists pending update_dashboard proposals. It prefers the
// shared Redis (passed per-call from ToolDeps, so the store doesn't own the
// client); the in-memory map is only used when no Redis client is available.
type proposalStore struct {
	mu    sync.Mutex
	items map[string]*dashboardProposal // process-local fallback (rds == nil)
}

var dashboardProposals = &proposalStore{items: map[string]*dashboardProposal{}}

// put stashes a proposal. With Redis it writes the JSON under a TTL'd key;
// without, it stores in the fallback map.
func (s *proposalStore) put(ctx context.Context, rds storage.Redis, p *dashboardProposal) error {
	if rds != nil {
		body, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return rds.Set(ctx, proposalKey(p.ID), body, proposalTTL).Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	s.items[p.ID] = p
	return nil
}

// peek returns the proposal for id WITHOUT removing it, or nil if missing/
// expired. confirmDashboardProposal validates against a peeked copy so a
// rejected confirm (e.g. the model confirming in the same turn, which the gate
// forbids) doesn't burn the proposal — the user's genuine confirm next turn
// must still find it. Only a successful apply consumes it (via take).
func (s *proposalStore) peek(ctx context.Context, rds storage.Redis, id string) *dashboardProposal {
	if rds != nil {
		body, err := rds.Get(ctx, proposalKey(id)).Bytes()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				logger.Warningf("update_dashboard: redis GET proposal %q failed: %v", id, err)
			}
			return nil
		}
		return decodeProposal(id, body)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	return s.items[id]
}

// take returns and removes the proposal for id, or nil if it's missing/expired.
// Consuming on read makes a proposal_id single-use: a confirm can't be replayed.
// With Redis the read+delete is one atomic GETDEL, so two concurrent confirms
// can't both succeed.
func (s *proposalStore) take(ctx context.Context, rds storage.Redis, id string) *dashboardProposal {
	if rds != nil {
		body, err := rds.GetDel(ctx, proposalKey(id)).Bytes()
		if err != nil {
			// redis.Nil = missing/expired (the common "regenerate" path); any
			// other error is infrastructure trouble worth surfacing in logs.
			if !errors.Is(err, redis.Nil) {
				logger.Warningf("update_dashboard: redis GETDEL proposal %q failed: %v", id, err)
			}
			return nil
		}
		return decodeProposal(id, body)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	p := s.items[id]
	if p == nil {
		return nil
	}
	delete(s.items, id)
	return p
}

// decodeProposal unmarshals a proposal read back from Redis, logging (and
// dropping) a corrupt payload rather than handing back a half-decoded struct.
func decodeProposal(id string, body []byte) *dashboardProposal {
	var p dashboardProposal
	if err := json.Unmarshal(body, &p); err != nil {
		logger.Warningf("update_dashboard: corrupt proposal %q in redis: %v", id, err)
		return nil
	}
	return &p
}

// gcLocked evicts expired entries from the in-memory fallback. The Redis path
// relies on key TTL instead and never touches this.
func (s *proposalStore) gcLocked() {
	now := time.Now()
	for id, p := range s.items {
		if now.Sub(p.CreatedAt) > proposalTTL {
			delete(s.items, id)
		}
	}
}

// newProposalID returns an unguessable proposal id so the model can't fabricate
// one to skip the propose phase.
func newProposalID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "dbprop_" + hex.EncodeToString(b), nil
}

// hashConfigs fingerprints a board payload for the propose→confirm conflict check.
func hashConfigs(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
