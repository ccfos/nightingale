package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
	"github.com/toolkits/pkg/logger"
)

func SyncTeamUsers() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncTeamUsers()
	logger.Info("[cron] sync team start...")
	for {
		<-t1.C
		syncTeamUsers()
	}
}

func syncTeamUsers() {
	teamUsers, err := models.TeamUsers()

	if err != nil {
		logger.Warningf("get Teams err:%v %v", err)
	}

	teamUsersMap := make(map[int64][]int64)
	for _, teamUser := range teamUsers {
		if _, exists := teamUsersMap[teamUser.TeamId]; exists {
			teamUsersMap[teamUser.TeamId] = append(teamUsersMap[teamUser.TeamId], teamUser.UserId)
		} else {
			teamUsersMap[teamUser.TeamId] = []int64{teamUser.UserId}
		}
	}
	cache.TeamUsersCache.SetAll(teamUsersMap)
}
