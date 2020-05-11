package worker

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/dataobj"
)

// cached时间周期
const cachedDuration = 60

type counterCache struct {
	sync.RWMutex
	Points map[string]float64 `json:"points"`
}

type pushPointsCache struct {
	sync.RWMutex
	Counters map[string]*counterCache `json:"counters"`
}

var globalPushPoints = pushPointsCache{Counters: make(map[string]*counterCache)}

func init() {
	go CleanLoop()
}

func PostToCache(paramPoints []*dataobj.MetricValue) {
	for _, point := range paramPoints {
		globalPushPoints.AddPoint(point)
	}
}

func CleanLoop() {
	for {
		// 遍历，删掉过期的cache
		globalPushPoints.CleanOld()
		time.Sleep(5 * time.Second)
	}
}

func GetCachedAll() string {
	globalPushPoints.Lock()
	str, err := json.Marshal(globalPushPoints)
	globalPushPoints.Unlock()
	if err != nil {
		logger.Errorf("err when get cached all : [%s]", err.Error())
	}
	return string(str)
}

func (cc *counterCache) AddPoint(tms int64, value float64) {
	cc.Lock()
	tmsStr := fmt.Sprintf("%d", tms)
	cc.Points[tmsStr] = value
	cc.Unlock()
}

func (cc *counterCache) GetKeys() []string {
	cc.RLock()
	retList := make([]string, 0)
	for k := range cc.Points {
		retList = append(retList, k)
	}
	cc.RUnlock()
	return retList
}

func (cc *counterCache) RemoveTms(tms string) {
	cc.Lock()
	delete(cc.Points, tms)
	cc.Unlock()
}

func (pc *pushPointsCache) AddCounter(counter string) {
	pc.Lock()
	tmp := new(counterCache)
	tmp.Points = make(map[string]float64)
	pc.Counters[counter] = tmp
	pc.Unlock()
}

func (pc *pushPointsCache) GetCounters() []string {
	ret := make([]string, 0)
	pc.RLock()
	for k := range pc.Counters {
		ret = append(ret, k)
	}
	pc.RUnlock()
	return ret
}

func (pc *pushPointsCache) RemoveCounter(counter string) {
	pc.Lock()
	delete(pc.Counters, counter)
	pc.Unlock()
}

func (pc *pushPointsCache) GetCounterObj(key string) (*counterCache, bool) {
	pc.RLock()
	Points, ok := pc.Counters[key]
	pc.RUnlock()

	return Points, ok
}

func (pc *pushPointsCache) AddPoint(point *dataobj.MetricValue) {
	counter := calcCounter(point)
	if _, ok := pc.GetCounterObj(counter); !ok {
		pc.AddCounter(counter)
	}
	counterPoints, exists := pc.GetCounterObj(counter)
	if exists {
		counterPoints.AddPoint(point.Timestamp, point.Value)
	}
}

func (pc *pushPointsCache) CleanOld() {
	counters := pc.GetCounters()
	for _, counter := range counters {
		counterObj, exists := pc.GetCounterObj(counter)
		if !exists {
			continue
		}
		tmsList := counterObj.GetKeys()

		//如果列表为空，清理掉这个counter
		if len(tmsList) == 0 {
			pc.RemoveCounter(counter)
			continue
		}

		for _, tmsStr := range tmsList {
			tms, err := strconv.Atoi(tmsStr)
			if err != nil {
				logger.Errorf("clean cached point, atoi error : [%v]", err)
				counterObj.RemoveTms(tmsStr)
			} else if (time.Now().Unix() - int64(tms)) > cachedDuration {
				counterObj.RemoveTms(tmsStr)
			}
		}
	}
}

func calcCounter(point *dataobj.MetricValue) string {
	tagstring := dataobj.SortedTags(point.TagsMap)
	counter := fmt.Sprintf("%s/%s", point.Metric, tagstring)
	return counter
}
