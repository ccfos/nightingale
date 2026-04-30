package a2a

import (
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2asrv"
	a2astore "github.com/a2aproject/a2a-go/v2/a2asrv/taskstore"
)

// HandlerOptions tunes the A2A REST handler. The zero value is valid.
type HandlerOptions struct {
	// TaskStore persists tasks across the cluster so tasks/get and
	// tasks/resubscribe survive process restarts and LB-induced instance
	// switches. When nil, the SDK's in-memory default is used (per-process,
	// non-durable).
	TaskStore a2astore.Store
}

// NewHTTPHandler builds the REST/HTTP+JSON A2A handler that should be mounted
// at the path advertised in AgentCard.SupportedInterfaces. Mount it under any
// gin group that has the n9e tokenAuth middleware applied; the executor reads
// the user from request.Context (see WithUser).
func NewHTTPHandler(backend AssistantBackend, opts HandlerOptions) http.Handler {
	options := []a2asrv.RequestHandlerOption{}
	if opts.TaskStore != nil {
		options = append(options, a2asrv.WithTaskStore(opts.TaskStore))
	}
	requestHandler := a2asrv.NewHandler(NewExecutor(backend), options...)
	return a2asrv.NewRESTHandler(requestHandler)
}
