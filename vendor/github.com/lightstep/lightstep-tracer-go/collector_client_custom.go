package lightstep

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/lightstep/lightstep-tracer-common/golang/gogo/collectorpb"
)

func newCustomCollector(opts Options) *customCollectorClient {
	return &customCollectorClient{collector: opts.CustomCollector}
}

type customCollectorClient struct {
	collector Collector
}

func (client *customCollectorClient) Report(ctx context.Context, req reportRequest) (collectorResponse, error) {
	if req.protoRequest == nil {
		return nil, fmt.Errorf("protoRequest cannot be null")
	}

	resp, err := client.collector.Report(ctx, req.protoRequest)
	if err != nil {
		return nil, err
	}
	return protoResponse{ReportResponse: resp}, nil
}

func (client *customCollectorClient) Translate(protoRequest *collectorpb.ReportRequest) (reportRequest, error) {
	return reportRequest{protoRequest: protoRequest}, nil
}

func (customCollectorClient) ConnectClient() (Connection, error) {
	return ioutil.NopCloser(nil), nil
}

func (customCollectorClient) ShouldReconnect() bool {
	return false
}
