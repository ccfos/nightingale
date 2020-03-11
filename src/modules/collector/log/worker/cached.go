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
const CACHED_DURATION = 60

type counterCache struct {
	sync.RWMutex
	Points map[string]float64 `json:"points"`
}

type pushPointsCache struct {
	sync.RWMutex
	Counters map[string]*counterCache `json:"counters"`
}

var globalPushPoints = pushPointsCache{Counters: make(map[string]*counterCache, 0)}

func init() {
	go CleanLoop()
}

func (this *counterCache) AddPoint(tms int64, value float64) {
	this.Lock()
	tmsStr := fmt.Sprintf("%d", tms)
	this.Points[tmsStr] = value
	this.Unlock()
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

func (this *counterCache) GetKeys() []string {
	this.RLock()
	retList := make([]string, 0)
	for k, _ := range this.Points {
		retList = append(retList, k)
	}
	this.RUnlock()
	return retList
}

func (this *counterCache) RemoveTms(tms string) {
	this.Lock()
	delete(this.Points, tms)
	this.Unlock()
}

func (this *pushPointsCache) AddCounter(counter string) {
	this.Lock()
	tmp := new(counterCache)
	tmp.Points = make(map[string]float64, 0)
	this.Counters[counter] = tmp
	this.Unlock()
}

func (this *pushPointsCache) GetCounters() []string {
	ret := make([]string, 0)
	this.RLock()
	for k, _ := range this.Counters {
		ret = append(ret, k)
	}
	this.RUnlock()
	return ret
}

func (this *pushPointsCache) RemoveCounter(counter string) {
	this.Lock()
	delete(this.Counters, counter)
	this.Unlock()
}

func (this *pushPointsCache) GetCounterObj(key string) (*counterCache, bool) {
	this.RLock()
	Points, ok := this.Counters[key]
	this.RUnlock()

	return Points, ok
}

func (this *pushPointsCache) AddPoint(point *dataobj.MetricValue) {
	counter := calcCounter(point)
	if _, ok := this.GetCounterObj(counter); !ok {
		this.AddCounter(counter)
	}
	counterPoints, exists := this.GetCounterObj(counter)
	if exists {
		counterPoints.AddPoint(point.Timestamp, point.Value)
	}
}

func (this *pushPointsCache) CleanOld() {
	counters := this.GetCounters()
	for _, counter := range counters {
		counterObj, exists := this.GetCounterObj(counter)
		if !exists {
			continue
		}
		tmsList := counterObj.GetKeys()

		//如果列表为空，清理掉这个counter
		if len(tmsList) == 0 {
			this.RemoveCounter(counter)
		} else {
			for _, tmsStr := range tmsList {
				tms, err := strconv.Atoi(tmsStr)
				if err != nil {
					logger.Errorf("clean cached point, atoi error : [%v]", err)
					counterObj.RemoveTms(tmsStr)
				} else if (time.Now().Unix() - int64(tms)) > CACHED_DURATION {
					counterObj.RemoveTms(tmsStr)
				}
			}
		}
	}
}

func calcCounter(point *dataobj.MetricValue) string {
	tagstring := dataobj.SortedTags(point.TagsMap)
	counter := fmt.Sprintf("%s/%s", point.Metric, tagstring)
	return counter
}
