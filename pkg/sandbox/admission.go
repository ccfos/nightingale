package sandbox

import (
	"context"
	"sync"
)

// admission is the host-level guard that keeps N concurrent skill runs from
// exhausting the host (§14.2). Concurrency is a bounded slot channel (callers
// queue until a slot frees or ctx expires); the total-memory budget is a hard
// reject (fast-fail rather than queue) so a burst of large requests can't pile
// up reservations behind the slot queue.
type admission struct {
	slots chan struct{}

	memMu       sync.Mutex
	memBudgetMB int64
	memUsedMB   int64
}

func newAdmission(cfg AdmissionConfig) *admission {
	n := cfg.MaxConcurrent
	if n <= 0 {
		n = defaultMaxConcurrent
	}
	return &admission{
		slots:       make(chan struct{}, n),
		memBudgetMB: cfg.MaxTotalMemoryMB,
	}
}

// acquire reserves memMB of the budget and one concurrency slot. It returns a
// release func that must be called exactly once when the run finishes. The
// returned error is errAdmissionMemory (over budget) or ctx.Err() (cancelled /
// timed out while queued for a slot).
func (a *admission) acquire(ctx context.Context, memMB int64) (func(), error) {
	if memMB <= 0 {
		memMB = defaultMemoryMB
	}

	// Memory budget: reject immediately if this run would overflow it.
	a.memMu.Lock()
	if a.memBudgetMB > 0 && a.memUsedMB+memMB > a.memBudgetMB {
		used, budget := a.memUsedMB, a.memBudgetMB
		a.memMu.Unlock()
		return nil, &admissionError{reason: "memory_budget", detail: memBudgetMsg(memMB, used, budget)}
	}
	a.memUsedMB += memMB
	a.memMu.Unlock()

	// Concurrency slot: queue until free or context done.
	select {
	case a.slots <- struct{}{}:
	case <-ctx.Done():
		a.releaseMem(memMB)
		return nil, ctx.Err()
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			<-a.slots
			a.releaseMem(memMB)
		})
	}, nil
}

func (a *admission) releaseMem(memMB int64) {
	a.memMu.Lock()
	a.memUsedMB -= memMB
	if a.memUsedMB < 0 {
		a.memUsedMB = 0
	}
	a.memMu.Unlock()
}
