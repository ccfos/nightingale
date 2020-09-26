package cron

import (
	"time"

	"github.com/didi/nightingale/src/modules/transfer/backend"
)

func RebuildJudgePool() {
	ticker := time.NewTicker(time.Duration(8) * time.Second)
	for {
		<-ticker.C
		judges := backend.GetJudges()
		if len(judges) == 0 {
			//防止心跳服务故障导致 judge 不可用，如果 judges 个数为 0，先不更新 judge 连接池
			continue
		}

		backend.JudgeConnPools.UpdatePools(judges)
	}
}
