package prom

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/prometheus/client_golang/api"
	"github.com/toolkits/pkg/logger"
)

func NewPromClient(ctx *ctx.Context, heartbeat aconf.HeartbeatConfig) *PromClientMap {
	pc := &PromClientMap{
		ReaderClients: make(map[int64]prom.API),
		WriterClients: make(map[int64]prom.WriterType),
		ctx:           ctx,
		heartbeat:     heartbeat,
	}
	pc.InitReader()
	return pc
}

func (pc *PromClientMap) InitReader() error {
	go func() {
		for {
			pc.loadFromDatabase()
			time.Sleep(time.Second)
		}
	}()
	return nil
}

type PromSetting struct {
	WriterAddr string `json:"write_addr"`
}

func (pc *PromClientMap) loadFromDatabase() {
	datasources, err := models.GetDatasourcesGetsBy(pc.ctx, models.PROMETHEUS, "", "", 1000, 0)
	if err != nil {
		logger.Errorf("failed to get datasources, error: %v", err)
		return
	}
	newCluster := make(map[int64]struct{})
	for _, ds := range datasources {
		dsId := ds.Id
		var header []string
		for k, v := range ds.HTTPJson.Headers {
			header = append(header, k)
			header = append(header, v)
		}

		var promSetting PromSetting
		if ds.Settings != "" {
			err := json.Unmarshal([]byte(ds.Settings), &promSetting)
			if err != nil {
				logger.Errorf("failed to unmarshal prom settings, error: %v", err)
				continue
			}
		}

		po := PromOption{
			ClusterName:         ds.Name,
			Url:                 ds.HTTPJson.Url,
			WriteAddr:           promSetting.WriterAddr,
			BasicAuthUser:       ds.AuthJson.BasicAuthUser,
			BasicAuthPass:       ds.AuthJson.BasicAuthPassword,
			Timeout:             ds.HTTPJson.Timeout,
			DialTimeout:         ds.HTTPJson.DialTimeout,
			MaxIdleConnsPerHost: ds.HTTPJson.MaxIdleConnsPerHost,
			Headers:             header,
		}

		newCluster[dsId] = struct{}{}
		if pc.IsNil(dsId) {
			// first time
			if err = pc.setClientFromPromOption(dsId, po); err != nil {
				logger.Errorf("failed to setClientFromPromOption: %v", err)
				continue
			}

			logger.Info("setClientFromPromOption success: ", dsId)
			PromOptions.Set(dsId, po)
			continue
		}

		localPo, has := PromOptions.Get(dsId)
		if !has || !localPo.Equal(po) {
			if err = pc.setClientFromPromOption(dsId, po); err != nil {
				logger.Errorf("failed to setClientFromPromOption: %v", err)
				continue
			}

			PromOptions.Set(dsId, po)
		}
	}

	// delete useless cluster
	oldIds := pc.GetDatasourceIds()
	for _, oldId := range oldIds {
		if _, has := newCluster[oldId]; !has {
			pc.Del(oldId)
			PromOptions.Del(oldId)
			logger.Info("delete cluster: ", oldId)
		}
	}
}

func (pc *PromClientMap) newReaderClientFromPromOption(po PromOption) (api.Client, error) {
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

func (pc *PromClientMap) newWriterClientFromPromOption(po PromOption) (api.Client, error) {
	return api.NewClient(api.Config{
		Address: po.WriteAddr,
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

func (pc *PromClientMap) setClientFromPromOption(datasourceId int64, po PromOption) error {
	if datasourceId < 0 {
		return fmt.Errorf("argument clusterName is blank")
	}

	if po.Url == "" {
		return fmt.Errorf("prometheus url is blank")
	}

	readerCli, err := pc.newReaderClientFromPromOption(po)
	if err != nil {
		return fmt.Errorf("failed to newClientFromPromOption: %v", err)
	}

	reader := prom.NewAPI(readerCli, prom.ClientOptions{
		BasicAuthUser: po.BasicAuthUser,
		BasicAuthPass: po.BasicAuthPass,
		Headers:       po.Headers,
	})

	writerCli, err := pc.newWriterClientFromPromOption(po)
	if err != nil {
		return fmt.Errorf("failed to newClientFromPromOption: %v", err)
	}

	w := prom.NewWriter(writerCli, prom.ClientOptions{
		Url:           po.WriteAddr,
		BasicAuthUser: po.BasicAuthUser,
		BasicAuthPass: po.BasicAuthPass,
		Headers:       po.Headers,
	})

	logger.Debugf("setClientFromPromOption: %d, %+v", datasourceId, po)
	pc.Set(datasourceId, reader, w)

	return nil
}
