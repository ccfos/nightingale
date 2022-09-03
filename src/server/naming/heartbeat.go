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
var localss string

func Heartbeat(ctx context.Context) error {
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
	cluster := ""
	if config.C.ReaderFrom == "config" {
		cluster = config.C.ClusterName
	}

	err := models.AlertingEngineHeartbeat(config.C.Heartbeat.Endpoint, cluster)
	if err != nil {
		return err
	}

	servers, err := ActiveServers()
	if err != nil {
		return err
	}

	sort.Strings(servers)
	newss := strings.Join(servers, " ")
	if newss != localss {
		RebuildConsistentHashRing(servers)
		localss = newss
	}

	return nil
}

func ActiveServers() ([]string, error) {
	cluster, err := models.AlertingEngineGetCluster(config.C.Heartbeat.Endpoint)
	if err != nil {
		return nil, err
	}

	// 30秒内有心跳，就认为是活的
	return models.AlertingEngineGetsInstances("cluster = ? and clock > ?", cluster, time.Now().Unix()-30)
}
