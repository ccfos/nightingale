package naming

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

type Naming struct {
	ctx             *ctx.Context
	heartbeatConfig aconf.HeartbeatConfig
	astats          *astats.Stats
}

func NewNaming(ctx *ctx.Context, heartbeat aconf.HeartbeatConfig, alertStats *astats.Stats) *Naming {
	naming := &Naming{
		ctx:             ctx,
		heartbeatConfig: heartbeat,
		astats:          alertStats,
	}
	naming.Heartbeats()
	return naming
}

// local servers
var localss map[int64]string
var localHostServers map[string]string

func (n *Naming) Heartbeats() error {
	localss = make(map[int64]string)
	localHostServers = make(map[string]string)
	if err := n.heartbeat(); err != nil {
		fmt.Println("failed to heartbeat:", err)
		return err
	}

	go n.loopHeartbeat()
	go n.loopDeleteInactiveInstances()
	return nil
}

func (n *Naming) loopDeleteInactiveInstances() {
	if !n.ctx.IsCenter {
		return
	}

	interval := time.Duration(10) * time.Minute
	for {
		time.Sleep(interval)
		n.DeleteInactiveInstances()
	}
}

func (n *Naming) DeleteInactiveInstances() {
	err := models.DB(n.ctx).Where("clock < ?", time.Now().Unix()-600).Delete(new(models.AlertingEngines)).Error
	if err != nil {
		logger.Errorf("delete inactive instances err:%v", err)
	}
}

func (n *Naming) loopHeartbeat() {
	interval := time.Duration(n.heartbeatConfig.Interval) * time.Millisecond
	for {
		time.Sleep(interval)
		if err := n.heartbeat(); err != nil {
			logger.Warning(err)
		}
	}
}

func (n *Naming) heartbeat() error {
	var datasourceIds []int64
	var err error

	// 在页面上维护实例和集群的对应关系
	datasourceIds, err = models.GetDatasourceIdsByEngineName(n.ctx, n.heartbeatConfig.EngineName)
	if err != nil {
		return err
	}

	if len(datasourceIds) == 0 {
		err := models.AlertingEngineHeartbeatWithCluster(n.ctx, n.heartbeatConfig.Endpoint, n.heartbeatConfig.EngineName, 0)
		if err != nil {
			logger.Warningf("heartbeat with cluster %s err:%v", "", err)
			n.astats.CounterHeartbeatErrorTotal.WithLabelValues().Inc()
		}
	} else {
		for i := 0; i < len(datasourceIds); i++ {
			err := models.AlertingEngineHeartbeatWithCluster(n.ctx, n.heartbeatConfig.Endpoint, n.heartbeatConfig.EngineName, datasourceIds[i])
			if err != nil {
				logger.Warningf("heartbeat with cluster %d err:%v", datasourceIds[i], err)
				n.astats.CounterHeartbeatErrorTotal.WithLabelValues().Inc()
			}
		}
	}

	if len(datasourceIds) == 0 {
		DatasourceHashRing.Clear(n.heartbeatConfig.EngineName)
		for dsId := range localss {
			delete(localss, dsId)
		}
	}

	newDatasource := make(map[int64]struct{})
	for i := 0; i < len(datasourceIds); i++ {
		newDatasource[datasourceIds[i]] = struct{}{}
		servers, err := n.ActiveServers(datasourceIds[i])
		if err != nil {
			logger.Warningf("hearbeat %d get active server err:%v", datasourceIds[i], err)
			n.astats.CounterHeartbeatErrorTotal.WithLabelValues().Inc()
			continue
		}

		sort.Strings(servers)
		newss := strings.Join(servers, " ")

		oldss, exists := localss[datasourceIds[i]]
		if exists && oldss == newss {
			continue
		}

		RebuildConsistentHashRing(fmt.Sprintf("%d", datasourceIds[i]), servers)
		localss[datasourceIds[i]] = newss
	}

	for dsId := range localss {
		if _, exists := newDatasource[dsId]; !exists {
			delete(localss, dsId)
			DatasourceHashRing.Del(fmt.Sprintf("%d", dsId))
		}
	}

	// host 告警使用的是 hash ring
	err = models.AlertingEngineHeartbeatWithCluster(n.ctx, n.heartbeatConfig.Endpoint, n.heartbeatConfig.EngineName, HostDatasource)
	if err != nil {
		logger.Warningf("heartbeat with cluster %s err:%v", "", err)
		n.astats.CounterHeartbeatErrorTotal.WithLabelValues().Inc()
	}

	servers, err := n.ActiveServersByEngineName()
	if err != nil {
		logger.Warningf("hearbeat %d get active server err:%v", HostDatasource, err)
		n.astats.CounterHeartbeatErrorTotal.WithLabelValues().Inc()
		return nil
	}

	sort.Strings(servers)
	newss := strings.Join(servers, " ")

	oldss, exists := localHostServers[n.heartbeatConfig.EngineName]
	if exists && oldss == newss {
		return nil
	}

	RebuildConsistentHashRing(n.heartbeatConfig.EngineName, servers)
	localHostServers[n.heartbeatConfig.EngineName] = newss

	return nil
}

func (n *Naming) ActiveServers(datasourceId int64) ([]string, error) {
	if datasourceId == -1 {
		return nil, fmt.Errorf("cluster is empty")
	}

	if !n.ctx.IsCenter {
		lst, err := poster.GetByUrls[[]string](n.ctx, "/v1/n9e/servers-active?dsid="+fmt.Sprintf("%d", datasourceId))
		return lst, err
	}

	// 30秒内有心跳，就认为是活的
	return models.AlertingEngineGetsInstances(n.ctx, "datasource_id = ? and clock > ?", datasourceId, time.Now().Unix()-30)
}

func (n *Naming) ActiveServersByEngineName() ([]string, error) {
	if !n.ctx.IsCenter {
		lst, err := poster.GetByUrls[[]string](n.ctx, "/v1/n9e/servers-active?engine_name="+n.heartbeatConfig.EngineName)
		return lst, err
	}

	// 30秒内有心跳，就认为是活的
	return models.AlertingEngineGetsInstances(n.ctx, "engine_cluster = ? and clock > ?", n.heartbeatConfig.EngineName, time.Now().Unix()-30)
}
