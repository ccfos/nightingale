package reader

import (
	"github.com/didi/nightingale/v5/src/pkg/prom"
	"github.com/didi/nightingale/v5/src/webapi/config"
	"github.com/prometheus/client_golang/api"
	"net"
	"net/http"
	"net/url"
	"time"
)

type ReaderType struct {
	Clients map[string]prom.API
}

var Reader ReaderType

func Init() error {

	Reader = ReaderType{
		Clients: make(map[string]prom.API),
	}

	count := len(config.C.Clusters)
	for i := 0; i < count; i++ {
		cluster := config.C.Clusters[i]

		target, err := url.Parse(cluster.Prom)
		if err != nil {
			return err
		}
		url := target.Scheme + "://" + target.Host
		cli, err := api.NewClient(api.Config{

			Address: url,
			RoundTripper: &http.Transport{
				// TLSClientConfig: tlsConfig,
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   time.Duration(cluster.Timeout) * time.Millisecond,
					KeepAlive: time.Duration(cluster.KeepAlive) * time.Millisecond,
				}).DialContext,
				ResponseHeaderTimeout: time.Duration(cluster.Timeout) * time.Millisecond,
				MaxIdleConnsPerHost:   cluster.MaxIdleConnsPerHost,
			},
		})

		Reader.Clients[cluster.Name] = prom.NewAPI(cli, prom.ClientOptions{
			BasicAuthUser: cluster.BasicAuthUser,
			BasicAuthPass: cluster.BasicAuthPass,
			Headers:       cluster.Headers,
		})
	}
	return nil
}
