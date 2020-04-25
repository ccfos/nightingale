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

func (c *counterCache) AddPoint(tms int64, value float64) {
	c.Lock()
	tmsStr := fmt.Sprintf("%d", tms)
	c.Points[tmsStr] = value
	c.Unlock()
}

func (c *counterCache) GetKeys() []string {
	c.RLock()
	retList := make([]string, 0)
	for k := range c.Points {
		retList = append(retList, k)
	}
	c.RUnlock()
	return retList
}

func (c *counterCache) RemoveTms(tms string) {
	c.Lock()
	delete(c.Points, tms)
	c.Unlock()
}

func (c *pushPointsCache) AddCounter(counter string) {
	c.Lock()
	tmp := new(counterCache)
	tmp.Points = make(map[string]float64)
	c.Counters[counter] = tmp
	c.Unlock()
}

func (c *pushPointsCache) GetCounters() []string {
	ret := make([]string, 0)
	c.RLock()
	for k := range c.Counters {
		ret = append(ret, k)
	}
	c.RUnlock()
	return ret
}

func (c *pushPointsCache) RemoveCounter(counter string) {
	c.Lock()
	delete(c.Counters, counter)
	c.Unlock()
}

func (c *pushPointsCache) GetCounterObj(key string) (*counterCache, bool) {
	c.RLock()
	Points, ok := c.Counters[key]
	c.RUnlock()

	return Points, ok
}

func (c *pushPointsCache) AddPoint(point *dataobj.MetricValue) {
	counter := calcCounter(point)
	if _, ok := c.GetCounterObj(counter); !ok {
		c.AddCounter(counter)
	}
	counterPoints, exists := c.GetCounterObj(counter)
	if exists {
		counterPoints.AddPoint(point.Timestamp, point.Value)
	}
}

func (c *pushPointsCache) CleanOld() {
	counters := c.GetCounters()
	for _, counter := range counters {
		counterObj, exists := c.GetCounterObj(counter)
		if !exists {
			continue
		}
		tmsList := counterObj.GetKeys()

		//如果列表为空，清理掉这个counter
		if len(tmsList) == 0 {
			c.RemoveCounter(counter)
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

func PostToCache(paramPoints []*dataobj.MetricValue) {
	for _, point := range paramPoints {
		globalPushPoints.AddPoint(point)
	}
}

func CleanLoop() {
	for {
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

func calcCounter(point *dataobj.MetricValue) string {
	tagstring := dataobj.SortedTags(point.TagsMap)
	counter := fmt.Sprintf("%s/%s", point.Metric, tagstring)
	return counter
}
