package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/models"
)

// Update 对于上报的监控数据，会缓存在内存里，然后周期性更新其alias
// 主要是性能考虑，要不然每秒上报千万条监控指标，每条都去更新alias耗时太久
// server是无状态的，对于某个ident，如果刚开始上报alias1到server1，后来上报alias2到server2
// 如果server1和server2同时去更新数据库，可能会造成混乱，一会是alias1，一会是alias2
// 所以，models.UpdateAlias中做了一个逻辑，先清空了15s之前的数据，这样就可以保证只需要更新新数据即可
// go进程中的AliasMapper这个变量，在进程运行时间久了之后不知道是否会让内存持续增长而不释放
// 如果真的出现这个问题，可能要考虑把这个变量的存储放到redis之类的KV中
func UpdateAlias() {
	go loopUpdateAlias()
}

func loopUpdateAlias() {
	randtime := rand.Intn(2000)
	fmt.Printf("timer: update alias: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	// 5s跑一次，只会使用最近15s有过更新的数据，在models.UpdateAlias有15s的清理逻辑
	interval := time.Duration(5) * time.Second

	for {
		time.Sleep(interval)
		updateAlias()
	}
}

func updateAlias() {
	err := models.UpdateAlias()
	if err != nil {
		logger.Warningf("UpdateAlias fail: %v", err)
	}
}
