package reader

import (
	"github.com/prometheus/client_golang/api"
	"net"
	"net/http"
	"time"
)

var Client api.Client

func Init() error {
	var err error
	Client, err = api.NewClient(api.Config{
		RoundTripper: &http.Transport{
			// TLSClientConfig: tlsConfig,
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   time.Duration(10000) * time.Millisecond,
				KeepAlive: time.Duration(30000) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(30000) * time.Millisecond,
			TLSHandshakeTimeout:   time.Duration(30000) * time.Millisecond,
			ExpectContinueTimeout: time.Duration(1000) * time.Millisecond,
			MaxConnsPerHost:       0,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       time.Duration(90000) * time.Millisecond,
		},
	})

	if err != nil {
		return err
	}

	return nil
}
