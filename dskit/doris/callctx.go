package doris

import "context"

// Caller is the high-level call site classification recorded in a CallContext.
//
// It is consumed by observation hooks (metrics, audit, tracing) to distinguish
// query traffic produced by interactive user requests from background workers
// such as alert/recording rule schedulers. The value is intentionally a plain
// string so downstream observers can introduce their own categories without
// changing this package; the constants below cover the common cases.
type Caller string

const (
	// CallerUser denotes interactive, user-facing query traffic
	// (dashboards, log search, ad-hoc SQL...).
	CallerUser Caller = "user"
	// CallerAlert denotes alert rule evaluation queries.
	CallerAlert Caller = "alert"
	// CallerRecord denotes recording rule evaluation queries.
	CallerRecord Caller = "record"
)

// CallContext carries the originating call site information for a Doris query.
// It is propagated through context so deep observation hooks (audit, metrics,
// tracing) can correlate raw execution with the user-facing request.
//
// The fields are deliberately neutral so this helper is reusable for any
// observability concern, not just auditing.
type CallContext struct {
	DatasourceID int64  // datasource id resolved at the entry handler
	Operator     string // best-effort caller identity, usually username
	Caller       Caller // high-level classification of the call site; empty when unknown
}

type callCtxKey struct{}

// WithCallContext returns a child context carrying cc. Passing zero-value cc
// is allowed; it simply records "no call context".
func WithCallContext(ctx context.Context, cc CallContext) context.Context {
	return context.WithValue(ctx, callCtxKey{}, cc)
}

// CallContextFromCtx retrieves the CallContext previously stored by
// WithCallContext. The second return value reports whether a value was found.
func CallContextFromCtx(ctx context.Context) (CallContext, bool) {
	cc, ok := ctx.Value(callCtxKey{}).(CallContext)
	return cc, ok
}
