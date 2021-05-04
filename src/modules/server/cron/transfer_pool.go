package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/modules/server/judge"
)

func RebuildJudgePool() {
	ticker := time.NewTicker(time.Duration(8) * time.Second)
	for {
		<-ticker.C
		judges := judge.GetJudges()
		if len(judges) == 0 {
			//防止心跳服务故障导致 judge 不可用，如果 judges 个数为 0，先不更新 judge 连接池
			continue
		}

		judge.JudgeConnPools.UpdatePools(judges)
	}
}
