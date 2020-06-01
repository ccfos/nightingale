package worker

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/didi/nightingale/src/modules/collector/log/strategy"
	"github.com/didi/nightingale/src/modules/collector/stra"

	"github.com/toolkits/pkg/logger"
)

type callbackHandler func(int64, int64)

//单个worker对象
type Worker struct {
	FilePath  string
	Counter   int64
	LatestTms int64 //正在处理的单条日志时间
	Delay     int64 //时间戳乱序差值, 每个worker独立更新
	Close     chan struct{}
	Stream    chan string
	Mark      string //标记该worker信息，方便打log及上报自监控指标, 追查问题
	Analyzing bool   //标记当前Worker状态是否在分析中,还是空闲状态
	Callback  callbackHandler
}

//worker组
type WorkerGroup struct {
	WorkerNum          int
	LatestTms          int64 //日志文件最新处理的时间戳
	MaxDelay           int64 //日志文件存在的时间戳乱序最大差值
	ResetTms           int64 //maxDelay上次重置的时间
	Workers            []*Worker
	TimeFormatStrategy string
}

/*
 * filepath和stream依赖外部，其他的都自己创建
 */
func NewWorkerGroup(filePath string, stream chan string) *WorkerGroup {
	wokerNum := WorkerConfig.WorkerNum
	wg := &WorkerGroup{
		WorkerNum: wokerNum,
		Workers:   make([]*Worker, 0),
	}

	logger.Infof("new worker group, [file:%s][worker_num:%d]", filePath, wokerNum)

	for i := 0; i < wg.WorkerNum; i++ {
		mark := fmt.Sprintf("[worker][file:%s][num:%d][id:%d]", filePath, wokerNum, i)
		w := Worker{}
		w.Close = make(chan struct{})
		w.FilePath = filePath
		w.Stream = stream
		w.Mark = mark
		w.Analyzing = false
		w.Counter = 0
		w.LatestTms = 0
		w.Delay = 0
		w.Callback = wg.SetLatestTmsAndDelay
		wg.Workers = append(wg.Workers, &w)
	}

	return wg
}

func (wg WorkerGroup) GetLatestTmsAndDelay() (tms int64, delay int64) {
	return wg.LatestTms, wg.MaxDelay
}

func (wg *WorkerGroup) SetLatestTmsAndDelay(tms int64, delay int64) {
	latest := atomic.LoadInt64(&wg.LatestTms)

	if latest < tms {
		swapped := atomic.CompareAndSwapInt64(&wg.LatestTms, latest, tms)
		if swapped {
			logger.Debugf("[work group:%s][set latestTms:%d]", wg.Workers[0].Mark, tms)
		}
	}

	if delay == 0 {
		return
	}

	newest := atomic.LoadInt64(&wg.MaxDelay)
	if newest < delay {
		atomic.CompareAndSwapInt64(&wg.MaxDelay, newest, delay)
	}
}

func (wg *WorkerGroup) Start() {
	for _, worker := range wg.Workers {
		worker.Start()
	}
}

func (wg *WorkerGroup) Stop() {
	for _, worker := range wg.Workers {
		worker.Stop()
	}
}

func (wg *WorkerGroup) ResetMaxDelay() {
	// 默认1天重置一次
	ts := time.Now().Unix()
	if ts-wg.ResetTms > 86400 {
		wg.ResetTms = ts
		atomic.StoreInt64(&wg.MaxDelay, 0)
	}
}

func (w *Worker) Start() {
	go func() {
		w.Work()
	}()
}

func (w *Worker) Stop() {
	close(w.Close)
}

func (w *Worker) Work() {
	defer func() {
		if reason := recover(); reason != nil {
			logger.Infof("%s -- worker quit: panic reason: %v", w.Mark, reason)
		} else {
			logger.Infof("%s -- worker quit: normally", w.Mark)
		}
	}()
	logger.Infof("worker starting...[%s]", w.Mark)

	var anaCnt, anaSwp int64
	analysClose := make(chan int)

	go func() {
		for {
			//休眠10s
			select {
			case <-analysClose:
				return
			case <-time.After(time.Second * 10):
			}
			a := anaCnt
			//metric.MetricWorkerAnalysisNum(int(a - anaSwp))
			logger.Debugf("analysis %d line in last 10s", a-anaSwp)
			anaSwp = a
		}
	}()

	for {
		select {
		case line := <-w.Stream:
			w.Analyzing = true
			anaCnt = anaCnt + 1
			w.analysis(line)
			w.Analyzing = false
		case <-w.Close:
			analysClose <- 0
			return
		}

	}
}

//内部的分析方法
//轮全局的规则列表
//单次遍历
func (w *Worker) analysis(line string) {
	defer func() {
		if err := recover(); err != nil {
			logger.Infof("%s[analysis panic] : %v", w.Mark, err)
		}
	}()

	sts := strategy.GetAll()
	for _, strategy := range sts {
		if strategy.FilePath == w.FilePath && strategy.ParseSucc {
			analyspoint, err := w.producer(line, strategy)

			if err != nil {
				log := fmt.Sprintf("%s[producer error][sid:%d] : %v", w.Mark, strategy.ID, err)
				//sample_log.Error(log)
				logger.Error(log)
				continue
			} else {
				if analyspoint != nil {
					toCounter(analyspoint, w.Mark)
				}
			}
		}
	}
}

func (w *Worker) producer(line string, strategy *stra.Strategy) (*AnalysPoint, error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Errorf("%s[producer panic] : %v", w.Mark, err)
		}
	}()

	var reg *regexp.Regexp
	_, timeFormat := stra.GetPatAndTimeFormat(strategy.TimeFormat)

	reg = strategy.TimeReg

	t := reg.FindString(line)
	if len(t) <= 0 {
		return nil, fmt.Errorf("cannot get timestamp:[sname:%s][sid:%d][timeFormat:%v]", strategy.Name, strategy.ID, timeFormat)
	}

	// 如果没有年，需添加当前年
	// 需干掉内部的多于空格, 如Dec  7,有的有一个空格，有的有两个，这里统一替换成一个
	if timeFormat == "Jan 2 15:04:05" || timeFormat == "0102 15:04:05" {
		timeFormat = fmt.Sprintf("2006 %s", timeFormat)
		t = fmt.Sprintf("%d %s", time.Now().Year(), t)
		reg := regexp.MustCompile(`\s+`)
		rep := " "
		t = reg.ReplaceAllString(t, rep)
	}

	loc, err := time.LoadLocation("Local")
	tms, err := time.ParseInLocation(timeFormat, t, loc)
	if err != nil {
		return nil, err
	}

	tmsUnix := tms.Unix()
	if tmsUnix > time.Now().Unix() {
		logger.Debugf("%s[illegal timestamp][id:%d][tmsUnix:%d][current:%d]",
			w.Mark, strategy.ID, tmsUnix, time.Now().Unix())
		return nil, errors.New("illegal timestamp, greater than current")
	}

	// 更新worker的时间戳和乱序差值
	// 如有必要, 更新上层group的时间戳和乱序差值
	updateLatest := false
	delay := int64(0)
	if w.LatestTms < tmsUnix {
		updateLatest = true
		w.LatestTms = tmsUnix

	} else if w.LatestTms > tmsUnix {
		logger.Debugf("%s[timestamp disorder][id:%d][latest:%d][producing:%d]",
			w.Mark, strategy.ID, w.LatestTms, tmsUnix)

		delay = w.LatestTms - tmsUnix
	}
	if updateLatest || delay > 0 {
		w.Callback(tmsUnix, delay)
	}

	//处理用户正则
	var patternReg, excludeReg *regexp.Regexp
	var value float64
	patternReg = strategy.PatternReg
	if patternReg != nil {
		v := patternReg.FindStringSubmatch(line)
		var vString string
		if v != nil && len(v) != 0 {
			if len(v) > 1 {
				vString = v[1]
			} else {
				vString = ""
			}
			value, err = strconv.ParseFloat(vString, 64)
			if err != nil {
				value = math.NaN()
			}
		} else {
			//外边匹配err之后，要确保返回值不是nil再推送至counter
			//正则有表达式，没匹配到，直接返回
			return nil, nil
		}

	} else {
		value = math.NaN()
	}

	//处理exclude
	excludeReg = strategy.ExcludeReg
	if excludeReg != nil {
		v := excludeReg.FindStringSubmatch(line)
		if v != nil && len(v) != 0 {
			//匹配到exclude了，需要返回
			return nil, nil
		}
	}

	//处理tag 正则
	tag := map[string]string{}
	for tagk, tagv := range strategy.Tags {
		var regTag *regexp.Regexp
		regTag, ok := strategy.TagRegs[tagk]
		if !ok {
			logger.Errorf("%s[get tag reg error][sid:%d][tagk:%s][tagv:%s]", w.Mark, strategy.ID, tagk, tagv)
			return nil, nil
		}
		t := regTag.FindStringSubmatch(line)
		if t != nil && len(t) > 1 {
			tag[tagk] = t[1]
		} else {
			return nil, nil
		}
	}

	ret := &AnalysPoint{
		StrategyID: strategy.ID,
		Value:      value,
		Tms:        tms.Unix(),
		Tags:       tag,
	}

	return ret, nil
}

//将解析数据给counter
func toCounter(analyspoint *AnalysPoint, mark string) {
	if err := PushToCount(analyspoint); err != nil {
		logger.Errorf("%s push to counter error: %v", mark, err)
	}
}
