package naming

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

type Naming struct {
	ctx       *ctx.Context
	Heartbeat aconf.HeartbeatConfig
}

func NewNaming(ctx *ctx.Context, heartbeat aconf.HeartbeatConfig) *Naming {
	naming := &Naming{
		ctx:       ctx,
		Heartbeat: heartbeat,
	}
	naming.Heartbeats()
	return naming
}

// local servers
var localss map[int64]string

func (n *Naming) Heartbeats() error {
	localss = make(map[int64]string)
	if err := n.heartbeat(); err != nil {
		fmt.Println("failed to heartbeat:", err)
		return err
	}

	go n.loopHeartbeat()
	go n.loopDeleteInactiveInstances()
	return nil
}

func (n *Naming) loopDeleteInactiveInstances() {
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
	interval := time.Duration(n.Heartbeat.Interval) * time.Millisecond
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
	datasourceIds, err = models.GetDatasourceIdsByClusterName(n.ctx, n.Heartbeat.ClusterName)
	if err != nil {
		return err
	}

	if len(datasourceIds) == 0 {
		err := models.AlertingEngineHeartbeatWithCluster(n.ctx, n.Heartbeat.Endpoint, n.Heartbeat.ClusterName, 0)
		if err != nil {
			logger.Warningf("heartbeat with cluster %s err:%v", "", err)
		}
	} else {
		for i := 0; i < len(datasourceIds); i++ {
			err := models.AlertingEngineHeartbeatWithCluster(n.ctx, n.Heartbeat.Endpoint, n.Heartbeat.ClusterName, datasourceIds[i])
			if err != nil {
				logger.Warningf("heartbeat with cluster %d err:%v", datasourceIds[i], err)
			}
		}
	}

	for i := 0; i < len(datasourceIds); i++ {
		servers, err := n.ActiveServers(datasourceIds[i])
		if err != nil {
			logger.Warningf("hearbeat %d get active server err:%v", datasourceIds[i], err)
			continue
		}

		sort.Strings(servers)
		newss := strings.Join(servers, " ")

		oldss, exists := localss[datasourceIds[i]]
		if exists && oldss == newss {
			continue
		}

		RebuildConsistentHashRing(datasourceIds[i], servers)
		localss[datasourceIds[i]] = newss
	}

	return nil
}

func (n *Naming) ActiveServers(datasourceId int64) ([]string, error) {
	if datasourceId == -1 {
		return nil, fmt.Errorf("cluster is empty")
	}

	// 30秒内有心跳，就认为是活的
	return models.AlertingEngineGetsInstances(n.ctx, "datasource_id = ? and clock > ?", datasourceId, time.Now().Unix()-30)
}

func (n *Naming) AllActiveServers() ([]string, error) {
	// 30秒内有心跳，就认为是活的
	return models.AlertingEngineGetsInstances(n.ctx, "clock > ?", time.Now().Unix()-30)
}
