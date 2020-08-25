package worker

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/core"
	"github.com/didi/nightingale/src/modules/collector/stra"
	"github.com/didi/nightingale/src/toolkits/identity"

	"github.com/toolkits/pkg/logger"
)

var pushQueue chan *dataobj.MetricValue

type SortByTms []*dataobj.MetricValue

func (p SortByTms) Len() int           { return len(p) }
func (p SortByTms) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p SortByTms) Less(i, j int) bool { return p[i].Timestamp < p[j].Timestamp }

func init() {
	//拍一个队列大小,10s一清，理论上肯定够用
	pushQueue = make(chan *dataobj.MetricValue, 1024*100)
}

func PusherStart() {
	PosterLoop() //归类，批量发送给collector
	PusherLoop() //计算，推送给发送队列
}

//循环推送，10s一次
func PosterLoop() {
	go func() {
		for {
			select {
			case p := <-pushQueue:
				points := make([]*dataobj.MetricValue, 0)
				points = append(points, p)
			DONE:
				for {
					select {
					case tmp := <-pushQueue:
						points = append(points, tmp)
						continue
					default:
						break DONE
					}
				}
				//先推到cache中
				PostToCache(points)
				//开一个协程，异步发送至collector
				go postToCollector(points)
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

func PusherLoop() {
	for {
		gIds := GlobalCount.GetIDs()
		for _, id := range gIds {
			stCount, err := GlobalCount.GetStrategyCountByID(id)
			if err != nil {
				logger.Errorf("get strategy count by id %v error: %v\n", id, err)
				continue
			}

			if stCount.Strategy == nil {
				logger.Errorf("strategy id %v is nil\n", id)
				continue
			}

			step := stCount.Strategy.Interval
			filePath := stCount.Strategy.FilePath
			tmsList := stCount.GetTmsList()
			for _, tms := range tmsList {
				if tmsNeedPush(tms, filePath, step, WorkerConfig.WaitPush) {
					pointsCount, err := stCount.GetByTms(tms)
					if err == nil {
						ToPushQueue(stCount.Strategy, tms, pointsCount.TagstringMap)
					} else {
						logger.Errorf("get by tms [%d] error : %v", tms, err)
					}
					stCount.DeleteTms(tms)
				}
			}
		}
		time.Sleep(time.Second * time.Duration(WorkerConfig.PushInterval))
	}
}

func tmsNeedPush(tms int64, filePath string, step int64, waitPush int) bool {

	latest, delay, found := GetLatestTmsAndDelay(filePath)
	logger.Debugf("filepath:%s tms:%d latest tms:%d delay:%d", filePath, tms, latest, delay)

	if !found {
		return true
	}

	// 为解决日志时间戳乱序的最大等待时间, hard code
	// delay == 0时, 不用额外等待, 进而提高时效性
	if delay > 0 {
		var maxDelay int64
		if step <= 10 {
			maxDelay = step * 3
		} else if step > 10 && step <= 30 {
			maxDelay = step * 2
		} else {
			maxDelay = step
		}
		if delay > maxDelay {
			delay = maxDelay
		}
	}

	waitTime := step
	if waitPush != 0 {
		waitTime = int64(waitPush)
	}

	//如果日志文件更新时间晚于一个采集周期，则进行补零
	if latest < time.Now().Unix()-waitTime {
		return true
	}

	if tms < AlignStepTms(step, latest-delay) {
		return true
	}

	return false
}

// 这个参数是为了最大限度的对接
// pointMap的key，是打平了的tagkv
func ToPushQueue(strategy *stra.Strategy, tms int64, pointMap map[string]*PointCounter) error {
	for tagstring, PointCounter := range pointMap {
		var value float64 = 0
		switch strategy.Func {
		case "cnt":
			value = float64(PointCounter.Count)
		case "avg":
			if PointCounter.Count == 0 {
				//这种就不用往监控推了
				continue
			} else {
				avg := PointCounter.Sum / float64(PointCounter.Count)
				value = getPrecision(avg, strategy.Degree)
			}
		case "sum":
			value = PointCounter.Sum
		case "max":
			value = PointCounter.Max
		case "min":
			value = PointCounter.Min
		default:
			logger.Errorf("Strategy Func Error: %s ", strategy.Func)
			return fmt.Errorf("Strategy Func Error: %s ", strategy.Func)
		}

		var tags map[string]string
		if tagstring == "null" {
			tags = make(map[string]string)
		} else {
			tags = dataobj.DictedTagstring(tagstring)
		}

		if math.IsNaN(value) {
			continue
		}

		tmpPoint := &dataobj.MetricValue{
			Metric:       strategy.Name,
			Endpoint:     identity.Identity,
			ValueUntyped: value,
			Timestamp:    tms,
			Step:         strategy.Interval,
			TagsMap:      tags,
			CounterType:  "GAUGE",
		}
		//metric.MetricPushDelay(tms)
		pushQueue <- tmpPoint
	}

	return nil
}

func postToCollector(paramPoints []*dataobj.MetricValue) {
	// 按照时间戳分组发送
	tsPsMap := make(map[int64][]*dataobj.MetricValue)
	for _, p := range paramPoints {
		if _, exist := tsPsMap[p.Timestamp]; !exist {
			tsPsMap[p.Timestamp] = make([]*dataobj.MetricValue, 0)
		}

		tsPsMap[p.Timestamp] = append(tsPsMap[p.Timestamp], p)
	}

	var tsps tsPs
	for ts, ps := range tsPsMap {
		tsps = append(tsps, _tsPs{ts: ts, ps: ps})
	}
	sort.Sort(tsps)

	for _, ps := range tsps {
		core.Push(ps.ps)

		// 1000ms是经验值
		// 对于10G/小时的数据量+异步落盘的场景, 产生的结果友好一些
		time.Sleep(time.Millisecond * 1000)
	}
}

func getPrecision(num float64, degree int64) float64 {
	tmpFloat := num * float64(math.Pow10(int(degree)))
	tmpInt := int(tmpFloat + 0.5)
	return float64(tmpInt) / float64(math.Pow10(int(degree)))
}

type _tsPs struct {
	ts int64
	ps []*dataobj.MetricValue
}

type tsPs []_tsPs

func (tp tsPs) Len() int           { return len(tp) }
func (tp tsPs) Swap(i, j int)      { tp[i], tp[j] = tp[j], tp[i] }
func (tp tsPs) Less(i, j int) bool { return tp[i].ts < tp[j].ts }
