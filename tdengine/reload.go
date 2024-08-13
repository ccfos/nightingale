package tdengine

import (
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

func NewTdengineClient(ctx *ctx.Context, heartbeat aconf.HeartbeatConfig) *TdengineClientMap {
	pc := &TdengineClientMap{
		ReaderClients: make(map[int64]*tdengineClient),
		heartbeat:     heartbeat,
		ctx:           ctx,
	}
	pc.InitReader()
	return pc
}

func (pc *TdengineClientMap) InitReader() error {
	go func() {
		for {
			pc.loadFromDatabase()
			time.Sleep(time.Second)
		}
	}()
	return nil
}

func (pc *TdengineClientMap) loadFromDatabase() {
	var datasources []*models.Datasource
	var err error
	if !pc.ctx.IsCenter {
		datasources, err = poster.GetByUrls[[]*models.Datasource](pc.ctx, "/v1/n9e/datasources?typ="+models.TDENGINE)
		if err != nil {
			logger.Errorf("failed to get datasources, error: %v", err)
			return
		}
		for i := 0; i < len(datasources); i++ {
			datasources[i].FE2DB()
		}
	} else {
		datasources, err = models.GetDatasourcesGetsBy(pc.ctx, models.TDENGINE, "", "", "")
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

		po := TdengineOption{
			DatasourceName:      ds.Name,
			Url:                 ds.HTTPJson.Url,
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

		if len(ds.SettingsJson) > 0 {
			for k, v := range ds.SettingsJson {
				if strings.Contains(k, "token") {
					po.Headers = append(po.Headers, "Authorization")
					po.Headers = append(po.Headers, "Taosd "+v.(string))
				}
			}
		}

		newCluster[dsId] = struct{}{}
		if pc.IsNil(dsId) {
			// first time
			if err = pc.setClientFromTdengineOption(dsId, po); err != nil {
				logger.Errorf("failed to setClientFromTdengineOption po:%+v err:%v", po, err)
				continue
			}

			logger.Info("setClientFromTdengineOption success: ", dsId)
			TdengineOptions.Set(dsId, po)
			continue
		}

		localPo, has := TdengineOptions.Get(dsId)
		if !has || !localPo.Equal(po) {
			if err = pc.setClientFromTdengineOption(dsId, po); err != nil {
				logger.Errorf("failed to setClientFromTdengineOption: %v", err)
				continue
			}

			TdengineOptions.Set(dsId, po)
		}
	}

	// delete useless cluster
	oldIds := pc.GetDatasourceIds()
	for _, oldId := range oldIds {
		if _, has := newCluster[oldId]; !has {
			pc.Del(oldId)
			TdengineOptions.Del(oldId)
			logger.Info("delete cluster: ", oldId)
		}
	}
}

func (pc *TdengineClientMap) setClientFromTdengineOption(datasourceId int64, po TdengineOption) error {
	if datasourceId < 0 {
		return fmt.Errorf("argument clusterName is blank")
	}

	if po.Url == "" {
		return fmt.Errorf("prometheus url is blank")
	}

	reader := newTdengine(po)

	logger.Debugf("setClientFromTdengineOption: %d, %+v", datasourceId, po)
	pc.Set(datasourceId, reader)

	return nil
}
