package doris

import "context"

// CallContext carries the originating call site information for a Doris query.
// It is propagated through context so deep observation hooks (audit, metrics,
// tracing) can correlate raw execution with the user-facing request.
//
// The fields are deliberately neutral so this helper is reusable for any
// observability concern, not just auditing.
type CallContext struct {
	DatasourceID int64  // datasource id resolved at the entry handler
	Operator     string // best-effort caller identity, usually username
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
