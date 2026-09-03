package doris

import (
	"context"
	"time"
)

// QueryEvent describes one ExecQuery invocation. It is emitted to OnQuery
// after the query finishes, regardless of success or failure.
//
// The struct stays neutral on purpose: it carries raw execution facts only.
// Higher layers (audit, metrics, tracing, …) decide what to do with the data
// based on CallContext attached to ctx.
type QueryEvent struct {
	Database  string
	SQL       string
	StartedAt time.Time
	Duration  time.Duration
	RowCount  int
	Err       error
}

// OnQuery, when non-nil, is invoked after every ExecQuery finishes.
//
// It is a package-level variable rather than a slice of listeners because
// dskit/doris must stay free of orchestration logic; callers that need fan-out
// can register a single dispatcher function. Setting OnQuery is expected to
// happen exactly once during process init and is not safe for concurrent
// reassignment.
var OnQuery func(ctx context.Context, ev QueryEvent)
