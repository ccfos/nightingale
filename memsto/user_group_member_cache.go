package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type UserGroupMemberCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	gUsers  map[int64][]int64 //key: group_id
	uGroups map[int64][]int64 //key: user_id
}

func NewUserGroupMemberCache(ctx *ctx.Context, stats *Stats) *UserGroupMemberCacheType {
	ugc := &UserGroupMemberCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		gUsers:          make(map[int64][]int64),
		uGroups:         make(map[int64][]int64),
	}
	ugc.SyncUserGroupMember()
	return ugc
}

func (ugc *UserGroupMemberCacheType) StatChanged(total, lastUpdated int64) bool {
	if ugc.statTotal == total && ugc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (ugc *UserGroupMemberCacheType) Set(gUsers, uGroups map[int64][]int64, total, lastUpdated int64) {
	ugc.Lock()
	ugc.gUsers = gUsers
	ugc.uGroups = uGroups
	ugc.Unlock()

	// only one goroutine used, so no need lock
	ugc.statTotal = total
	ugc.statLastUpdated = lastUpdated
}

func (ugc *UserGroupMemberCacheType) GetUidByGroupIds(gid []int64) []int64 {
	set := make(map[int64]struct{})
	ugc.RLock()
	defer ugc.RUnlock()
	var res []int64
	for i := range gid {
		if ugc.gUsers[gid[i]] == nil { // no value
			continue
		}
		users := ugc.gUsers[gid[i]]
		for j := range users {
			if _, has := set[users[j]]; has {
				continue
			}
			res = append(res, users[j])
			set[users[j]] = struct{}{}
		}
	}
	if res == nil {
		return []int64{}
	}

	return res
}
func (ugc *UserGroupMemberCacheType) GetGidByUserIds(uid []int64) []int64 {
	set := make(map[int64]struct{})

	ugc.RLock()
	defer ugc.RUnlock()

	var res []int64
	for _, id := range uid {
		if ugc.uGroups[id] == nil {
			continue
		}
		groups := ugc.uGroups[id]
		for i := range groups {
			if _, has := set[groups[i]]; has {
				continue
			}

			res = append(res, groups[i])
			set[id] = struct{}{}
		}

	}

	if res == nil {
		return []int64{}
	}

	return res
}

func (ugc *UserGroupMemberCacheType) SyncUserGroupMember() {
	err := ugc.syncUserGroupMembers()
	if err != nil {
		fmt.Println("failed to sync user group member:", err)
		exit(1)
	}

	go ugc.loopSyncUserGroupMembers()
}

func (ugc *UserGroupMemberCacheType) loopSyncUserGroupMembers() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := ugc.syncUserGroupMembers(); err != nil {
			logger.Warning("failed to sync user group member:", err)
		}
	}
}

func (ugc *UserGroupMemberCacheType) syncUserGroupMembers() error {
	start := time.Now()

	stat, err := models.UserGroupMemberStatistics(ugc.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to exec UserGroupMemberStatistics")
	}

	if !ugc.StatChanged(stat.Total, stat.LastUpdated) {
		ugc.stats.GaugeCronDuration.WithLabelValues("sync_user_group_members").Set(0)
		ugc.stats.GaugeSyncNumber.WithLabelValues("sync_user_group_members").Set(0)

		logger.Debug("user_group_members not changed")
		return nil
	}

	lst, err := models.UserGroupMemberGetAll(ugc.ctx)
	if err != nil {
		return errors.WithMessage(err, "failed to exec UserGroupGetAll")
	}

	mGUsers, mUGroups := make(map[int64][]int64), make(map[int64][]int64)
	for i := 0; i < len(lst); i++ {
		if mUGroups[lst[i].UserId] == nil { //key uid : value []gid
			mUGroups[lst[i].UserId] = make([]int64, 0)
		}
		mUGroups[lst[i].UserId] = append(mUGroups[lst[i].UserId], lst[i].GroupId)
		if mGUsers[lst[i].GroupId] == nil { //key gid : value []uid
			mGUsers[lst[i].GroupId] = make([]int64, 0)
		}
		mGUsers[lst[i].GroupId] = append(mGUsers[lst[i].GroupId], lst[i].UserId)
	}

	ugc.Set(mGUsers, mUGroups, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	ugc.stats.GaugeCronDuration.WithLabelValues("sync_user_group_members").Set(float64(ms))
	ugc.stats.GaugeSyncNumber.WithLabelValues("sync_user_group_members").Set(float64(len(mUGroups)))

	logger.Infof("timer: sync user groups member done, cost: %dms, gUser number: %d, uGroup number: %d", ms, len(mGUsers), len(mUGroups))

	return nil
}
