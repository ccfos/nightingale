package sandbox

import (
	"errors"
	"fmt"
)

// DisabledError is returned by Sandbox.Run when skill execution is disabled on
// this host (sandbox.enabled=false, or RequireIsolation=true with no isolation-
// capable engine). Reason is operator-actionable (what to enable / set).
type DisabledError struct {
	Reason string
}

func (e *DisabledError) Error() string {
	return "sandbox execution is disabled: " + e.Reason
}

// IsDisabled reports whether err is a DisabledError, so callers can render a
// clean "skill execution unavailable" message instead of a stack-y failure.
func IsDisabled(err error) bool {
	var d *DisabledError
	return errors.As(err, &d)
}

// admissionError is returned when a run is refused by host-level admission
// control (memory budget). Concurrency exhaustion surfaces as ctx.Err() instead
// (callers queue for a slot).
type admissionError struct {
	reason string
	detail string
}

func (e *admissionError) Error() string {
	return "sandbox admission denied (" + e.reason + "): " + e.detail
}

func memBudgetMsg(reqMB, usedMB, budgetMB int64) string {
	return fmt.Sprintf("request %dMB would exceed host budget (in use %dMB / %dMB)", reqMB, usedMB, budgetMB)
}
