package memsto

import (
	"fmt"

	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/flashduty"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type UserCacheType struct {
	statTotal          int64
	statLastUpdated    int64
	configsTotal       int64
	configsLastUpdated int64
	ctx                *ctx.Context
	stats              *Stats

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

func (uc *UserCacheType) StatChanged(total, lastUpdated, configsTotal, configsLastUpdated int64) bool {
	if uc.statTotal == total && uc.statLastUpdated == lastUpdated && uc.configsTotal == configsTotal && uc.configsLastUpdated == configsLastUpdated {
		return false
	}

	return true
}

func (uc *UserCacheType) Set(m map[int64]*models.User, total, lastUpdated, configsTotal, configsLastUpdated int64) {
	uc.Lock()
	uc.users = m
	uc.Unlock()

	// only one goroutine used, so no need lock
	uc.statTotal = total
	uc.statLastUpdated = lastUpdated
	uc.configsTotal = configsTotal
	uc.configsLastUpdated = configsLastUpdated
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
	go uc.loopUpdateLastActiveTime()
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

	configsStat, err := models.ConfigsUserVariableStatistics(uc.ctx)
	if err != nil {
		dumper.PutSyncRecord("user_variables", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec ConfigsUserVariableStatistics")
	}

	if !uc.StatChanged(stat.Total, stat.LastUpdated, configsStat.Total, configsStat.LastUpdated) {
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
	uc.Set(m, stat.Total, stat.LastUpdated, configsStat.Total, configsStat.LastUpdated)

	if flashduty.NeedSyncUser(uc.ctx) {
		go func() {
			err := flashduty.SyncUsersChange(uc.ctx, lst)
			if err != nil {
				logger.Warning("failed to sync users to flashduty:", err)
				dumper.PutSyncRecord("users", start.Unix(), -1, -1, "failed to sync to flashduty: "+err.Error())
			}
		}()
	}

	ms := time.Since(start).Milliseconds()
	uc.stats.GaugeCronDuration.WithLabelValues("sync_users").Set(float64(ms))
	uc.stats.GaugeSyncNumber.WithLabelValues("sync_users").Set(float64(len(m)))

	logger.Infof("timer: sync users done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("users", start.Unix(), ms, len(m), "success")

	return nil
}

func (uc *UserCacheType) SetLastActiveTime(userId int64, lastActiveTime int64) {
	uc.Lock()
	defer uc.Unlock()
	if user, exists := uc.users[userId]; exists {
		user.LastActiveTime = lastActiveTime
	}
}

func (uc *UserCacheType) loopUpdateLastActiveTime() {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("panic to loopUpdateLastActiveTime(), err: %v", r)
		}
	}()

	// Sync every five minutes
	duration := 5 * time.Minute
	for {
		time.Sleep(duration)
		if err := uc.UpdateUsersLastActiveTime(); err != nil {
			logger.Warningf("failed to update users' last active time: %v", err)
		}
	}
}

func (uc *UserCacheType) UpdateUsersLastActiveTime() error {
	// read the full list of users from the database
	users, err := models.UserGetAll(uc.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to get all users from database")
	}

	for _, dbUser := range users {
		cacheUser := uc.GetByUserId(dbUser.Id)

		if cacheUser == nil {
			continue
		}

		if dbUser.LastActiveTime >= cacheUser.LastActiveTime {
			continue
		}

		err = models.UpdateUserLastActiveTime(uc.ctx, cacheUser.Id, cacheUser.LastActiveTime)
		if err != nil {
			logger.Warningf("failed to update last active time for user %d: %v", cacheUser.Id, err)
			return err
		}
	}

	return nil
}
