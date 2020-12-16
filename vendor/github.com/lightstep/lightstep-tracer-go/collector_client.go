package lightstep

import (
	"context"
	"io"
	"net/http"

	"github.com/lightstep/lightstep-tracer-common/golang/gogo/collectorpb"
)

var accessTokenHeader = http.CanonicalHeaderKey("Lightstep-Access-Token")

// Connection describes a closable connection. Exposed for testing.
type Connection interface {
	io.Closer
}

// ConnectorFactory is for testing purposes.
type ConnectorFactory func() (interface{}, Connection, error)

// collectorResponse encapsulates internal grpc/http responses.
type collectorResponse interface {
	GetErrors() []string
	Disable() bool
	DevMode() bool
}

// Collector encapsulates custom transport of protobuf messages
type Collector interface {
	Report(context.Context, *collectorpb.ReportRequest) (*collectorpb.ReportResponse, error)
}

type reportRequest struct {
	protoRequest *collectorpb.ReportRequest
	httpRequest  *http.Request
}

// collectorClient encapsulates internal grpc/http transports.
type collectorClient interface {
	Report(context.Context, reportRequest) (collectorResponse, error)
	Translate(*collectorpb.ReportRequest) (reportRequest, error)
	ConnectClient() (Connection, error)
	ShouldReconnect() bool
}

func newCollectorClient(opts Options) (collectorClient, error) {
	if opts.CustomCollector != nil {
		return newCustomCollector(opts), nil
	}
	if opts.UseHttp {
		return newHTTPCollectorClient(opts)
	}
	if opts.UseGRPC {
		return newGrpcCollectorClient(opts)
	}

	// No transport specified, defaulting to HTTP
	return newHTTPCollectorClient(opts)
}
