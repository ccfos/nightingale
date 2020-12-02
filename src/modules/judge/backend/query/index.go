package query

import (
	"fmt"
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/report"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

var IndexList IndexAddrs

type IndexAddrs struct {
	sync.RWMutex
	Data []string
}

func (i *IndexAddrs) Set(addrs []string) {
	i.Lock()
	defer i.Unlock()
	i.Data = addrs
}

func (i *IndexAddrs) Get() []string {
	i.RLock()
	defer i.RUnlock()
	return i.Data
}

func GetIndexLoop(hbsMod string) {
	t1 := time.NewTicker(time.Duration(9) * time.Second)
	GetIndex(hbsMod)
	for {
		<-t1.C
		GetIndex(hbsMod)
	}
}

func GetIndex(hbsMod string) {
	instances, err := report.GetAlive(Config.IndexMod, hbsMod)
	if err != nil {
		stats.Counter.Set("get.index.err", 1)
		logger.Warningf("get index list err:%v", err)
		return
	}

	activeIndexs := []string{}
	for _, instance := range instances {
		activeIndexs = append(activeIndexs, fmt.Sprintf("%s:%s", instance.Identity, instance.HTTPPort))
	}

	IndexList.Set(activeIndexs)
	return
}
