package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type UserGroupCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	ugs map[int64]*models.UserGroup // key: id
}

func NewUserGroupCache(ctx *ctx.Context, stats *Stats) *UserGroupCacheType {
	ugc := &UserGroupCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		ugs:             make(map[int64]*models.UserGroup),
	}
	ugc.SyncUserGroups()
	return ugc
}

func (ugc *UserGroupCacheType) StatChanged(total, lastUpdated int64) bool {
	if ugc.statTotal == total && ugc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (ugc *UserGroupCacheType) Set(ugs map[int64]*models.UserGroup, total, lastUpdated int64) {
	ugc.Lock()
	ugc.ugs = ugs
	ugc.Unlock()

	// only one goroutine used, so no need lock
	ugc.statTotal = total
	ugc.statLastUpdated = lastUpdated
}

func (ugc *UserGroupCacheType) GetByUserGroupId(id int64) *models.UserGroup {
	ugc.RLock()
	defer ugc.RUnlock()
	return ugc.ugs[id]
}

func (ugc *UserGroupCacheType) GetByUserGroupIds(ids []int64) []*models.UserGroup {
	set := make(map[int64]struct{})

	ugc.RLock()
	defer ugc.RUnlock()

	var ugs []*models.UserGroup
	for _, id := range ids {
		if ugc.ugs[id] == nil {
			continue
		}

		if _, has := set[id]; has {
			continue
		}

		ugs = append(ugs, ugc.ugs[id])
		set[id] = struct{}{}
	}

	if ugs == nil {
		return []*models.UserGroup{}
	}

	return ugs
}

func (ugc *UserGroupCacheType) SyncUserGroups() {
	err := ugc.syncUserGroups()
	if err != nil {
		fmt.Println("failed to sync user groups:", err)
		exit(1)
	}

	go ugc.loopSyncUserGroups()
}

func (ugc *UserGroupCacheType) loopSyncUserGroups() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := ugc.syncUserGroups(); err != nil {
			logger.Warning("failed to sync user groups:", err)
		}
	}
}

func (ugc *UserGroupCacheType) syncUserGroups() error {
	start := time.Now()

	stat, err := models.UserGroupStatistics(ugc.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_groups", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec UserGroupStatistics")
	}

	if !ugc.StatChanged(stat.Total, stat.LastUpdated) {
		ugc.stats.GaugeCronDuration.WithLabelValues("sync_user_groups").Set(0)
		ugc.stats.GaugeSyncNumber.WithLabelValues("sync_user_groups").Set(0)
		dumper.PutSyncRecord("user_groups", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.UserGroupGetAll(ugc.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_groups", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec UserGroupGetAll")
	}

	m := make(map[int64]*models.UserGroup)
	for i := 0; i < len(lst); i++ {
		m[lst[i].Id] = lst[i]
	}

	// fill user ids
	members, err := models.UserGroupMemberGetAll(ugc.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_groups", start.Unix(), -1, -1, "failed to query members: "+err.Error())
		return errors.WithMessage(err, "failed to exec UserGroupMemberGetAll")
	}

	for i := 0; i < len(members); i++ {
		ug, has := m[members[i].GroupId]
		if !has {
			continue
		}

		if ug == nil {
			continue
		}

		ug.UserIds = append(ug.UserIds, members[i].UserId)
	}

	ugc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	ugc.stats.GaugeCronDuration.WithLabelValues("sync_user_groups").Set(float64(ms))
	ugc.stats.GaugeSyncNumber.WithLabelValues("sync_user_groups").Set(float64(len(m)))

	logger.Infof("timer: sync user groups done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("user_groups", start.Unix(), ms, len(m), "success")

	return nil
}
