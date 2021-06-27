package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"

	"github.com/toolkits/pkg/logger"
)

func SyncUserGroupMember() {
	if err := syncUserGroupMember(); err != nil {
		fmt.Println(err)
		exit(1)
	}

	go loopSyncUserGroupMember()
}

func loopSyncUserGroupMember() {
	randtime := rand.Intn(60000)
	fmt.Printf("timer: sync group users: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	interval := time.Duration(60) * time.Second

	for {
		time.Sleep(interval)
		if err := syncUserGroupMember(); err != nil {
			logger.Warning(err)
		}
	}
}

func syncUserGroupMember() error {
	start := time.Now()

	members, err := models.UserGroupMemberGetAll()
	if err != nil {
		return fmt.Errorf("UserGroupMemberGetAll error: %v", err)
	}

	memberMap := make(map[int64]map[int64]struct{})
	for _, m := range members {
		if _, exists := memberMap[m.GroupId]; !exists {
			memberMap[m.GroupId] = make(map[int64]struct{})
		}
		memberMap[m.GroupId][m.UserId] = struct{}{}
	}

	cache.UserGroupMember.SetAll(memberMap)

	logger.Debugf("timer: sync group users done, cost: %dms", time.Since(start).Milliseconds())

	return nil
}
