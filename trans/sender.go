package trans

import (
	"time"

	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/judge"
	"github.com/didi/nightingale/v5/vos"
	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

// 多个judge实例，如果对端地址等于本地地址走内存
func send2JudgeTask(q *list.SafeListLimited, addr string) {
	if config.Config.Heartbeat.LocalAddr == addr {
		send2LocalJudge(q)
	} else {
		send2RemoteJudge(q, addr)
	}
}

func send2LocalJudge(q *list.SafeListLimited) {
	for {
		items := q.PopBackBy(config.Config.Judge.ReadBatch)

		count := len(items)
		if count == 0 {
			time.Sleep(time.Millisecond * 100)
			continue
		}

		points := make([]*vos.MetricPoint, count)
		for i := 0; i < count; i++ {
			item := items[i].(*vos.MetricPoint)
			item.TagsMap["ident"] = item.Ident
			points[i] = item
		}

		judge.Send(points)
	}

}

func send2RemoteJudge(q *list.SafeListLimited, addr string) {
	sema := semaphore.NewSemaphore(config.Config.Judge.WriterNum)

	for {
		items := q.PopBackBy(config.Config.Judge.ReadBatch)
		count := len(items)
		if count == 0 {
			time.Sleep(time.Millisecond * 50)
			if !queues.Exists(addr) {
				// 对端实例已挂，我已经没有存在的必要了
				logger.Infof("server instance %s dead, queue reader exiting...", addr)
				return
			}
			continue
		}

		judgeItems := make([]*vos.MetricPoint, count)
		for i := 0; i < count; i++ {
			judgeItems[i] = items[i].(*vos.MetricPoint)
		}

		sema.Acquire()
		go func(addr string, judgeItems []*vos.MetricPoint, count int) {
			defer sema.Release()

			var res string
			var err error
			sendOk := false
			for i := 0; i < 15; i++ {
				err = connPools.Call(addr, "Server.PushToJudge", judgeItems, &res)
				if err == nil {
					sendOk = true
					break
				}
				time.Sleep(time.Second)
			}

			if !sendOk {
				for _, item := range judgeItems {
					logger.Errorf("send %v to judge %s fail: %v", item, addr, err)
				}
			}

		}(addr, judgeItems, count)
	}
}
