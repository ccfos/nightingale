package statsd

import (
	"fmt"
	"sync"
	"time"

	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

var (
	currentState     = &state{Metrics: map[string]*metricState{}, packageCounter: map[string]int{}}
	currentStateLock = &sync.RWMutex{}
)

type StatsdState struct{}

func (self StatsdState) GetState() *state {
	currentStateLock.RLock()
	ptr := currentState
	currentStateLock.RUnlock()
	return ptr
}

func (self StatsdState) RollState() *state {
	currentStateLock.Lock()
	oldState := currentState
	newState := &state{
		Metrics:        map[string]*metricState{},
		packageCounter: map[string]int{},
	}
	currentState = newState
	currentStateLock.Unlock()

	return oldState
}

////////////////////////////////////////////////////////////
// 						struct state
// 所有metric 的 所有tag组合 的 统计器, 全局只有一个
////////////////////////////////////////////////////////////
type state struct {
	isCollecting   bool
	Metrics        map[string]*metricState
	packageCounter map[string]int // 每个ns/metric的请求数统计, 用于INFO日志
}

// @input
//		value:   $value 或者 $value,$status "," 就是 ${CodeDelimiter}
//				 并包模式下 $value${MergeDelimeter}$value 或者 $value,$status${MergeDelimeter}$value,$status
//		metric:  $ns/$metric_name
//		argLines:$tagk1=$tagv2\n...$tagkN=$tagvN\n$aggr
func (self *state) Collect(value string, metric string, argLines string) error {
	self.isCollecting = true

	metricState, err := self.getMetricState(metric)
	if err != nil {
		self.isCollecting = false
		return err
	}

	// Metrics 与 packageCounter的 map key 相同
	if _, found := self.packageCounter[metric]; !found {
		self.packageCounter[metric] = 1
	} else {
		self.packageCounter[metric] += 1
	}

	err = metricState.Collect(value, metric, argLines)
	self.isCollecting = false
	return err
}

func (self *state) Size() int {
	cnt := 0
	for _, ms := range self.Metrics {
		cnt += len(ms.Aggrs)
	}
	return cnt
}

func (self *state) ToMap() (map[string]interface{}, error) {
	serialized := map[string]interface{}{}
	for k, v := range self.Metrics {
		m, err := v.ToMap()
		if err != nil {
			return nil, err
		}
		serialized[k] = m
	}
	return map[string]interface{}{"metrics": serialized}, nil
}

func (self *state) Summarize() {
	// 等待最后一次Collect执行完毕, 避免state内存区的读写冲突
	var waitMs int
	for waitMs = 0; waitMs < SummarizeWaitCollectTimeoutMsConst; waitMs += 5 {
		time.Sleep(5 * time.Millisecond)
		if !self.isCollecting {
			break
		}
	}
	if self.isCollecting {
		logger.Warningf("summarize wait collect timeout(%dms), summarize skipped", SummarizeWaitCollectTimeoutMsConst)
		return
	}

	// 调试信息
	if waitMs > 0 {
		logger.Debugf("system info: summarize wait collect %dms", waitMs)
	}

	for nsmetric, ms := range self.Metrics {
		ms.Summarize(nsmetric)
	}
}

func (self *state) getMetricState(metricName string) (*metricState, error) {
	metric, ok := self.Metrics[metricName]
	if ok && metric != nil {
		return metric, nil
	}

	metric = &metricState{Aggrs: map[string]aggregator{}}
	self.Metrics[metricName] = metric
	return metric, nil
}

////////////////////////////////////////////////////////////
// 					struct metricState
// 一个metric 的 所有tag组合的 统计器
////////////////////////////////////////////////////////////
type metricState struct {
	Aggrs map[string]aggregator
}

// @input
//		value:   $value 或者 $value,$status, "," 就是 ${CodeDelimiter}
//				 并包模式下 $value${MergeDelimeter}$value 或者 $value,$status${MergeDelimeter}$value,$status
//		metric:  $ns/$metric_name
//		argLines:$tagk1=$tagv2\n...$tagkN=$tagvN\n$aggr
func (self *metricState) Collect(value string, metric string, argLines string) error {
	aggregator, err := self.getAggregator(value, metric, argLines)
	if err != nil {
		return err
	}

	values, err := Func{}.TranslateValueLine(value)
	if err != nil {
		return err
	}

	// 记录实际的打点请求数
	stats.Counter.Set("metric.recv.cnt", len(values))
	return aggregator.collect(values, metric, argLines)
}

func (self *metricState) ToMap() (map[string]interface{}, error) {
	maps := map[string]interface{}{}
	for k, v := range self.Aggrs {
		m, err := v.toMap()
		if err != nil {
			return nil, err
		}
		maps[k] = m
	}

	return map[string]interface{}{"aggrs": maps}, nil
}

func (self *metricState) Summarize(nsmetric string) {
	if len(self.Aggrs) == 0 {
		return
	}

	newAggrs := make(map[string]aggregator, 0)
	// copy
	for argLines, aggr := range self.Aggrs {
		key := argLines
		ptrAggr := aggr
		newAggrs[key] = ptrAggr
	}
	// summarize
	for argLines, aggr := range self.Aggrs {
		key := argLines
		ptrAggr := aggr
		if ptrAggr == nil {
			continue
		}
		ptrAggr.summarize(nsmetric, key, newAggrs)
	}
	self.Aggrs = newAggrs
}

func (self *metricState) getAggregator(value, metric, argLines string) (aggregator, error) {
	aggr, ok := self.Aggrs[argLines]
	if ok && aggr != nil {
		return aggr, nil
	}

	// 创建 聚合器
	aggregatorNames, err := Func{}.GetAggrsFromArgLines(argLines)
	if err != nil {
		return nil, err
	}

	aggr, err = self.createAggregator(aggregatorNames, value, metric, argLines)
	if err != nil {
		return nil, err
	}
	self.Aggrs[argLines] = aggr
	return aggr, nil
}

func (self *metricState) createAggregator(aggregatorNames []string, value, metric, argLines string) (aggregator, error) {
	switch aggregatorNames[0] {
	case "c":
		return (&counterAggregator{}).new(aggregatorNames)
	case "ce":
		return (&counterEAggregator{}).new(aggregatorNames)
	case "g":
		return (&gaugeAggregator{}).new(aggregatorNames)
	case "rpc":
		return (&rpcAggregator{}).new(aggregatorNames)
	case "rpce":
		return (&rpcEAggregator{}).new(aggregatorNames)
	case "r":
		return (&ratioAggregator{}).new(aggregatorNames)
	case "rt":
		return (&ratioAsTagsAggregator{}).new(aggregatorNames)
	case "p1", "p5", "p25", "p50", "p75", "p90", "p95", "p99", "max", "min", "avg", "sum", "cnt":
		return (&histogramAggregator{}).new(aggregatorNames)
	default:
		return nil, fmt.Errorf("unknown aggregator %s", argLines)
	}
}

// internals
func (self state) StateFromMap(serialized map[string]interface{}) (*state, error) {
	state := &state{Metrics: map[string]*metricState{}}
	for k, v := range serialized {
		ms, err := (metricState{}.MetricFromMap(v.(map[string]interface{})))
		if err != nil {
			return nil, err
		}
		state.Metrics[k] = ms
	}
	return state, nil
}

func (self metricState) MetricFromMap(serialized map[string]interface{}) (*metricState, error) {
	metricState := &metricState{Aggrs: map[string]aggregator{}}
	keys := (serialized["aggrs"]).(map[string]interface{})
	for k, v := range keys {
		ret, err := self.aggregatorFromMap(v.(map[string]interface{}))
		if err != nil {
			return nil, err
		}
		metricState.Aggrs[k] = ret
	}
	return metricState, nil
}

func (self metricState) aggregatorFromMap(serialized map[string]interface{}) (aggregator, error) {
	switch serialized["__aggregator__"] {
	case "counter":
		return (&counterAggregator{}).fromMap(serialized)
	case "counterE":
		return (&counterEAggregator{}).fromMap(serialized)
	case "gauge":
		return (&gaugeAggregator{}).fromMap(serialized)
	case "ratio":
		return (&ratioAggregator{}).fromMap(serialized)
	case "ratioAsTags":
		return (&ratioAsTagsAggregator{}).fromMap(serialized)
	case "histogram":
		return (&histogramAggregator{}).fromMap(serialized)
	case "rpc":
		return (&rpcAggregator{}).fromMap(serialized)
	case "rpce":
		return (&rpcEAggregator{}).fromMap(serialized)
	default:
		return nil, fmt.Errorf("unknown aggregator: %v", serialized)
	}
}
