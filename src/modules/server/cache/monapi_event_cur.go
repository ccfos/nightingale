package cache

import (
	"github.com/toolkits/pkg/logger"

	"sync"
	"time"

	"github.com/didi/nightingale/v4/src/models"
)

type HashIdEventCurMap struct {
	sync.RWMutex
	Data map[uint64]*models.EventCur
}

var HashIdEventCurMapCache *HashIdEventCurMap

func NewHashIdEventCurMapCache() *HashIdEventCurMap {
	return &HashIdEventCurMap{Data: make(map[uint64]*models.EventCur)}
}

func (t *HashIdEventCurMap) GetBy(hashid uint64) *models.EventCur {
	t.RLock()
	defer t.RUnlock()

	return t.Data[hashid]
}

func (t *HashIdEventCurMap) GetAll() []*models.EventCur {
	t.RLock()
	defer t.RUnlock()
	var objs []*models.EventCur
	for _, obj := range t.Data {
		objs = append(objs, obj)
	}
	return objs
}

func (t *HashIdEventCurMap) SetAll(objs map[uint64]*models.EventCur) {
	t.Lock()
	defer t.Unlock()

	t.Data = objs
	return
}

func SyncHashIdEventCur() {
	t1 := time.NewTicker(time.Duration(60) * time.Second)

	syncHashIdEventCur()
	logger.Info("[cron] sync HashIdEventCur cron start...")
	for {
		<-t1.C
		logger.Info("[cron] sync HashIdEventCur start...")
		syncHashIdEventCur()
		logger.Info("[cron] sync HashIdEventCur end...")

	}
}

func syncHashIdEventCur() {
	allEventCur, err := models.AllEventCurGet()
	if err != nil {
		logger.Warningf("get all EventCur err:%v %v", err)
		return
	}

	allEventCurMap := make(map[uint64]*models.EventCur)
	for _, v := range allEventCur {
		allEventCurMap[v.HashId] = &v
	}

	HashIdEventCurMapCache.SetAll(allEventCurMap)
}
