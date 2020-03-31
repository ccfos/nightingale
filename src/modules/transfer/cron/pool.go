package cron

import (
	"time"

	"github.com/didi/nightingale/src/modules/transfer/backend"
)

func RebuildJudgePool() {
	t1 := time.NewTicker(time.Duration(8) * time.Second)
	for {
		<-t1.C
		judges := backend.GetJudges()
		if len(judges) == 0 {
			//防止心跳服务故障导致judge不可用，如果judges个数为0，先不更新judge连接池
			continue
		}

		backend.JudgeConnPools.Update(judges)
	}
}
