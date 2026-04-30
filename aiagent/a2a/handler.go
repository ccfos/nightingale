package a2a

import (
	"net/http"

	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// NewHTTPHandler builds the REST/HTTP+JSON A2A handler that should be mounted
// at the path advertised in AgentCard.SupportedInterfaces. Mount it under any
// gin group that has the n9e tokenAuth middleware applied; the executor reads
// the user from request.Context (see WithUser).
func NewHTTPHandler(backend AssistantBackend) http.Handler {
	requestHandler := a2asrv.NewHandler(NewExecutor(backend))
	return a2asrv.NewRESTHandler(requestHandler)
}
