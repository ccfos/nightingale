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

type UserCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	users map[int64]*models.User // key: id
}

func NewUserCache(ctx *ctx.Context, stats *Stats) *UserCacheType {
	uc := &UserCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		users:           make(map[int64]*models.User),
	}
	uc.SyncUsers()
	return uc
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

func (uc *UserCacheType) GetByUsername(name string) *models.User {
	uc.RLock()
	defer uc.RUnlock()
	for _, v := range uc.users {
		if v.Username == name {
			return v
		}
	}
	return nil
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

func (uc *UserCacheType) SyncUsers() {
	err := uc.syncUsers()
	if err != nil {
		fmt.Println("failed to sync users:", err)
		exit(1)
	}

	go uc.loopSyncUsers()
}

func (uc *UserCacheType) loopSyncUsers() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := uc.syncUsers(); err != nil {
			logger.Warning("failed to sync users:", err)
		}
	}
}

func (uc *UserCacheType) syncUsers() error {
	start := time.Now()

	stat, err := models.UserStatistics(uc.ctx)
	if err != nil {
		dumper.PutSyncRecord("users", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec UserStatistics")
	}

	if !uc.StatChanged(stat.Total, stat.LastUpdated) {
		uc.stats.GaugeCronDuration.WithLabelValues("sync_users").Set(0)
		uc.stats.GaugeSyncNumber.WithLabelValues("sync_users").Set(0)
		dumper.PutSyncRecord("users", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.UserGetAll(uc.ctx)
	if err != nil {
		dumper.PutSyncRecord("users", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec UserGetAll")
	}

	m := make(map[int64]*models.User)
	for i := 0; i < len(lst); i++ {
		m[lst[i].Id] = lst[i]
	}

	uc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	uc.stats.GaugeCronDuration.WithLabelValues("sync_users").Set(float64(ms))
	uc.stats.GaugeSyncNumber.WithLabelValues("sync_users").Set(float64(len(m)))

	logger.Infof("timer: sync users done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("users", start.Unix(), ms, len(m), "success")

	return nil
}
