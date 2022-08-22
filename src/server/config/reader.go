package config

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/prom"
	"github.com/prometheus/client_golang/api"
	"github.com/toolkits/pkg/logger"
)

func InitReader() error {
	rf := strings.ToLower(strings.TrimSpace(C.ReaderFrom))
	if rf == "" || rf == "config" {
		return setClientFromPromOption(C.ClusterName, C.Reader)
	}

	if rf == "database" {
		return initFromDatabase()
	}

	return fmt.Errorf("invalid configuration ReaderFrom: %s", rf)
}

func initFromDatabase() error {
	go func() {
		for {
			loadFromDatabase()
			time.Sleep(time.Second)
		}
	}()
	return nil
}

func loadFromDatabase() {
	cluster, err := models.AlertingEngineGetCluster(C.Heartbeat.Endpoint)
	if err != nil {
		logger.Errorf("failed to get current cluster, error: %v", err)
		return
	}

	if cluster == "" {
		ReaderClient.Reset()
		logger.Warning("no datasource binded to me")
		return
	}

	ckey := "prom." + cluster + ".option"
	cval, err := models.ConfigsGet(ckey)
	if err != nil {
		logger.Errorf("failed to get ckey: %s, error: %v", ckey, err)
		return
	}

	if cval == "" {
		ReaderClient.Reset()
		return
	}

	var po PromOption
	err = json.Unmarshal([]byte(cval), &po)
	if err != nil {
		logger.Errorf("failed to unmarshal PromOption: %s", err)
		return
	}

	if ReaderClient.IsNil() {
		// first time
		if err = setClientFromPromOption(cluster, po); err != nil {
			logger.Errorf("failed to setClientFromPromOption: %v", err)
			return
		}

		PromOptions.Sets(cluster, po)
		return
	}

	localPo, has := PromOptions.Get(cluster)
	if !has || !localPo.Equal(po) {
		if err = setClientFromPromOption(cluster, po); err != nil {
			logger.Errorf("failed to setClientFromPromOption: %v", err)
			return
		}

		PromOptions.Sets(cluster, po)
		return
	}
}

func newClientFromPromOption(po PromOption) (api.Client, error) {
	return api.NewClient(api.Config{
		Address: po.Url,
		RoundTripper: &http.Transport{
			// TLSClientConfig: tlsConfig,
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: time.Duration(po.DialTimeout) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(po.Timeout) * time.Millisecond,
			MaxIdleConnsPerHost:   po.MaxIdleConnsPerHost,
		},
	})
}

func setClientFromPromOption(clusterName string, po PromOption) error {
	if clusterName == "" {
		return fmt.Errorf("argument clusterName is blank")
	}

	if po.Url == "" {
		return fmt.Errorf("prometheus url is blank")
	}

	cli, err := newClientFromPromOption(po)
	if err != nil {
		return fmt.Errorf("failed to newClientFromPromOption: %v", err)
	}

	ReaderClient.Set(clusterName, prom.NewAPI(cli, prom.ClientOptions{
		BasicAuthUser: po.BasicAuthUser,
		BasicAuthPass: po.BasicAuthPass,
		Headers:       po.Headers,
	}))

	return nil
}
