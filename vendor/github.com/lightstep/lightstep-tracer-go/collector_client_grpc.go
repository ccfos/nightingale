package lightstep

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	// N.B.(jmacd): Do not use google.golang.org/glog in this package.
	"github.com/lightstep/lightstep-tracer-common/golang/gogo/collectorpb"
)

const (
	spansDropped     = "spans.dropped"
	logEncoderErrors = "log_encoder.errors"
)

var (
	intType = reflect.TypeOf(int64(0))
)

// grpcCollectorClient specifies how to send reports back to a LightStep
// collector via grpc.
type grpcCollectorClient struct {
	// accessToken is the access token used for explicit trace collection requests.
	accessToken        string
	maxReportingPeriod time.Duration // set by GrpcOptions.MaxReportingPeriod
	reconnectPeriod    time.Duration // set by GrpcOptions.ReconnectPeriod
	reportingTimeout   time.Duration // set by GrpcOptions.ReportTimeout

	// Remote service that will receive reports.
	address       string
	grpcClient    collectorpb.CollectorServiceClient
	connTimestamp time.Time
	dialOptions   []grpc.DialOption

	// For testing purposes only
	grpcConnectorFactory ConnectorFactory
}

func newGrpcCollectorClient(opts Options) (*grpcCollectorClient, error) {
	rec := &grpcCollectorClient{
		accessToken:          opts.AccessToken,
		maxReportingPeriod:   opts.ReportingPeriod,
		reconnectPeriod:      opts.ReconnectPeriod,
		reportingTimeout:     opts.ReportTimeout,
		dialOptions:          opts.DialOptions,
		grpcConnectorFactory: opts.ConnFactory,
	}

	if len(opts.Collector.Scheme) > 0 {
		rec.address = opts.Collector.urlWithoutPath()
	} else {
		rec.address = opts.Collector.SocketAddress()
	}

	rec.dialOptions = append(rec.dialOptions, grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(opts.GRPCMaxCallSendMsgSizeBytes)))
	if opts.Collector.Plaintext {
		rec.dialOptions = append(rec.dialOptions, grpc.WithInsecure())
		return rec, nil
	}

	if len(opts.Collector.CustomCACertFile) > 0 {
		creds, err := credentials.NewClientTLSFromFile(opts.Collector.CustomCACertFile, "")
		if err != nil {
			return nil, err
		}
		rec.dialOptions = append(rec.dialOptions, grpc.WithTransportCredentials(creds))
	} else {
		rec.dialOptions = append(rec.dialOptions, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, "")))
	}

	return rec, nil
}

func (client *grpcCollectorClient) ConnectClient() (Connection, error) {
	now := time.Now()
	var conn Connection
	if client.grpcConnectorFactory != nil {
		uncheckedClient, transport, err := client.grpcConnectorFactory()
		if err != nil {
			return nil, err
		}

		grpcClient, ok := uncheckedClient.(collectorpb.CollectorServiceClient)
		if !ok {
			return nil, fmt.Errorf("gRPC connector factory did not provide valid client")
		}

		conn = transport
		client.grpcClient = grpcClient
	} else {
		transport, err := grpc.Dial(client.address, client.dialOptions...)
		if err != nil {
			return nil, err
		}

		conn = transport
		client.grpcClient = collectorpb.NewCollectorServiceClient(transport)
	}
	client.connTimestamp = now
	return conn, nil
}

func (client *grpcCollectorClient) ShouldReconnect() bool {
	return time.Since(client.connTimestamp) > client.reconnectPeriod
}

func (client *grpcCollectorClient) Report(ctx context.Context, req reportRequest) (collectorResponse, error) {
	if req.protoRequest == nil {
		return nil, fmt.Errorf("protoRequest cannot be null")
	}

	ctx = metadata.NewOutgoingContext(
		ctx,
		metadata.Pairs(
			accessTokenHeader,
			client.accessToken,
		),
	)

	resp, err := client.grpcClient.Report(ctx, req.protoRequest)
	if err != nil {
		return nil, err
	}
	return protoResponse{ReportResponse: resp}, nil
}

func (client *grpcCollectorClient) Translate(protoRequest *collectorpb.ReportRequest) (reportRequest, error) {
	return reportRequest{protoRequest: protoRequest}, nil
}

type protoResponse struct {
	*collectorpb.ReportResponse
}

func (res protoResponse) Disable() bool {
	for _, command := range res.GetCommands() {
		if command.Disable {
			return true
		}
	}
	return false
}

func (res protoResponse) DevMode() bool {
	for _, command := range res.GetCommands() {
		if command.DevMode {
			return true
		}
	}
	return false
}
