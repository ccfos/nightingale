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
		if len(C.Readers) == 0 {
			C.Reader.ClusterName = C.ClusterName
			C.Readers = append(C.Readers, C.Reader)
		}

		for _, reader := range C.Readers {
			err := setClientFromPromOption(reader.ClusterName, reader)
			if err != nil {
				logger.Errorf("failed to setClientFromPromOption: %v", err)
				continue
			}
		}
		return nil
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
	clusters, err := models.AlertingEngineGetClusters(C.Heartbeat.Endpoint)
	if err != nil {
		logger.Errorf("failed to get current cluster, error: %v", err)
		return
	}

	if len(clusters) == 0 {
		ReaderClients.Reset()
		logger.Warning("no datasource binded to me")
		return
	}

	newCluster := make(map[string]struct{})
	for _, cluster := range clusters {
		newCluster[cluster] = struct{}{}
		ckey := "prom." + cluster + ".option"
		cval, err := models.ConfigsGet(ckey)
		if err != nil {
			logger.Errorf("failed to get ckey: %s, error: %v", ckey, err)
			continue
		}

		if cval == "" {
			logger.Warningf("ckey: %s is empty", ckey)
			continue
		}

		var po PromOption
		err = json.Unmarshal([]byte(cval), &po)
		if err != nil {
			logger.Errorf("failed to unmarshal PromOption: %s", err)
			continue
		}

		if ReaderClients.IsNil(cluster) {
			// first time
			if err = setClientFromPromOption(cluster, po); err != nil {
				logger.Errorf("failed to setClientFromPromOption: %v", err)
				continue
			}

			logger.Info("setClientFromPromOption success: ", cluster)
			PromOptions.Sets(cluster, po)
			continue
		}

		localPo, has := PromOptions.Get(cluster)
		if !has || !localPo.Equal(po) {
			if err = setClientFromPromOption(cluster, po); err != nil {
				logger.Errorf("failed to setClientFromPromOption: %v", err)
				continue
			}

			PromOptions.Sets(cluster, po)
		}
	}

	// delete useless cluster
	oldClusters := ReaderClients.GetClusterNames()
	for _, oldCluster := range oldClusters {
		if _, has := newCluster[oldCluster]; !has {
			ReaderClients.Del(oldCluster)
			PromOptions.Del(oldCluster)
			logger.Info("delete cluster: ", oldCluster)
		}
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

	ReaderClients.Set(clusterName, prom.NewAPI(cli, prom.ClientOptions{
		BasicAuthUser: po.BasicAuthUser,
		BasicAuthPass: po.BasicAuthPass,
		Headers:       po.Headers,
	}))

	return nil
}
