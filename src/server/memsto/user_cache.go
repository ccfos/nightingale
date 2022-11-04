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

type UserCacheType struct {
	statTotal       int64
	statLastUpdated int64

	sync.RWMutex
	users map[int64]*models.User // key: id
}

var UserCache = UserCacheType{
	statTotal:       -1,
	statLastUpdated: -1,
	users:           make(map[int64]*models.User),
}

func (uc *UserCacheType) StatChanged(total, lastUpdated int64) bool {
	if uc.statTotal == total && uc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (uc *UserCacheType) Set(m map[int64]*models.User, total, lastUpdated int64) {
	uc.Lock()
	uc.users = m
	uc.Unlock()

	// only one goroutine used, so no need lock
	uc.statTotal = total
	uc.statLastUpdated = lastUpdated
}

func (uc *UserCacheType) GetByUserId(id int64) *models.User {
	uc.RLock()
	defer uc.RUnlock()
	return uc.users[id]
}

func (uc *UserCacheType) GetByUserIds(ids []int64) []*models.User {
	set := make(map[int64]struct{})

	uc.RLock()
	defer uc.RUnlock()

	var users []*models.User
	for _, id := range ids {
		if uc.users[id] == nil {
			continue
		}

		if _, has := set[id]; has {
			continue
		}

		users = append(users, uc.users[id])
		set[id] = struct{}{}
	}

	if users == nil {
		users = []*models.User{}
	}

	return users
}

func (uc *UserCacheType) GetMaintainerUsers() []*models.User {
	uc.RLock()
	defer uc.RUnlock()

	var users []*models.User
	for _, v := range uc.users {
		if v.Maintainer == 1 {
			users = append(users, v)
		}
	}

	if users == nil {
		users = []*models.User{}
	}

	return users
}

func SyncUsers() {
	err := syncUsers()
	if err != nil {
		fmt.Println("failed to sync users:", err)
		exit(1)
	}

	go loopSyncUsers()
}

func loopSyncUsers() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := syncUsers(); err != nil {
			logger.Warning("failed to sync users:", err)
		}
	}
}

func syncUsers() error {
	start := time.Now()

	stat, err := models.UserStatistics()
	if err != nil {
		return errors.WithMessage(err, "failed to exec UserStatistics")
	}

	clusterName := config.ReaderClient.GetClusterName()

	if !UserCache.StatChanged(stat.Total, stat.LastUpdated) {
		if clusterName != "" {
			promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_users").Set(0)
			promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_users").Set(0)
		}

		logger.Debug("users not changed")
		return nil
	}

	lst, err := models.UserGetAll()
	if err != nil {
		return errors.WithMessage(err, "failed to exec UserGetAll")
	}

	m := make(map[int64]*models.User)
	for i := 0; i < len(lst); i++ {
		m[lst[i].Id] = lst[i]
	}

	UserCache.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	if clusterName != "" {
		promstat.GaugeCronDuration.WithLabelValues(clusterName, "sync_users").Set(float64(ms))
		promstat.GaugeSyncNumber.WithLabelValues(clusterName, "sync_users").Set(float64(len(m)))
	}

	logger.Infof("timer: sync users done, cost: %dms, number: %d", ms, len(m))

	return nil
}
