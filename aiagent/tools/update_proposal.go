package tools

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/redis/go-redis/v9"
	"github.com/toolkits/pkg/logger"
)

// ============================================================================
// Two-phase write confirmation for the update_* tools (dashboard, alert rule,
// alert mute, alert subscribe, notify rule).
//
// The "show the diff, wait for the user to confirm" rule used to live only in
// the prompt. Nothing on the server stopped the model — in the same turn, or
// when steered by an injected prompt — from calling an update tool and writing
// straight to the DB. This adds a server-side gate the prompt can't bypass:
//
//   1. The first (propose) call computes the change set, stashes it under a
//      freshly minted random proposal_id, and returns WITHOUT writing — the
//      turn ends with an approval interrupt whose confirmation copy is rendered
//      deterministically by the tool (never paraphrased by the model).
//   2. When the user confirms in a later turn, the runtime replays the tool
//      with the stashed ResumeArgs (proposal_id + confirmed=true) — zero LLM
//      involvement. The confirm leg is honored only if it arrives in a LATER
//      chat turn (so one model turn can't both propose and confirm) and the
//      target's current state still matches the baseline captured at propose
//      time (so a concurrent edit can't be silently clobbered).
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
const proposalKeyPrefix = "n9e:ai:update_proposal:"

func proposalKey(id string) string { return proposalKeyPrefix + id }

// updateProposal is a pending, not-yet-applied update_* change set.
type updateProposal struct {
	ID           string    `json:"id"`
	ChatID       string    `json:"chat_id"`           // conversation the proposal was made in ("" when unknown)
	SeqID        int64     `json:"seq_id"`            // chat turn the proposal was made in (0 when unknown)
	Kind         string    `json:"kind"`              // tool family: dashboard / alert_rule / alert_mute / alert_subscribe / notify_rule
	TargetID     int64     `json:"target_id"`         // the resource the proposal targets
	BaselineHash string    `json:"baseline_hash"`     // sha256 of the target's state at propose time (conflict guard)
	Payload      string    `json:"payload,omitempty"` // precomputed write payload (dashboard); rule tools re-derive from the replayed args instead
	Changes      []string  `json:"changes"`
	CreatedAt    time.Time `json:"created_at"`
}

// proposalStore persists pending update_* proposals. It prefers the shared
// Redis (passed per-call from ToolDeps, so the store doesn't own the client);
// the in-memory map is only used when no Redis client is available.
type proposalStore struct {
	mu    sync.Mutex
	items map[string]*updateProposal // process-local fallback (rds == nil)
}

var updateProposals = &proposalStore{items: map[string]*updateProposal{}}

// put stashes a proposal. With Redis it writes the JSON under a TTL'd key;
// without, it stores in the fallback map.
func (s *proposalStore) put(ctx context.Context, rds storage.Redis, p *updateProposal) error {
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
// expired. confirmUpdateGate validates against a peeked copy so a rejected
// confirm (e.g. the model confirming in the same turn, which the gate forbids)
// doesn't burn the proposal — the user's genuine confirm next turn must still
// find it. Only a successful apply consumes it (via take).
func (s *proposalStore) peek(ctx context.Context, rds storage.Redis, id string) *updateProposal {
	if rds != nil {
		body, err := rds.Get(ctx, proposalKey(id)).Bytes()
		if err != nil {
			if !errors.Is(err, redis.Nil) {
				logger.Warningf("update proposal: redis GET %q failed: %v", id, err)
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
func (s *proposalStore) take(ctx context.Context, rds storage.Redis, id string) *updateProposal {
	if rds != nil {
		body, err := rds.GetDel(ctx, proposalKey(id)).Bytes()
		if err != nil {
			// redis.Nil = missing/expired (the common "regenerate" path); any
			// other error is infrastructure trouble worth surfacing in logs.
			if !errors.Is(err, redis.Nil) {
				logger.Warningf("update proposal: redis GETDEL %q failed: %v", id, err)
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
func decodeProposal(id string, body []byte) *updateProposal {
	var p updateProposal
	if err := json.Unmarshal(body, &p); err != nil {
		logger.Warningf("update proposal: corrupt proposal %q in redis: %v", id, err)
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
	return "uprop_" + hex.EncodeToString(b), nil
}

// hashConfigs fingerprints a payload string for the propose→confirm conflict check.
func hashConfigs(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// updateBaselineHash fingerprints a rule's FE-shape struct for the
// propose→confirm conflict guard. json.Marshal field order is fixed by the
// struct definition, so identical state always hashes identically. Must be
// computed on the freshly fetched (DB2FE'd) rule BEFORE any merge/mutation.
func updateBaselineHash(v interface{}) string {
	b, _ := json.Marshal(v)
	return hashConfigs(string(b))
}

// proposeUpdate stashes a pending update and ends the turn with an approval
// interrupt: the runtime shows prompt to the user verbatim and, on their
// explicit confirmation in a later turn, replays the tool with resumeArgs +
// {proposal_id, confirmed:true} — zero LLM involvement in the confirm leg, so
// the model never needs to remember or restate the proposal_id. The caller
// fills p.Kind/TargetID/BaselineHash/Changes (and Payload if it precomputes the
// write); ID/ChatID/SeqID/CreatedAt are stamped here.
func proposeUpdate(ctx context.Context, deps *aiagent.ToolDeps, params map[string]string, p *updateProposal, prompt string, resumeArgs map[string]interface{}) (string, error) {
	pid, err := newProposalID()
	if err != nil {
		return "", fmt.Errorf("failed to generate proposal id: %v", err)
	}
	p.ID = pid
	p.ChatID = params["chat_id"]
	p.SeqID, _ = strconv.ParseInt(params["seq_id"], 10, 64)
	p.CreatedAt = time.Now()
	if err := updateProposals.put(ctx, deps.Redis, p); err != nil {
		return "", fmt.Errorf("failed to stash %s proposal: %v", p.Kind, err)
	}

	replay := make(map[string]interface{}, len(resumeArgs)+2)
	for k, v := range resumeArgs {
		replay[k] = v
	}
	replay["proposal_id"] = pid
	replay["confirmed"] = true
	raw, _ := json.Marshal(replay)
	return "", &aiagent.ToolInterrupt{
		Kind:       aiagent.InterruptKindApproval,
		Prompt:     prompt,
		ResumeArgs: string(raw),
	}
}

// confirmUpdateGate enforces the guarantees the prompt alone couldn't, shared
// by every update_* tool: the proposal must exist (and be single-use), it must
// be of the same kind and target this resource, the confirmation must land in a
// LATER chat turn than the proposal (so a single model turn can't propose and
// confirm itself), and the target's state must be unchanged since the proposal
// baseline (so a concurrent edit isn't silently overwritten). On success the
// proposal is consumed (atomic single-use) and returned for the caller to apply.
func confirmUpdateGate(ctx context.Context, deps *aiagent.ToolDeps, params map[string]string, toolName, kind string, targetID int64, proposalID string, confirmed bool, currentBaselineHash string) (*updateProposal, error) {
	if !confirmed {
		return nil, fmt.Errorf("proposal_id was provided but confirmed is not true: pass confirmed=true to apply the proposal")
	}
	if strings.TrimSpace(proposalID) == "" {
		return nil, fmt.Errorf("confirmed=true requires the proposal_id returned by the initial call; first call %s without confirmed to generate a proposal, show the changes, then confirm", toolName)
	}

	// Validate against a PEEKED copy (no delete yet): a confirm rejected by the
	// gate below must NOT burn the proposal, or the user's genuine confirm next
	// turn would fail with "not found". The proposal is consumed only once every
	// check passes and we're about to write (the take() at the end).
	p := updateProposals.peek(ctx, deps.Redis, proposalID)
	if p == nil {
		return nil, fmt.Errorf("proposal %q not found or expired; call %s again without confirmed to regenerate a fresh proposal against the current config, show the changes, and confirm again", proposalID, toolName)
	}
	if p.Kind != kind || p.TargetID != targetID {
		return nil, fmt.Errorf("proposal %q is for %s id=%d, not %s id=%d", proposalID, p.Kind, p.TargetID, kind, targetID)
	}

	// Turn-identity gate — FAIL CLOSED. A destructive edit is applied only when
	// the confirmation provably arrives in a LATER chat turn than the proposal
	// (proof that a real user message landed in between). The assistant router
	// always injects a non-empty chat_id + numeric seq_id (chat_id is required
	// upstream). A caller that supplies neither — a headless workflow or CLI
	// agent — can't prove a human confirmed, so we REFUSE rather than let one
	// model turn both propose and confirm: the model already sees the
	// proposal_id in the propose response, so it is no barrier on its own.
	// Rejection here is recoverable — the proposal was only peeked, not consumed.
	curSeq, seqErr := strconv.ParseInt(strings.TrimSpace(params["seq_id"]), 10, 64)
	switch {
	case params["chat_id"] == "" || params["chat_id"] != p.ChatID:
		return nil, fmt.Errorf("this update can only be confirmed inside the same chat conversation that proposed it; regenerate the proposal here, show the changes, and confirm")
	case seqErr != nil || curSeq <= p.SeqID:
		return nil, fmt.Errorf("an update must be confirmed by the user in a later turn than the proposal; show the changes, wait for the user to confirm, then call %s again with the proposal_id", toolName)
	}

	// Conflict guard: refuse if the target changed since the proposal baseline.
	if currentBaselineHash != p.BaselineHash {
		return nil, fmt.Errorf("%s id=%d has changed since this proposal was generated, so it is stale; call %s again without confirmed to regenerate against the current config, show the new changes, and confirm again", kind, targetID, toolName)
	}

	// All checks passed — consume now (atomic single-use). If another confirm
	// won the race, or the proposal expired between the peek and here, take()
	// returns nil and we don't write twice.
	if updateProposals.take(ctx, deps.Redis, proposalID) == nil {
		return nil, fmt.Errorf("proposal %q is no longer available (already confirmed or expired); call %s again without confirmed to regenerate", proposalID, toolName)
	}
	return p, nil
}

// renderUpdateProposalPrompt 把提案改动渲染成给用户的确认文案（markdown）。
// 由工具确定性生成，不依赖模型转述，保证用户看到的就是将要写入的全部改动。
func renderUpdateProposalPrompt(subject string, changes []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("即将修改%s，改动如下：\n", subject))
	for _, c := range changes {
		sb.WriteString("\n- ")
		sb.WriteString(c)
	}
	sb.WriteString("\n\n以上改动尚未写入。回复「确认」立即生效，回复「取消」放弃本次修改，也可以直接提出新的调整要求。")
	return sb.String()
}

// describePatchChanges renders "`key` → value" lines from an incremental-patch
// config for the confirmation copy, so the user sees the exact values about to
// be written, not just which fields move. Long values are truncated — the copy
// is a confirmation, not a full dump.
func describePatchChanges(patch map[string]json.RawMessage, keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		v := strings.TrimSpace(string(patch[k]))
		if r := []rune(v); len(r) > 120 {
			v = string(r[:120]) + "…"
		}
		out = append(out, fmt.Sprintf("`%s` → %s", k, v))
	}
	return out
}
