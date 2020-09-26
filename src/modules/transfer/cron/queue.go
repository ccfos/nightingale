package cron

import (
	"time"

	"github.com/didi/nightingale/src/modules/transfer/backend"

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
	instances := backend.GetJudges()
	if len(instances) == 0 {
		return
	}

	for _, instance := range instances {
		if !backend.JudgeQueues.Exists(instance) {
			q := list.NewSafeListLimited(backend.DefaultSendQueueMaxSize)
			backend.JudgeQueues.Set(instance, q)
			go backend.Send2JudgeTask(q, instance, backend.Judge.WorkerNum)
		} else {
			backend.JudgeQueues.UpdateTS(instance)
		}
	}
	backend.JudgeQueues.Clean()
}
