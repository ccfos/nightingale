package types

import "context"

// CallContext carries the originating call site information for a query.
// Datasource hooks (audit, metrics, tracing) use it to attribute raw
// execution back to the user-facing request or rule scheduler.
//
// All fields are datasource-neutral; this type intentionally lives in
// dskit/types (not under any ds-specific package) so generic dispatchers
// can populate it without depending on a particular datasource impl.
type CallContext struct {
	DatasourceID int64  // datasource id resolved at the entry handler
	Operator     string // username for human queries; "alert_rule" / "recording_rule" for rule schedulers
	RuleID       int64  // rule id when Operator is a rule scheduler; 0 otherwise
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
