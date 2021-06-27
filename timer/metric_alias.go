package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"
	"github.com/toolkits/pkg/logger"
)

func SyncMetricDesc() {
	if err := syncMetricDesc(); err != nil {
		fmt.Println("timer: sync metric desc fail:", err)
		exit(1)
	}

	go loopSyncMetricDesc()
}

func loopSyncMetricDesc() {
	randtime := rand.Intn(30000)
	fmt.Printf("timer: sync metric desc: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	for {
		time.Sleep(time.Second * time.Duration(30))
		if err := syncMetricDesc(); err != nil {
			logger.Warning("timer: sync metric desc fail:", err)
		}
	}
}

func syncMetricDesc() error {
	start := time.Now()

	metricDescs, err := models.MetricDescriptionGetAll()
	if err != nil {
		logger.Error("MetricDescriptionGetAll err:", err)
		return err
	}

	metricDescMap := make(map[string]interface{})
	for _, m := range metricDescs {
		metricDescMap[m.Metric] = m.Description
	}

	cache.MetricDescMapper.Clear()
	cache.MetricDescMapper.MSet(metricDescMap)
	logger.Debugf("timer: sync metric desc done, cost: %dms", time.Since(start).Milliseconds())
	return nil
}
