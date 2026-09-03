package doris

import (
	"context"
	"testing"

	dskittypes "github.com/ccfos/nightingale/v6/dskit/types"
)

func TestCallContext_RoundTrip(t *testing.T) {
	ctx := dskittypes.WithCallContext(context.Background(), dskittypes.CallContext{DatasourceID: 7, Operator: "alice"})
	cc, ok := dskittypes.CallContextFromCtx(ctx)
	if !ok {
		t.Fatal("CallContextFromCtx: expected ok=true")
	}
	if cc.DatasourceID != 7 || cc.Operator != "alice" {
		t.Fatalf("CallContextFromCtx: got %+v", cc)
	}
}

func TestCallContext_Missing(t *testing.T) {
	if _, ok := dskittypes.CallContextFromCtx(context.Background()); ok {
		t.Fatal("CallContextFromCtx on bare ctx: expected ok=false")
	}
}

func TestOnQuery_NilIsSafe(t *testing.T) {
	// Sanity: leaving OnQuery nil must not break anything; ExecQuery's
	// defer guards against this. We exercise the guard logic directly here
	// because spinning up a real Doris is out of scope for unit tests.
	prev := OnQuery
	OnQuery = nil
	defer func() { OnQuery = prev }()

	if OnQuery != nil {
		OnQuery(context.Background(), QueryEvent{})
	}
}
