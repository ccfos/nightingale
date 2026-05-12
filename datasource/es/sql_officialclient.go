package es

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"

	elasticsearch8 "github.com/elastic/go-elasticsearch/v8"
)

// productCheckTransport wraps an http.RoundTripper to ensure the
// X-Elastic-Product header is present in every response.
// ES < 7.14 does not return this header, but the go-elasticsearch/v8
// SDK requires it to pass its built-in product verification check.
// The SQL wire protocol is identical across 7.x/8.x/9.x, so this
// is safe for genuine Elasticsearch clusters of any supported version.
type productCheckTransport struct {
	base http.RoundTripper
}

func (t *productCheckTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if resp.Header.Get("X-Elastic-Product") == "" {
		resp.Header.Set("X-Elastic-Product", "Elasticsearch")
	}
	return resp, nil
}

var clientCache sync.Map // map[*Elasticsearch]*elasticsearch8.Client

// officialClient returns a cached go-elasticsearch/v8 client for the given datasource.
// The v8 SDK's HTTP-level SQL API is compatible with ES 7.x, 8.x, and 9.x.
func officialClient(escli *Elasticsearch) (*elasticsearch8.Client, error) {
	if cached, ok := clientCache.Load(escli); ok {
		return cached.(*elasticsearch8.Client), nil
	}

	base := http.DefaultTransport
	if escli.TLS.SkipTlsVerify {
		base = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec
			},
		}
	}

	cfg := elasticsearch8.Config{
		Addresses: escli.Nodes,
		Transport: &productCheckTransport{base: base},
	}

	if escli.Basic.Enable {
		cfg.Username = escli.Basic.Username
		cfg.Password = escli.Basic.Password
	}

	if len(escli.Headers) > 0 {
		cfg.Header = make(http.Header)
		for k, v := range escli.Headers {
			cfg.Header.Set(k, v)
		}
	}

	client, err := elasticsearch8.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch client: %w", err)
	}

	clientCache.Store(escli, client)
	return client, nil
}
