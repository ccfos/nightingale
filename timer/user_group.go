package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"

	"github.com/toolkits/pkg/logger"
)

// user_group_id->user_group 将数据库中的用户信息缓存在内存里，
// 在生成告警事件的时候，根据用户ID快速找到用户的详情
func SyncUserGroups() {
	err := syncUserGroups()
	if err != nil {
		fmt.Println("timer: sync users fail:", err)
		exit(1)
	}

	go loopSyncUserGroups()
}

func loopSyncUserGroups() {
	randtime := rand.Intn(9000)
	fmt.Printf("timer: sync users: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	for {
		time.Sleep(time.Second * time.Duration(9))
		err := syncUserGroups()
		if err != nil {
			logger.Warning("timer: sync users fail:", err)
		}
	}
}

func syncUserGroups() error {
	start := time.Now()

	userGroups, err := models.UserGroupGetAll()
	if err != nil {
		return err
	}

	userGroupsMap := make(map[int64]*models.UserGroup)
	for i := range userGroups {
		userGroupsMap[userGroups[i].Id] = &userGroups[i]
	}

	cache.UserGroupCache.SetAll(userGroupsMap)
	logger.Debugf("timer: sync userGroups done, cost: %dms", time.Since(start).Milliseconds())

	return nil
}
