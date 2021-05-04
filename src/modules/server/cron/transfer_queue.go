package cron

import (
	"time"

	"github.com/didi/nightingale/v4/src/modules/server/judge"

	"github.com/toolkits/pkg/container/list"
)

func UpdateJudgeQueue() {
	ticker := time.NewTicker(time.Duration(8) * time.Second)
	for {
		<-ticker.C
		updateJudgeQueue()
	}
}

func updateJudgeQueue() {
	instances := judge.GetJudges()
	if len(instances) == 0 {
		return
	}

	for _, instance := range instances {
		if !judge.JudgeQueues.Exists(instance) {
			q := list.NewSafeListLimited(judge.DefaultSendQueueMaxSize)
			judge.JudgeQueues.Set(instance, q)
			go judge.Send2JudgeTask(q, instance, judge.JudgeConfig.WorkerNum)
		} else {
			judge.JudgeQueues.UpdateTS(instance)
		}
	}
	judge.JudgeQueues.Clean()
}
