package prom

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
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

func (pc *PromClientMap) loadFromDatabase() {
	var datasources []*models.Datasource
	var err error
	if !pc.ctx.IsCenter {
		datasources, err = poster.GetByUrls[[]*models.Datasource](pc.ctx, "/v1/n9e/datasources?typ="+models.PROMETHEUS)
		if err != nil {
			logger.Errorf("failed to get datasources, error: %v", err)
			return
		}
		for i := 0; i < len(datasources); i++ {
			datasources[i].FE2DB()
		}
	} else {
		datasources, err = models.GetDatasourcesGetsBy(pc.ctx, models.PROMETHEUS, "", "", "")
		if err != nil {
			logger.Errorf("failed to get datasources, error: %v", err)
			return
		}
	}

	newCluster := make(map[int64]struct{})
	for _, ds := range datasources {
		dsId := ds.Id
		var header []string
		for k, v := range ds.HTTPJson.Headers {
			header = append(header, k)
			header = append(header, v)
		}

		var writeAddr string
		var internalAddr string
		for k, v := range ds.SettingsJson {
			if strings.Contains(k, "write_addr") {
				writeAddr = v.(string)
			} else if strings.Contains(k, "internal_addr") && v.(string) != "" {
				internalAddr = v.(string)
			}
		}

		po := PromOption{
			ClusterName:         ds.Name,
			Url:                 ds.HTTPJson.Url,
			WriteAddr:           writeAddr,
			BasicAuthUser:       ds.AuthJson.BasicAuthUser,
			BasicAuthPass:       ds.AuthJson.BasicAuthPassword,
			Timeout:             ds.HTTPJson.Timeout,
			DialTimeout:         ds.HTTPJson.DialTimeout,
			MaxIdleConnsPerHost: ds.HTTPJson.MaxIdleConnsPerHost,
			Headers:             header,
		}

		if strings.HasPrefix(ds.HTTPJson.Url, "https") {
			po.UseTLS = true
			po.InsecureSkipVerify = ds.HTTPJson.TLS.SkipTlsVerify
		}

		if internalAddr != "" && !pc.ctx.IsCenter {
			// internal addr is set, use internal addr when edge mode
			po.Url = internalAddr
		}

		newCluster[dsId] = struct{}{}
		if pc.IsNil(dsId) {
			// first time
			if err = pc.setClientFromPromOption(dsId, po); err != nil {
				logger.Errorf("failed to setClientFromPromOption po:%+v err:%v", po, err)
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
	tlsConfig, _ := po.TLSConfig()

	return api.NewClient(api.Config{
		Address: po.Url,
		RoundTripper: &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: time.Duration(po.DialTimeout) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(po.Timeout) * time.Millisecond,
			MaxIdleConnsPerHost:   po.MaxIdleConnsPerHost,
		},
	})
}

func (pc *PromClientMap) newWriterClientFromPromOption(po PromOption) (api.Client, error) {
	tlsConfig, _ := po.TLSConfig()

	return api.NewClient(api.Config{
		Address: po.WriteAddr,
		RoundTripper: &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
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
