package memsto

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"regexp"
	"strings"
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
	targets       map[string]*models.Target // key: ident
	groupToIdents map[int64][]string        // key: group_id
}

func NewTargetCache(ctx *ctx.Context, stats *Stats, redis storage.Redis) *TargetCacheType {
	tc := &TargetCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		redis:           redis,
		targets:         make(map[string]*models.Target),
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
}

func (tc *TargetCacheType) StatChanged(total, lastUpdated int64) bool {
	if tc.statTotal == total && tc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (tc *TargetCacheType) Set(m map[string]*models.Target, groupToIdents map[int64][]string, total, lastUpdated int64) {
	tc.Lock()
	tc.targets = m
	tc.groupToIdents = groupToIdents
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

		if now-target.UpdateAt > 120 {
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
	groupToIdents := make(map[int64][]string)

	metaMap := tc.GetHostMetas(lst)
	if len(metaMap) > 0 {
		for i := 0; i < len(lst); i++ {
			if meta, ok := metaMap[lst[i].Ident]; ok {
				lst[i].FillMeta(meta)
			}
		}
	}

	for i := 0; i < len(lst); i++ {
		m[lst[i].Ident] = lst[i]
		for _, groupID := range lst[i].GroupIds {
			groupToIdents[groupID] = append(groupToIdents[groupID], lst[i].Ident)
		}
	}

	tc.Set(m, groupToIdents, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	tc.stats.GaugeCronDuration.WithLabelValues("sync_targets").Set(float64(ms))
	tc.stats.GaugeSyncNumber.WithLabelValues("sync_targets").Set(float64(len(lst)))
	logger.Infof("timer: sync targets done, cost: %dms, number: %d", ms, len(lst))
	dumper.PutSyncRecord("targets", start.Unix(), ms, len(lst), "success")

	return nil
}

// get host update time
func (tc *TargetCacheType) GetHostUpdateTime(targets []string) map[string]int64 {
	metaMap := make(map[string]int64)
	if tc.redis == nil {
		return metaMap
	}

	num := 0
	var keys []string
	for i := 0; i < len(targets); i++ {
		keys = append(keys, models.WrapIdentUpdateTime(targets[i]))
		num++
		if num == 100 {
			vals := storage.MGet(context.Background(), tc.redis, keys)
			for _, value := range vals {
				var hostUpdateTime models.HostUpdteTime
				if value == nil {
					continue
				}

				err := json.Unmarshal(value, &hostUpdateTime)
				if err != nil {
					logger.Errorf("failed to unmarshal host meta: %s value:%v", err, value)
					continue
				}
				metaMap[hostUpdateTime.Ident] = hostUpdateTime.UpdateTime
			}
			keys = keys[:0]
			num = 0
		}
	}

	vals := storage.MGet(context.Background(), tc.redis, keys)
	for _, value := range vals {
		var hostUpdateTime models.HostUpdteTime
		if value == nil {
			continue
		}

		err := json.Unmarshal(value, &hostUpdateTime)
		if err != nil {
			continue
		}
		metaMap[hostUpdateTime.Ident] = hostUpdateTime.UpdateTime
	}

	for _, ident := range targets {
		if _, ok := metaMap[ident]; !ok {
			// if not exists, get from cache
			target, exists := tc.Get(ident)
			if exists {
				metaMap[ident] = target.UpdateAt
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

func (tc *TargetCacheType) getAllHostIdentsWithoutLock() []string {
	var idents []string
	for ident, _ := range tc.targets {
		idents = append(idents, ident)
	}
	return idents
}

func (tc *TargetCacheType) getHostIdentsByGroupIdsWithoutLock(groupIDs []int64) []string {
	var targetIdents []string
	for _, groupID := range groupIDs {
		if idents, has := tc.groupToIdents[groupID]; has {
			targetIdents = append(targetIdents, idents...)
		}
	}
	return targetIdents
}

func (tc *TargetCacheType) getHostIdentsExcludeGroupIdsWithoutLock(groupIDs []int64) []string {
	var targetIdents []string
	exclude := make(map[string]struct{})
	for _, id := range groupIDs {
		if idents, has := tc.groupToIdents[id]; has {
			for _, ident := range idents {
				exclude[ident] = struct{}{}
			}
		}
	}

	for ident, _ := range tc.targets {
		if _, ok := exclude[ident]; ok {
			continue
		}
		targetIdents = append(targetIdents, ident)
	}
	return targetIdents
}

func (tc *TargetCacheType) getHostsByIdentsWithoutLock(idents []string) []*models.Target {
	var targets []*models.Target
	for _, ident := range idents {
		if target, has := tc.targets[ident]; has {
			targets = append(targets, target)
		}
	}
	return targets
}

func (tc *TargetCacheType) getHostIdentsExcludeIdentsWithoutLock(idents []string) []string {
	var targetIdents []string
	exclude := make(map[string]struct{})
	for _, id := range idents {
		exclude[id] = struct{}{}
	}

	for ident, _ := range tc.targets {
		if _, ok := exclude[ident]; ok {
			continue
		}
		targetIdents = append(targetIdents, ident)
	}
	return targetIdents
}

func (tc *TargetCacheType) getHostIdentsMatchIdentsWithoutLock(identPatterns []string) []string {
	var targetIdents []string
	for ident, _ := range tc.targets {
		for _, identPattern := range identPatterns {
			// 模糊匹配转正则
			if ok, _ := regexp.Match(strings.Replace(identPattern, "*", ".*", -1), []byte(ident)); ok {
				targetIdents = append(targetIdents, ident)
				break
			}
		}
	}
	return targetIdents
}

func (tc *TargetCacheType) getHostIdentsByTagsWithoutLock(tags []string) []string {
	var targetIdents []string

	tagMap := make(map[string]struct{})
	for _, tag := range tags {
		tagMap[tag] = struct{}{}
	}

	for ident, target := range tc.targets {
		for _, tag := range target.TagsJSON {
			if _, ok := tagMap[tag]; ok {
				targetIdents = append(targetIdents, ident)
				break
			}
		}
	}
	return targetIdents
}

func (tc *TargetCacheType) getHostIdentsExcludeTagsWithoutLock(tags []string) []string {
	var targetIdents []string

	for ident, target := range tc.targets {
		exclude := false
		curTags := make(map[string]struct{})
		for _, tag := range target.TagsJSON {
			curTags[tag] = struct{}{}
		}
		for _, tag := range tags {
			if _, ok := curTags[tag]; ok {
				exclude = true
				break
			}
		}
		if !exclude {
			targetIdents = append(targetIdents, ident)
		}
	}
	return targetIdents
}

func (tc *TargetCacheType) getHostIdentsMatchExcludeIdentsWithoutLock(identPatterns []string) []string {
	var targetIdents []string
	exclude := make(map[string]struct{})
	for _, id := range identPatterns {
		exclude[id] = struct{}{}
	}

	for ident, _ := range tc.targets {
		has := false
		for _, identPattern := range identPatterns {
			if ok, _ := regexp.Match(strings.Replace(identPattern, "*", ".*", -1), []byte(ident)); ok {
				has = true
				break
			}
		}
		if !has {
			targetIdents = append(targetIdents, ident)
		}
	}
	return targetIdents
}

func (tc *TargetCacheType) GetHostIdentsQuery(queries []models.HostQuery) []*models.Target {
	tc.Lock()
	defer tc.Unlock()

	targetIdents := tc.getAllHostIdentsWithoutLock()

	for _, q := range queries {
		var cur []string
		switch q.Key {
		case "group_ids":
			ids := models.ParseInt64(q.Values)
			if q.Op == "==" {
				cur = tc.getHostIdentsByGroupIdsWithoutLock(ids)
			} else {
				cur = tc.getHostIdentsExcludeGroupIdsWithoutLock(ids)
			}
		case "tags":
			var tags []string
			for _, v := range q.Values {
				if v == nil {
					continue
				}
				tags = append(tags, v.(string))
			}
			if q.Op == "==" {
				cur = tc.getHostIdentsByTagsWithoutLock(tags)
			} else {
				cur = tc.getHostIdentsExcludeTagsWithoutLock(tags)
			}
		case "hosts":
			var idents []string
			for _, v := range q.Values {
				if v == nil {
					continue
				}
				idents = append(idents, v.(string))
			}
			if q.Op == "==" {
				cur = idents
			} else if q.Op == "!=" {
				cur = tc.getHostIdentsExcludeIdentsWithoutLock(idents)
			} else if q.Op == "=~" {
				cur = tc.getHostIdentsMatchIdentsWithoutLock(idents)
			} else if q.Op == "!~" {
				cur = tc.getHostIdentsMatchExcludeIdentsWithoutLock(idents)
			}
		default:
			// all_hosts 与其他未知条件不改变已有集合
			cur = targetIdents
		}
		targetIdents = intersection(targetIdents, cur)
	}
	return tc.getHostsByIdentsWithoutLock(targetIdents)
}

func intersection(a, b []string) []string {
	m := make(map[string]struct{})
	for _, v := range a {
		m[v] = struct{}{}
	}

	var c []string
	for _, v := range b {
		if _, ok := m[v]; ok {
			c = append(c, v)
		}
	}
	return c
}
