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
			return
		}

		backend.JudgeConnPools.Update(judges)
	}
}
