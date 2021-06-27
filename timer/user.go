package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"

	"github.com/toolkits/pkg/logger"
)

// userid->user 将数据库中的用户信息缓存在内存里，
// 在生成告警事件的时候，根据用户ID快速找到用户的详情
func SyncUsers() {
	err := syncUsers()
	if err != nil {
		fmt.Println("timer: sync users fail:", err)
		exit(1)
	}

	go loopSyncUsers()
}

func loopSyncUsers() {
	randtime := rand.Intn(9000)
	fmt.Printf("timer: sync users: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	for {
		time.Sleep(time.Second * time.Duration(9))
		err := syncUsers()
		if err != nil {
			logger.Warning("timer: sync users fail:", err)
		}
	}
}

func syncUsers() error {
	start := time.Now()

	users, err := models.UserGetAll()
	if err != nil {
		return err
	}

	usersMap := make(map[int64]*models.User)
	for i := range users {
		usersMap[users[i].Id] = &users[i]
	}

	cache.UserCache.SetAll(usersMap)
	logger.Debugf("timer: sync users done, cost: %dms", time.Since(start).Milliseconds())

	return nil
}
