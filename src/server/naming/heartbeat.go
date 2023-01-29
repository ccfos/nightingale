package naming

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
)

// local servers
var localss map[string]string

func Heartbeat(ctx context.Context) error {
	localss = make(map[string]string)
	if err := heartbeat(); err != nil {
		fmt.Println("failed to heartbeat:", err)
		return err
	}

	go loopHeartbeat()
	return nil
}

func loopHeartbeat() {
	interval := time.Duration(config.C.Heartbeat.Interval) * time.Millisecond
	for {
		time.Sleep(interval)
		if err := heartbeat(); err != nil {
			logger.Warning(err)
		}
	}
}

func heartbeat() error {
	var clusters []string
	var err error
	if config.C.ReaderFrom == "config" {
		// 在配置文件维护实例和集群的对应关系
		for i := 0; i < len(config.C.Readers); i++ {
			clusters = append(clusters, config.C.Readers[i].ClusterName)
			err := models.AlertingEngineHeartbeatWithCluster(config.C.Heartbeat.Endpoint, config.C.Readers[i].ClusterName)
			if err != nil {
				logger.Warningf("heartbeat with cluster %s err:%v", config.C.Readers[i].ClusterName, err)
				continue
			}
		}
	} else {
		// 在页面上维护实例和集群的对应关系
		clusters, err = models.AlertingEngineGetClusters(config.C.Heartbeat.Endpoint)
		if err != nil {
			return err
		}
		if len(clusters) == 0 {
			// 告警引擎页面还没有配置集群，先上报一个空的集群，让 n9e-server 实例在页面上显示出来
			err := models.AlertingEngineHeartbeatWithCluster(config.C.Heartbeat.Endpoint, "")
			if err != nil {
				logger.Warningf("heartbeat with cluster %s err:%v", "", err)
			}
			logger.Warningf("heartbeat %s no cluster", config.C.Heartbeat.Endpoint)
		}

		err := models.AlertingEngineHeartbeat(config.C.Heartbeat.Endpoint)
		if err != nil {
			return err
		}
	}

	for i := 0; i < len(clusters); i++ {
		servers, err := ActiveServers(clusters[i])
		if err != nil {
			logger.Warningf("hearbeat %s get active server err:", clusters[i], err)
			continue
		}

		sort.Strings(servers)
		newss := strings.Join(servers, " ")

		oldss, exists := localss[clusters[i]]
		if exists && oldss == newss {
			continue
		}

		RebuildConsistentHashRing(clusters[i], servers)
		localss[clusters[i]] = newss
	}

	return nil
}

func ActiveServers(cluster string) ([]string, error) {
	if cluster == "" {
		return nil, fmt.Errorf("cluster is empty")
	}

	// 30秒内有心跳，就认为是活的
	return models.AlertingEngineGetsInstances("cluster = ? and clock > ?", cluster, time.Now().Unix()-30)
}
