package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
	"github.com/toolkits/pkg/logger"
)

func SyncTeams() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncTeam()
	logger.Info("[cron] sync team start...")
	for {
		<-t1.C
		syncTeam()
	}
}

func syncTeam() {
	teams, err := models.AllTeams()
	if err != nil {
		logger.Warningf("get teams err:%v %v", err)
		return
	}

	teamsMap := make(map[int64]*models.Team)
	for _, team := range teams {
		teamsMap[team.Id] = &team
	}

	cache.TeamCache.SetAll(teamsMap)
}
