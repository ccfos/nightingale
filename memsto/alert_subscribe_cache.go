package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type AlertSubscribeCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	subs map[int64][]*models.AlertSubscribe
}

func NewAlertSubscribeCache(ctx *ctx.Context, stats *Stats) *AlertSubscribeCacheType {
	asc := &AlertSubscribeCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		subs:            make(map[int64][]*models.AlertSubscribe),
	}
	asc.SyncAlertSubscribes()
	return asc
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

func (c *AlertSubscribeCacheType) SyncAlertSubscribes() {
	err := c.syncAlertSubscribes()
	if err != nil {
		fmt.Println("failed to sync alert subscribes:", err)
		exit(1)
	}

	go c.loopSyncAlertSubscribes()
}

func (c *AlertSubscribeCacheType) loopSyncAlertSubscribes() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := c.syncAlertSubscribes(); err != nil {
			logger.Warning("failed to sync alert subscribes:", err)
		}
	}
}

func (c *AlertSubscribeCacheType) syncAlertSubscribes() error {
	start := time.Now()
	stat, err := models.AlertSubscribeStatistics(c.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to exec AlertSubscribeStatistics")
	}

	if !c.StatChanged(stat.Total, stat.LastUpdated) {
		c.stats.GaugeCronDuration.WithLabelValues("sync_alert_subscribes").Set(0)
		c.stats.GaugeSyncNumber.WithLabelValues("sync_alert_subscribes").Set(0)
		logger.Debug("alert subscribes not changed")
		return nil
	}

	lst, err := models.AlertSubscribeGetsAll(c.ctx)
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

		err = lst[i].DB2FE()
		if err != nil {
			logger.Warningf("failed to db2fe alert subscribe, id: %d", lst[i].Id)
			continue
		}

		err = lst[i].FillDatasourceIds(c.ctx)
		if err != nil {
			logger.Warningf("failed to fill datasource ids, id: %d", lst[i].Id)
			continue
		}

		subs[lst[i].RuleId] = append(subs[lst[i].RuleId], lst[i])
	}

	c.Set(subs, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	c.stats.GaugeCronDuration.WithLabelValues("sync_alert_subscribes").Set(float64(ms))
	c.stats.GaugeSyncNumber.WithLabelValues("sync_alert_subscribes").Set(float64(len(lst)))
	logger.Infof("timer: sync subscribes done, cost: %dms, number: %d", ms, len(lst))

	return nil
}
