package reader

import (
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/api"
)

type Options struct {
	Url           string
	BasicAuthUser string
	BasicAuthPass string

	Timeout               int64
	DialTimeout           int64
	TLSHandshakeTimeout   int64
	ExpectContinueTimeout int64
	IdleConnTimeout       int64
	KeepAlive             int64

	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

type ReaderType struct {
	Opts   Options
	Client API
}

var Reader ReaderType

func Init(opts Options) error {
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

	Reader = ReaderType{
		Opts:   opts,
		Client: NewAPI(cli),
	}

	return nil
}
