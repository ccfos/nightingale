package statsd

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/config"
	"github.com/didi/nightingale/src/toolkits/exit"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

type StatsdReporter struct{}

// point to n9e-agent
type Point struct {
	Namespace string            `json:"namespace"`
	Name      string            `json:"name"`
	Timestamp int64             `json:"timestamp"`
	Tags      map[string]string `json:"tags"`
	Value     float64           `json:"value"`
	Step      int               `json:"step"`
}

func (self *Point) String() string {
	return fmt.Sprintf("<namespace:%s, name:%s, timestamp:%d, value:%v, step:%d, tags:%v>",
		self.Namespace, self.Name, self.Timestamp, self.Value, self.Step, self.Tags)
}

func (self Point) Strings(points []*Point) string {
	pointsString := ""
	for _, p := range points {
		pointsString += p.String() + "\n"
	}
	return pointsString
}

var (
	lastPointLock = &sync.RWMutex{}
	lastPoints    []*Point
)

var (
	isFirstPeriod = true // metrics启动后的第一个统计周期(非线程安全)
)

func (self StatsdReporter) Report() {
	// init schedule
	schedule := &schedule{}
	schedule.clearStateAt = self.nextTenSeconds(time.Now())
	schedule.reportAt = schedule.clearStateAt

	// send loop
	for !IsExited() {
		actions := schedule.listActions(time.Now())
		if len(actions) != 0 {
			self.handleActions(actions)
		}
		time.Sleep(time.Duration(config.Config.Metrics.ReportIntervalMs) * time.Millisecond)
	}
}

func (self StatsdReporter) LastPoints() []*Point {
	lastPointLock.RLock()
	ret := lastPoints
	lastPointLock.RUnlock()
	return ret
}

func (self StatsdReporter) setLastPoints(ps []*Point) {
	lastPointLock.Lock()
	lastPoints = ps
	lastPointLock.Unlock()
}

func (self StatsdReporter) handleActions(actions []action) {
	defer func() {
		if err := recover(); err != nil {
			stack := exit.Stack(3)
			logger.Warningf("udp handler exit unexpected, [error: %v],[stack: %s]", err, stack)
		}
	}()

	for _, action := range actions {
		switch action.actionType {
		case "report":
			previousState := StatsdState{}.RollState()
			//previousState.Summarize() // 指标进一步聚合,得到类似<all>的tag值

			// 第一个统计周期不准确, 扔掉
			if isFirstPeriod {
				isFirstPeriod = false
				break
			}

			// report cnt
			stats.Counter.Set("metric.report.cnt", 1)

			// proc
			stats.Counter.Set("metric.cache.size", previousState.Size())

			//startTs := time.Now()
			//cnt := self.translateAndSend(previousState, action.toTime, 10, action.prefix)

			// proc
			//latencyMs := int64(time.Now().Sub(startTs).Nanoseconds() / 1000000)
		default:
			logger.Debugf("ignored action %s", action.actionType)
		}
	}
}

func (self StatsdReporter) nextTenSeconds(t time.Time) time.Time {
	nowSec := t.Second()
	clearStateSec := ((nowSec / 10) * 10)
	diff := 10 - (nowSec - clearStateSec)
	t = t.Add(time.Duration(-t.Nanosecond()) * time.Nanosecond)
	return t.Add(time.Duration(diff) * time.Second)
}

func (self StatsdReporter) translateToN9EPoints(state *state, reportTime time.Time) []*Point {
	ts := reportTime.Unix()
	allPoints := make([]*Point, 0)
	for rawMetric, metricState := range state.Metrics {
		// 此处不考虑异常: 数据进入时 已经对metric行做了严格校验
		items, _ := Func{}.TranslateMetricLine(rawMetric)
		namespace := items[0]
		metric := items[1]

		for key, aggregator := range metricState.Aggrs {
			if nil == aggregator {
				continue
			}

			var (
				tags map[string]string
				err  error
			)
			// 包含 <all> 关键字, 是聚合的结果, 不能从缓存中查询
			if strings.Contains(key, "<all>") {
				tags, _, err = Func{}.TranslateArgLines(key, true)
			} else {
				tags, _, err = Func{}.TranslateArgLines(key)
			}

			if err != nil {
				logger.Warningf("post points to n9e-agent failed, tags/aggr error, "+
					"[msg: %s][nid/metric: %s][tags/aggr: %s]", err.Error(), rawMetric, key)
				continue
			}

			points := make([]*Point, 0)
			points, err = aggregator.dump(points, ts, tags, metric, key)
			if err != nil {
				logger.Warningf("post points to n9e-agent failed, generate points error, "+
					"[msg: %s][ns/metric: %s][tags/aggr: %s]", err.Error(), rawMetric, key)
				continue
			}

			for _, point := range points {
				point.Namespace = namespace
				allPoints = append(allPoints, point)
			}
		}
	}
	return allPoints
}

func TranslateToN9EPoints(points []*Point) []*dataobj.MetricValue {
	objs := []*dataobj.MetricValue{}
	for _, p := range points {
		obj := dataobj.MetricValue{
			Nid:          p.Namespace,
			Metric:       p.Name,
			Timestamp:    p.Timestamp,
			Step:         int64(p.Step),
			ValueUntyped: p.Value,
			TagsMap:      p.Tags,
		}
		objs = append(objs, &obj)
	}

	return objs
}

//
type action struct {
	actionType    string
	fromTime      time.Time
	toTime        time.Time
	fromFrequency int // in seconds
	toFrequency   int // in seconds
	prefix        string
}

//
type schedule struct {
	clearStateAt time.Time
	reportAt     time.Time
}

func (self *schedule) listActions(now time.Time) []action {
	actions := make([]action, 0)
	if now.After(self.reportAt) {
		actions = append(actions, action{
			actionType:  "report",
			fromTime:    self.reportAt.Add(-10 * time.Second),
			toTime:      self.reportAt,
			toFrequency: 10,
			prefix:      "",
		})
		self.reportAt = StatsdReporter{}.nextTenSeconds(now)
	}
	return actions
}
