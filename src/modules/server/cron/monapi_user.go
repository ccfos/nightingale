package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
	"github.com/toolkits/pkg/logger"
)

func SyncUsers() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncUsers()
	logger.Info("[cron] sync user start...")
	for {
		<-t1.C
		syncUsers()
	}
}

func syncUsers() {
	users, err := models.AllUsers()
	if err != nil {
		logger.Warningf("get users err:%v %v", err)
		return
	}

	usersMap := make(map[int64]*models.User)
	for i, _ := range users {
		usersMap[users[i].Id] = &users[i]
	}

	cache.UserCache.SetAll(usersMap)
}
