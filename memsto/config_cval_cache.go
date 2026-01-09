package memsto

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type CvalCache struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	mu    sync.RWMutex
	cvals map[string]string
}

func NewCvalCache(ctx *ctx.Context, stats *Stats) *CvalCache {
	cvalCache := &CvalCache{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		cvals:           make(map[string]string),
	}
	cvalCache.initSyncConfigs()
	return cvalCache
}

func (c *CvalCache) initSyncConfigs() {
	err := c.syncConfigs()
	if err != nil {
		log.Fatalln("failed to sync configs:", err)
	}

	err = models.RefreshPhoneEncryptionCache(c.ctx)
	if err != nil {
		logger.Errorf("failed to refresh phone encryption cache: %v", err)
	}

	go c.loopSyncConfigs()
}

func (c *CvalCache) loopSyncConfigs() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := c.syncConfigs(); err != nil {
			logger.Warning("failed to sync configs:", err)
		}
	}
}

func (c *CvalCache) syncConfigs() error {
	start := time.Now()

	stat, err := models.ConfigCvalStatistics(c.ctx)
	if err != nil {
		dumper.PutSyncRecord("cvals", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to call ConfigCvalStatistics")
	}

	if !c.statChanged(stat.Total, stat.LastUpdated) {
		c.stats.GaugeCronDuration.WithLabelValues("sync_cvals").Set(0)
		c.stats.GaugeSyncNumber.WithLabelValues("sync_cvals").Set(0)
		dumper.PutSyncRecord("cvals", start.Unix(), -1, -1, "not changed")
		return nil
	}

	cvals, err := models.ConfigsGetAll(c.ctx)
	if err != nil {
		dumper.PutSyncRecord("cvals", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call ConfigsGet")
	}

	c.Set(cvals, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	c.stats.GaugeCronDuration.WithLabelValues("sync_cvals").Set(float64(ms))
	c.stats.GaugeSyncNumber.WithLabelValues("sync_cvals").Set(float64(len(c.cvals)))
	dumper.PutSyncRecord("cvals", start.Unix(), ms, len(c.cvals), "success")

	return nil
}

func (c *CvalCache) statChanged(total int64, updated int64) bool {
	if c.statTotal == total && c.statLastUpdated == updated {
		return false
	}
	return true
}

func (c *CvalCache) Set(cvals []*models.Configs, total int64, updated int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statTotal = total
	c.statLastUpdated = updated
	for _, cfg := range cvals {
		c.cvals[cfg.Ckey] = cfg.Cval
	}
}

func (c *CvalCache) Get(ckey string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cvals[ckey]
}

func (c *CvalCache) GetLastUpdateTime() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statLastUpdated
}

type SiteInfo struct {
	PrintBodyPaths []string `json:"print_body_paths"`
	PrintAccessLog bool     `json:"print_access_log"`
	SiteUrl        string   `json:"site_url"`
	ReportHostNIC  bool     `json:"report_host_nic"`
}

func (c *CvalCache) GetSiteInfo() *SiteInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	si := SiteInfo{}
	if siteInfoStr := c.Get("site_info"); siteInfoStr != "" {
		if err := json.Unmarshal([]byte(siteInfoStr), &si); err != nil {
			logger.Errorf("Failed to unmarshal site info: %v", err)
		}
	}
	return &si
}

func (c *CvalCache) PrintBodyPaths() map[string]struct{} {
	printBodyPaths := c.GetSiteInfo().PrintBodyPaths
	pbp := make(map[string]struct{}, len(printBodyPaths))
	for _, p := range printBodyPaths {
		pbp[p] = struct{}{}
	}
	return pbp
}

func (c *CvalCache) PrintAccessLog() bool {
	return c.GetSiteInfo().PrintAccessLog
}
