package reader

import (
	"net"
	"net/http"
	"time"

	"github.com/didi/nightingale/v5/src/pkg/prom"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/prometheus/client_golang/api"
)

var Client prom.API

func Init(opts config.ReaderOptions) error {
	cli, err := api.NewClient(api.Config{
		Address: opts.Url,
		RoundTripper: &http.Transport{
			// TLSClientConfig: tlsConfig,
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   time.Duration(opts.DialTimeout) * time.Millisecond,
				KeepAlive: time.Duration(opts.KeepAlive) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(opts.Timeout) * time.Millisecond,
			TLSHandshakeTimeout:   time.Duration(opts.TLSHandshakeTimeout) * time.Millisecond,
			ExpectContinueTimeout: time.Duration(opts.ExpectContinueTimeout) * time.Millisecond,
			MaxConnsPerHost:       opts.MaxConnsPerHost,
			MaxIdleConns:          opts.MaxIdleConns,
			MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
			IdleConnTimeout:       time.Duration(opts.IdleConnTimeout) * time.Millisecond,
		},
	})

	if err != nil {
		return err
	}

	Client = prom.NewAPI(cli, prom.ClientOptions{
		BasicAuthUser: opts.BasicAuthUser,
		BasicAuthPass: opts.BasicAuthPass,
		Headers:       opts.Headers,
	})

	return nil
}
