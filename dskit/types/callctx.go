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

	// EnforceReadOnly demands a strict read-only check on SQL datasources
	// before execution. Entry handlers set it for callers that are not
	// authenticated users - currently anonymous dashboard share tokens.
	// The per-dialect keyword blacklists are not a security boundary
	// (they split on spaces, so `DELETE\tFROM` slips through); when this
	// flag is set the query must additionally pass sqlbase.ValidateReadOnly.
	// Leaving it false keeps the existing behaviour byte for byte.
	EnforceReadOnly bool
}

// ReadOnlyEnforced reports whether ctx carries a call context demanding the
// strict read-only check. Absent call context means "not enforced", so rule
// schedulers and other internal callers are unaffected.
func ReadOnlyEnforced(ctx context.Context) bool {
	cc, ok := CallContextFromCtx(ctx)
	return ok && cc.EnforceReadOnly
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
