package reader

import (
	"github.com/didi/nightingale/v5/src/pkg/prom"
	webapi_prom "github.com/didi/nightingale/v5/src/webapi/prom"
	"github.com/prometheus/client_golang/api"
)

type ReaderType struct {
	Clients map[string]prom.API
}

var Reader ReaderType

func Init() error {

	Reader = ReaderType{
		Clients: make(map[string]prom.API),
	}

	clusterTypes := webapi_prom.Clusters.GetClusters()

	for _, clusterType := range clusterTypes {

		cli, err := api.NewClient(api.Config{
			Address:      clusterType.Opts.Prom,
			RoundTripper: clusterType.Transport,
		})

		if err != nil {
			return err
		}

		Reader.Clients[clusterType.Opts.Name] = prom.NewAPI(cli, prom.ClientOptions{
			BasicAuthUser: clusterType.Opts.BasicAuthUser,
			BasicAuthPass: clusterType.Opts.BasicAuthPass,
			Headers:       clusterType.Opts.Headers,
		})
	}
	return nil
}
