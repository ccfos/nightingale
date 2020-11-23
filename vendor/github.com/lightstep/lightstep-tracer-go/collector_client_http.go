package lightstep

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/lightstep/lightstep-tracer-common/golang/gogo/collectorpb"
)

var (
	acceptHeader      = http.CanonicalHeaderKey("Accept")
	contentTypeHeader = http.CanonicalHeaderKey("Content-Type")
)

const (
	collectorHTTPMethod = "POST"
	collectorHTTPPath   = "/api/v2/reports"
	protoContentType    = "application/octet-stream"
)

// grpcCollectorClient specifies how to send reports back to a LightStep
// collector via grpc.
type httpCollectorClient struct {
	// auth and runtime information
	accessToken string // accessToken is the access token used for explicit trace collection requests.

	tlsClientConfig *tls.Config
	reportTimeout   time.Duration
	reportingPeriod time.Duration

	// Remote service that will receive reports.
	url    *url.URL
	client *http.Client
}

type transportCloser struct {
	*http.Transport
}

func (closer transportCloser) Close() error {
	closer.CloseIdleConnections()
	return nil
}

func newHTTPCollectorClient(opts Options) (*httpCollectorClient, error) {
	url, err := url.Parse(opts.Collector.URL())
	if err != nil {
		fmt.Println("collector config does not produce valid url", err)
		return nil, err
	}
	url.Path = collectorHTTPPath

	tlsClientConfig, err := getTLSConfig(opts.Collector.CustomCACertFile)
	if err != nil {
		fmt.Println("failed to get TLSConfig: ", err)
		return nil, err
	}

	return &httpCollectorClient{
		accessToken:     opts.AccessToken,
		tlsClientConfig: tlsClientConfig,
		reportTimeout:   opts.ReportTimeout,
		reportingPeriod: opts.ReportingPeriod,
		url:             url,
	}, nil
}

// getTLSConfig returns a *tls.Config according to whether a user has supplied a customCACertFile. If they have,
// we return a TLSConfig that uses the custom CA cert as the lone Root CA. If not, we return nil which http.Transport
// will interpret as the default system defined Root CAs.
func getTLSConfig(customCACertFile string) (*tls.Config, error) {
	if len(customCACertFile) == 0 {
		return nil, nil
	}

	caCerts := x509.NewCertPool()
	cert, err := ioutil.ReadFile(customCACertFile)
	if err != nil {
		return nil, err
	}

	if !caCerts.AppendCertsFromPEM(cert) {
		return nil, fmt.Errorf("credentials: failed to append certificate")
	}

	return &tls.Config{RootCAs: caCerts}, nil
}

func (client *httpCollectorClient) ConnectClient() (Connection, error) {
	// Use a transport independent from http.DefaultTransport to provide sane
	// defaults that make sense in the context of the lightstep client. The
	// differences are mostly on setting timeouts based on the report timeout
	// and period.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   client.reportTimeout / 2,
			DualStack: true,
		}).DialContext,
		// The collector responses are very small, there is no point asking for
		// a compressed payload, explicitly disabling it.
		DisableCompression:     true,
		IdleConnTimeout:        2 * client.reportingPeriod,
		TLSHandshakeTimeout:    client.reportTimeout / 2,
		ResponseHeaderTimeout:  client.reportTimeout,
		ExpectContinueTimeout:  client.reportTimeout,
		MaxResponseHeaderBytes: 64 * 1024, // 64 KB, just a safeguard
		TLSClientConfig:        client.tlsClientConfig,
	}

	client.client = &http.Client{
		Transport: transport,
		Timeout:   client.reportTimeout,
	}

	return transportCloser{transport}, nil
}

func (client *httpCollectorClient) ShouldReconnect() bool {
	// http.Transport will handle connection reuse under the hood
	return false
}

func (client *httpCollectorClient) Report(context context.Context, req reportRequest) (collectorResponse, error) {
	if req.httpRequest == nil {
		return nil, fmt.Errorf("httpRequest cannot be null")
	}

	httpResponse, err := client.client.Do(req.httpRequest.WithContext(context))
	if err != nil {
		return nil, err
	}
	defer httpResponse.Body.Close()

	response, err := client.toResponse(httpResponse)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (client *httpCollectorClient) Translate(protoRequest *collectorpb.ReportRequest) (reportRequest, error) {
	httpRequest, err := client.toRequest(protoRequest)
	if err != nil {
		return reportRequest{}, err
	}
	return reportRequest{
		httpRequest: httpRequest,
	}, nil
}

func (client *httpCollectorClient) toRequest(
	protoRequest *collectorpb.ReportRequest,
) (*http.Request, error) {
	buf, err := proto.Marshal(protoRequest)
	if err != nil {
		return nil, err
	}

	requestBody := bytes.NewReader(buf)

	request, err := http.NewRequest(collectorHTTPMethod, client.url.String(), requestBody)
	if err != nil {
		return nil, err
	}
	request.Header.Set(contentTypeHeader, protoContentType)
	request.Header.Set(acceptHeader, protoContentType)
	request.Header.Set(accessTokenHeader, client.accessToken)

	return request, nil
}

func (client *httpCollectorClient) toResponse(response *http.Response) (collectorResponse, error) {
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code (%d) is not ok", response.StatusCode)
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	resp := &collectorpb.ReportResponse{}
	if err := proto.Unmarshal(body, resp); err != nil {
		return nil, err
	}

	return protoResponse{ReportResponse: resp}, nil
}
