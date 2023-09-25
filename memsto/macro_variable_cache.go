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

type MacroVariableCache struct {
	statTotal  int64
	ctx        *ctx.Context
	stats      *Stats
	privateKey []byte
	passWord   string

	mu       sync.RWMutex
	macroMap map[string]string
}

const MacroVariableKey = "macroVariableCache"

// NewMacroVariableCache creates a new MacroVariableCache instance.
//
// Usage:
//   mvCache := ctx.Value(memsto.MacroVariableKey).(*memsto.MacroVariableCache)
//   if mvCache != nil {
//     get := mvCache.Get()
//     fmt.Printf("MacroVariable: %+v", get)
//   }
//
// It takes in the following parameters:
//   - ctx: a context.Context used for database queries and other operations.
//   - status: a Stats instance used to record metrics.
//   - privateKey: the private key used for decrypting macro variables.
//   - passWord: the password used along with the private key for decryption.
//
// It initializes the macroMap field to an empty map.
// It calls initSyncMacroVariables() to perform the initial sync of macro variables.
// It adds the MacroVariableCache instance to the context for later retrieval.
// It returns a pointer to the initialized MacroVariableCache.
func NewMacroVariableCache(ctx *ctx.Context, status *Stats, privateKey []byte, passWord string) *MacroVariableCache {
	mvc := &MacroVariableCache{
		statTotal:  -1,
		ctx:        ctx,
		stats:      status,
		privateKey: privateKey,
		passWord:   passWord,
		macroMap:   make(map[string]string),
	}
	mvc.initSyncMacroVariables()
	ctx.Ctx = context.WithValue(ctx.Ctx, MacroVariableKey, mvc) //add pointer to context
	return mvc
}

func (m *MacroVariableCache) initSyncMacroVariables() {

	err := m.syncMacroVariables()
	if err != nil {
		log.Fatalln("failed to sync macroVariables:", err)
	}

	go m.loopSyncMacroVariables()
}

func (m *MacroVariableCache) loopSyncMacroVariables() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := m.syncMacroVariables(); err != nil {
			logger.Warning("failed to sync macroVariables:", err)
		}
	}
}

func (m *MacroVariableCache) syncMacroVariables() interface{} {
	start := time.Now()

	stat, err := models.ConfigsUserVariableStatistics(m.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_variables", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to call macroVariables")
	}

	if !m.statChanged(stat.Total) {
		m.stats.GaugeCronDuration.WithLabelValues("sync_user_variables").Set(0)
		m.stats.GaugeSyncNumber.WithLabelValues("sync_user_variables").Set(0)
		dumper.PutSyncRecord("user_variables", start.Unix(), -1, -1, "not changed")
		return nil
	}

	decryptMap, decryptErr := models.MacroVariableGetDecryptMap(m.ctx, m.privateKey, m.passWord)
	if decryptErr != nil {
		dumper.PutSyncRecord("user_variables", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call MacroVariableGetDecryptMap")
	}

	m.Set(decryptMap, stat.Total)

	ms := time.Since(start).Milliseconds()
	m.stats.GaugeCronDuration.WithLabelValues("sync_user_variables").Set(float64(ms))
	m.stats.GaugeSyncNumber.WithLabelValues("sync_user_variables").Set(float64(len(decryptMap)))

	logger.Infof("timer: sync user_variables done, cost: %dms, number: %d", ms, len(decryptMap))
	dumper.PutSyncRecord("user_variables", start.Unix(), ms, len(decryptMap), "success")

	return nil
}

func (m *MacroVariableCache) statChanged(total int64) bool {
	if m.statTotal == total {
		return false
	}
	return true
}

func (m *MacroVariableCache) Set(decryptMap map[string]string, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.macroMap = decryptMap
	m.statTotal = total
}

func (m *MacroVariableCache) Get() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	resMap := make(map[string]string, len(m.macroMap))
	for k, v := range m.macroMap {
		resMap[k] = v
	}
	return resMap
}
