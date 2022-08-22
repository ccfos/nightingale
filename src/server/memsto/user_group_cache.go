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

type UserGroupCacheType struct {
	statTotal       int64
	statLastUpdated int64

	sync.RWMutex
	ugs map[int64]*models.UserGroup // key: id
}

var UserGroupCache = UserGroupCacheType{
	statTotal:       -1,
	statLastUpdated: -1,
	ugs:             make(map[int64]*models.UserGroup),
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

func SyncUserGroups() {
	err := syncUserGroups()
	if err != nil {
		fmt.Println("failed to sync user groups:", err)
		exit(1)
	}

	go loopSyncUserGroups()
}

func loopSyncUserGroups() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := syncUserGroups(); err != nil {
			logger.Warning("failed to sync user groups:", err)
		}
	}
}

func syncUserGroups() error {
	start := time.Now()

	stat, err := models.UserGroupStatistics()
	if err != nil {
		return errors.WithMessage(err, "failed to exec UserGroupStatistics")
	}

	clusterName := config.ReaderClient.GetClusterName()

	if !UserGroupCache.StatChanged(stat.Total, stat.LastUpdated) {
		if clusterName != "" {
			promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_user_groups").Set(0)
			promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_user_groups").Set(0)
		}

		logger.Debug("user_group not changed")
		return nil
	}

	lst, err := models.UserGroupGetAll()
	if err != nil {
		return errors.WithMessage(err, "failed to exec UserGroupGetAll")
	}

	m := make(map[int64]*models.UserGroup)
	for i := 0; i < len(lst); i++ {
		m[lst[i].Id] = lst[i]
	}

	// fill user ids
	members, err := models.UserGroupMemberGetAll()
	if err != nil {
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

	UserGroupCache.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	if clusterName != "" {
		promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_user_groups").Set(float64(ms))
		promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_user_groups").Set(float64(len(m)))
	}

	logger.Infof("timer: sync user groups done, cost: %dms, number: %d", ms, len(m))

	return nil
}
