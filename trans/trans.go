package trans

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/naming"
	"github.com/didi/nightingale/v5/pkg/ipool"

	"github.com/toolkits/pkg/logger"
)

var connPools *ipool.ConnPools
var svcsCache string

func Start(ctx context.Context) {
	// 初始化本包的数据结构，然后启动一个goroutine，周期性获取活着的judge实例，更新相应的pool、queue等
	judgeConf := config.Config.Judge
	connPools = ipool.NewConnPools(judgeConf.ConnMax, judgeConf.ConnIdle, judgeConf.ConnTimeout, judgeConf.CallTimeout, []string{})

	if err := syncInstances(); err != nil {
		fmt.Println("syncInstances fail:", err)
		logger.Close()
		os.Exit(1)
	}

	go loopSyncInstances()
}

func loopSyncInstances() {
	interval := time.Duration(config.Config.Heartbeat.Interval) * time.Millisecond
	for {
		time.Sleep(interval)
		if err := syncInstances(); err != nil {
			logger.Warning("syncInstances fail:", err)
		}
	}
}

func syncInstances() error {
	// 获取当前活着的所有实例
	instances, err := models.InstanceGetAlive(config.EndpointName)
	if err != nil {
		logger.Warningf("mysql.error: get alive server instances fail: %v", err)
		return err
	}

	// 排序，便于与内存中的实例列表做差别判断
	sort.Strings(instances)

	// 如果列表变化，就去处理，并且要更新内存变量serverStr
	newSvcs := strings.Join(instances, ",")
	if newSvcs != svcsCache {
		// 如果有新实例，创建对应的连接池，如果实例少了，删掉没用的连接池
		connPools.UpdatePools(instances)
		// 如果有新实例，创建对应的Queue，如果实例少了，删掉对应的Queue
		queues.Update(instances)
		// 重建哈希环
		naming.RebuildConsistentHashRing(instances)
		svcsCache = newSvcs
	}

	return nil
}
