package memsto

import (
	"context"
	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"log"
	"sync"
	"time"
)

type ConfigCache struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats
	privateKey      []byte
	passWord        string

	mu              sync.RWMutex
	userVariableMap map[string]string
}

const ConfigUserVariableKey = "userVariableCache"

// NewConfigsCache creates a new ConfigCache instance.
//
// Usage:
//   uvCache := ctx.Value(memsto.ConfigUserVariableKey).(*memsto.ConfigCache)
//   if uvCache != nil {
//     get := uvCache.Get()
//     fmt.Printf("UserVariable: %+v", get)
//   }
//
// It takes in the following parameters:
//   - ctx: a context.Context used for database queries and other operations.
//   - status: a Stats instance used to record metrics.
//   - privateKey: the private key used for decrypting user variables.
//   - passWord: the password used along with the private key for decryption.
//
// It initializes the userVariableMap field to an empty map.
// It calls initSyncConfigs() to perform the initial sync of user variables.
// It adds the ConfigCache instance to the context for later retrieval.
// It returns a pointer to the initialized ConfigCache.
func NewConfigsCache(ctx *ctx.Context, status *Stats, privateKey []byte, passWord string) *ConfigCache {
	configCache := &ConfigCache{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           status,
		privateKey:      privateKey,
		passWord:        passWord,
		userVariableMap: make(map[string]string),
	}
	configCache.initSyncConfigs()
	ctx.Ctx = context.WithValue(ctx.Ctx, ConfigUserVariableKey, configCache) //add pointer to context
	return configCache
}

func (c *ConfigCache) initSyncConfigs() {

	err := c.syncConfigs()
	if err != nil {
		log.Fatalln("failed to sync configs:", err)
	}

	go c.loopSyncConfigs()
}

func (c *ConfigCache) loopSyncConfigs() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := c.syncConfigs(); err != nil {
			logger.Warning("failed to sync configs:", err)
		}
	}
}

func (c *ConfigCache) syncConfigs() error {
	start := time.Now()

	stat, err := models.ConfigsUserVariableStatistics(c.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_variables", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to call userVariables")
	}

	if !c.statChanged(stat.Total, stat.LastUpdated) {
		c.stats.GaugeCronDuration.WithLabelValues("sync_user_variables").Set(0)
		c.stats.GaugeSyncNumber.WithLabelValues("sync_user_variables").Set(0)
		dumper.PutSyncRecord("user_variables", start.Unix(), -1, -1, "not changed")
		return nil
	}

	decryptMap, decryptErr := models.ConfigUserVariableGetDecryptMap(c.ctx, c.privateKey, c.passWord)
	if decryptErr != nil {
		dumper.PutSyncRecord("user_variables", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call ConfigUserVariableGetDecryptMap")
	}

	c.Set(decryptMap, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	c.stats.GaugeCronDuration.WithLabelValues("sync_user_variables").Set(float64(ms))
	c.stats.GaugeSyncNumber.WithLabelValues("sync_user_variables").Set(float64(len(decryptMap)))

	logger.Infof("timer: sync user_variables done, cost: %dms, number: %d", ms, len(decryptMap))
	dumper.PutSyncRecord("user_variables", start.Unix(), ms, len(decryptMap), "success")

	return nil
}

func (c *ConfigCache) statChanged(total int64, updated int64) bool {
	if c.statTotal == total {
		return false
	}
	return true
}

func (c *ConfigCache) Set(decryptMap map[string]string, total int64, updated int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userVariableMap = decryptMap
	c.statTotal = total
	c.statLastUpdated = updated
}

func (c *ConfigCache) Get() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	resMap := make(map[string]string, len(c.userVariableMap))
	for k, v := range c.userVariableMap {
		resMap[k] = v
	}
	return resMap
}
