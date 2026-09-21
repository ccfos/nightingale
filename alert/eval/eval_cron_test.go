package eval

import (
	"testing"

	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/models"
)

// An unschedulable cron pattern used to leave a zero cron.Entry behind, and
// getPromEvalInterval then called Next on its nil Schedule, crashing the engine.
func TestNewAlertRuleWorkerInvalidCron(t *testing.T) {
	for _, pattern := range []string{"*/5 * * * *", "not a cron"} {
		rule := &models.AlertRule{Id: 1, CronPattern: pattern}
		p := &process.Processor{}
		arw := NewAlertRuleWorker(rule, 1, p, nil, nil)

		if p.ScheduleEntry.Schedule == nil {
			t.Errorf("pattern %q: expected a fallback schedule", pattern)
		}
		if p.PromEvalInterval != 60 {
			t.Errorf("pattern %q: got interval %d, want 60", pattern, p.PromEvalInterval)
		}
		if len(arw.Scheduler.Entries()) != 1 {
			t.Errorf("pattern %q: got %d entries, want 1", pattern, len(arw.Scheduler.Entries()))
		}
	}
}

func TestGetPromEvalIntervalNilSchedule(t *testing.T) {
	if got := getPromEvalInterval(nil); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestGuardRuleRecovers(t *testing.T) {
	ran := false
	guardRule("alert_eval_1 build", func() { panic("boom") })
	guardRule("alert_eval_2 build", func() { ran = true })
	if !ran {
		t.Error("a panic in one rule must not stop the following ones")
	}
}

// syncAlertRules recovers from panics, so the lock must be released on every path,
// otherwise the next sync blocks on it forever.
func TestSyncExternalProcessorsReleasesLock(t *testing.T) {
	s := &Scheduler{ExternalProcessors: process.NewExternalProcessors()}
	s.ExternalProcessors.Processors["k"] = &process.Processor{}

	// Hash() on a zero Processor dereferences its nil rule and panics.
	s.syncExternalProcessors(map[string]*process.Processor{"k": {}, "k2": {}})

	if !s.ExternalProcessors.ExternalLock.TryLock() {
		t.Fatal("ExternalLock is still held after a panic")
	}
	s.ExternalProcessors.ExternalLock.Unlock()
}
