package memsto

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

// 1. append note to alert_event
// 2. append tags to series
type TargetCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats
	redis           storage.Redis

	sync.RWMutex
	targets      map[string]*models.Target // key: ident
	targetsIndex map[string][]string       // key: ip, value: ident list
}

func NewTargetCache(ctx *ctx.Context, stats *Stats, redis storage.Redis) *TargetCacheType {
	tc := &TargetCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		redis:           redis,
		targets:         make(map[string]*models.Target),
		targetsIndex:    make(map[string][]string),
	}

	tc.SyncTargets()
	return tc
}

func (tc *TargetCacheType) Reset() {
	tc.Lock()
	defer tc.Unlock()

	tc.statTotal = -1
	tc.statLastUpdated = -1
	tc.targets = make(map[string]*models.Target)
	tc.targetsIndex = make(map[string][]string)
}

func (tc *TargetCacheType) StatChanged(total, lastUpdated int64) bool {
	if tc.statTotal == total && tc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (tc *TargetCacheType) Set(m map[string]*models.Target, total, lastUpdated int64) {
	idx := make(map[string][]string, len(m))
	for ident, target := range m {
		if _, ok := idx[target.HostIp]; !ok {
			idx[target.HostIp] = []string{}
		}
		idx[target.HostIp] = append(idx[target.HostIp], ident)
	}

	tc.Lock()
	tc.targets = m
	tc.targetsIndex = idx
	tc.Unlock()

	// only one goroutine used, so no need lock
	tc.statTotal = total
	tc.statLastUpdated = lastUpdated
}

func (tc *TargetCacheType) Get(ident string) (*models.Target, bool) {
	tc.RLock()
	defer tc.RUnlock()
	val, has := tc.targets[ident]
	return val, has
}

func (tc *TargetCacheType) GetByIp(ip string) ([]*models.Target, bool) {
	tc.RLock()
	defer tc.RUnlock()
	idents, has := tc.targetsIndex[ip]
	if !has {
		return nil, false
	}
	targs := make([]*models.Target, 0, len(idents))
	for _, ident := range idents {
		if val, has := tc.targets[ident]; has {
			targs = append(targs, val)
		}
	}
	return targs, len(targs) > 0
}

func (tc *TargetCacheType) GetAll() []*models.Target {
	tc.RLock()
	defer tc.RUnlock()
	lst := make([]*models.Target, 0, len(tc.targets))
	for _, target := range tc.targets {
		lst = append(lst, target)
	}
	return lst
}

// GetAllBeatTime 返回所有 target 的心跳时间 map，key 为 ident，value 为 BeatTime
func (tc *TargetCacheType) GetAllBeatTime() map[string]int64 {
	tc.RLock()
	defer tc.RUnlock()
	beatTimeMap := make(map[string]int64, len(tc.targets))
	for ident, target := range tc.targets {
		beatTimeMap[ident] = target.BeatTime
	}
	return beatTimeMap
}

// refreshBeatTime 从 Redis 刷新缓存中所有 target 的 BeatTime
func (tc *TargetCacheType) refreshBeatTime() {
	if tc.redis == nil {
		return
	}

	// 快照 ident 列表，避免持锁访问 Redis
	tc.RLock()
	idents := make([]string, 0, len(tc.targets))
	for ident := range tc.targets {
		idents = append(idents, ident)
	}
	tc.RUnlock()

	if len(idents) == 0 {
		return
	}

	beatTimes := models.FetchBeatTimesFromRedis(tc.redis, idents)
	if len(beatTimes) == 0 {
		return
	}

	tc.Lock()
	for ident, ts := range beatTimes {
		if target, ok := tc.targets[ident]; ok {
			target.BeatTime = ts
		}
	}
	tc.Unlock()
}

func (tc *TargetCacheType) Gets(idents []string) []*models.Target {
	tc.RLock()
	defer tc.RUnlock()
	var targets []*models.Target
	for _, ident := range idents {
		if target, has := tc.targets[ident]; has {
			targets = append(targets, target)
		}
	}
	return targets
}

func (tc *TargetCacheType) GetOffsetHost(targets []*models.Target, now, offset int64) map[string]int64 {
	tc.RLock()
	defer tc.RUnlock()
	hostOffset := make(map[string]int64)
	for _, target := range targets {
		target, exists := tc.targets[target.Ident]
		if !exists {
			continue
		}

		if target.CpuNum <= 0 {
			// means this target is not collect by categraf, do not check offset
			continue
		}

		if now-target.BeatTime > 120 {
			// means this target is not a active host, do not check offset
			continue
		}

		if int64(math.Abs(float64(target.Offset))) > offset {
			hostOffset[target.Ident] = target.Offset
		}
	}

	return hostOffset
}

func (tc *TargetCacheType) SyncTargets() {
	err := tc.syncTargets()
	if err != nil {
		log.Fatalln("failed to sync targets:", err)
	}

	go tc.loopSyncTargets()
}

func (tc *TargetCacheType) loopSyncTargets() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := tc.syncTargets(); err != nil {
			logger.Warning("failed to sync targets:", err)
		}
	}
}

func (tc *TargetCacheType) syncTargets() error {
	start := time.Now()

	stat, err := models.TargetStatistics(tc.ctx)
	if err != nil {
		dumper.PutSyncRecord("targets", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to call TargetStatistics")
	}

	if !tc.StatChanged(stat.Total, stat.LastUpdated) {
		tc.refreshBeatTime()
		tc.stats.GaugeCronDuration.WithLabelValues("sync_targets").Set(0)
		tc.stats.GaugeSyncNumber.WithLabelValues("sync_targets").Set(0)
		dumper.PutSyncRecord("targets", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.TargetGetsAll(tc.ctx)
	if err != nil {
		dumper.PutSyncRecord("targets", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to call TargetGetsAll")
	}

	m := make(map[string]*models.Target)

	metaMap := tc.GetHostMetas(lst)
	if len(metaMap) > 0 {
		for i := 0; i < len(lst); i++ {
			if meta, ok := metaMap[lst[i].Ident]; ok {
				lst[i].FillMeta(meta)
			}
		}
	}

	// 从 Redis 批量获取心跳时间填充 BeatTime
	models.FillTargetsBeatTime(tc.redis, lst)

	for i := 0; i < len(lst); i++ {
		m[lst[i].Ident] = lst[i]
	}

	tc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	tc.stats.GaugeCronDuration.WithLabelValues("sync_targets").Set(float64(ms))
	tc.stats.GaugeSyncNumber.WithLabelValues("sync_targets").Set(float64(len(lst)))
	dumper.PutSyncRecord("targets", start.Unix(), ms, len(lst), "success")

	return nil
}

// get host update time
func (tc *TargetCacheType) GetHostUpdateTime(targets []string) map[string]int64 {
	if tc.redis == nil {
		return make(map[string]int64)
	}

	metaMap := models.FetchBeatTimesFromRedis(tc.redis, targets)

	for _, ident := range targets {
		if _, ok := metaMap[ident]; !ok {
			// if not exists, get from cache
			target, exists := tc.Get(ident)
			if exists {
				metaMap[ident] = target.BeatTime
			}
		}
	}

	return metaMap
}

func (tc *TargetCacheType) GetHostMetas(targets []*models.Target) map[string]*models.HostMeta {
	metaMap := make(map[string]*models.HostMeta)
	if tc.redis == nil {
		return metaMap
	}
	var metas []*models.HostMeta
	num := 0
	var keys []string
	for i := 0; i < len(targets); i++ {
		keys = append(keys, models.WrapIdent(targets[i].Ident))
		num++
		if num == 100 {
			vals := storage.MGet(context.Background(), tc.redis, keys)
			for _, value := range vals {
				var meta models.HostMeta
				if value == nil {
					continue
				}

				err := json.Unmarshal(value, &meta)
				if err != nil {
					logger.Errorf("failed to unmarshal host meta: %s value:%v", err, value)
					continue
				}
				metaMap[meta.Hostname] = &meta
			}
			keys = keys[:0]
			metas = metas[:0]
			num = 0
		}
	}

	vals := storage.MGet(context.Background(), tc.redis, keys)
	for _, value := range vals {
		var meta models.HostMeta
		if value == nil {
			continue
		}

		err := json.Unmarshal(value, &meta)
		if err != nil {
			continue
		}
		metaMap[meta.Hostname] = &meta
	}

	return metaMap
}
