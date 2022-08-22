package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

type AlertSubscribeCacheType struct {
	statTotal       int64
	statLastUpdated int64

	sync.RWMutex
	subs map[int64][]*models.AlertSubscribe
}

var AlertSubscribeCache = AlertSubscribeCacheType{
	statTotal:       -1,
	statLastUpdated: -1,
	subs:            make(map[int64][]*models.AlertSubscribe),
}

func (c *AlertSubscribeCacheType) Reset() {
	c.Lock()
	defer c.Unlock()

	c.statTotal = -1
	c.statLastUpdated = -1
	c.subs = make(map[int64][]*models.AlertSubscribe)
}

func (c *AlertSubscribeCacheType) StatChanged(total, lastUpdated int64) bool {
	if c.statTotal == total && c.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (c *AlertSubscribeCacheType) Set(m map[int64][]*models.AlertSubscribe, total, lastUpdated int64) {
	c.Lock()
	c.subs = m
	c.Unlock()

	// only one goroutine used, so no need lock
	c.statTotal = total
	c.statLastUpdated = lastUpdated
}

func (c *AlertSubscribeCacheType) Get(ruleId int64) ([]*models.AlertSubscribe, bool) {
	c.RLock()
	defer c.RUnlock()

	lst, has := c.subs[ruleId]
	return lst, has
}

func (c *AlertSubscribeCacheType) GetStructs(ruleId int64) []models.AlertSubscribe {
	c.RLock()
	defer c.RUnlock()

	lst, has := c.subs[ruleId]
	if !has {
		return []models.AlertSubscribe{}
	}

	ret := make([]models.AlertSubscribe, len(lst))
	for i := 0; i < len(lst); i++ {
		ret[i] = *lst[i]
	}

	return ret
}

func SyncAlertSubscribes() {
	err := syncAlertSubscribes()
	if err != nil {
		fmt.Println("failed to sync alert subscribes:", err)
		exit(1)
	}

	go loopSyncAlertSubscribes()
}

func loopSyncAlertSubscribes() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := syncAlertSubscribes(); err != nil {
			logger.Warning("failed to sync alert subscribes:", err)
		}
	}
}

func syncAlertSubscribes() error {
	start := time.Now()

	clusterName := config.ReaderClient.GetClusterName()
	if clusterName == "" {
		AlertSubscribeCache.Reset()
		logger.Warning("cluster name is blank")
		return nil
	}

	stat, err := models.AlertSubscribeStatistics(clusterName)
	if err != nil {
		return errors.WithMessage(err, "failed to exec AlertSubscribeStatistics")
	}

	if !AlertSubscribeCache.StatChanged(stat.Total, stat.LastUpdated) {
		promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_alert_subscribes").Set(0)
		promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_alert_subscribes").Set(0)
		logger.Debug("alert subscribes not changed")
		return nil
	}

	lst, err := models.AlertSubscribeGetsByCluster(clusterName)
	if err != nil {
		return errors.WithMessage(err, "failed to exec AlertSubscribeGetsByCluster")
	}

	subs := make(map[int64][]*models.AlertSubscribe)

	for i := 0; i < len(lst); i++ {
		err = lst[i].Parse()
		if err != nil {
			logger.Warningf("failed to parse alert subscribe, id: %d", lst[i].Id)
			continue
		}

		subs[lst[i].RuleId] = append(subs[lst[i].RuleId], lst[i])
	}

	AlertSubscribeCache.Set(subs, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_alert_subscribes").Set(float64(ms))
	promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_alert_subscribes").Set(float64(len(lst)))
	logger.Infof("timer: sync subscribes done, cost: %dms, number: %d", ms, len(lst))

	return nil
}
