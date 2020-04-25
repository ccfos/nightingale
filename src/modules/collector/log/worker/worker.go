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

type WorkerGroup struct {
	WorkerNum          int   // worker 数量
	LatestTms          int64 // 日志文件最新处理的时间戳
	MaxDelay           int64 // 日志文件存在的时间戳乱序最大差值
	ResetTms           int64 // maxDelay 上次重置的时间
	Workers            []*Worker
	TimeFormatStrategy string
}

func NewWorkerGroup(filePath string, stream chan string) *WorkerGroup {
	workerNum := WorkerConfig.WorkerNum
	wg := &WorkerGroup{
		WorkerNum: workerNum,
		Workers:   make([]*Worker, 0),
	}

	logger.Infof("new worker group, file:[%s]; worker_num:[%d]", filePath, workerNum)
	// filepath 和 stream 依赖外部，其他的都自己创建
	for i := 0; i < wg.WorkerNum; i++ {
		mark := fmt.Sprintf("[worker][file:%s][num:%d][id:%d]", filePath, workerNum, i)
		w := Worker{
			FilePath: filePath,
			Mark:     mark,
			Close:    make(chan struct{}),
			Stream:   stream,
			Callback: wg.SetLatestTmsAndDelay,
		}
		wg.Workers = append(wg.Workers, &w)
	}

	return wg
}

func (wg WorkerGroup) GetLatestTmsAndDelay() (tms, delay int64) {
	return wg.LatestTms, wg.MaxDelay
}

func (wg *WorkerGroup) SetLatestTmsAndDelay(tms, delay int64) {
	latest := atomic.LoadInt64(&wg.LatestTms)

	if latest < tms {
		swapped := atomic.CompareAndSwapInt64(&wg.LatestTms, latest, tms)
		if swapped {
			logger.Debugf("work group:[%s]; latestTms:[%d]", wg.Workers[0].Mark, tms)
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

type Worker struct {
	FilePath  string
	Counter   int64
	LatestTms int64  //正在处理的单条日志时间
	Delay     int64  //时间戳乱序差值, 每个worker独立更新
	Mark      string //标记该worker信息，方便打log及上报自监控指标, 追查问题
	Analyzing bool   //标记当前Worker状态是否在分析中,还是空闲状态
	Close     chan struct{}
	Stream    chan string
	Callback  callbackHandler
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

	var cnt, swp int64
	done := make(chan int)

	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Second * 10):
			}
			tmp := cnt
			//metric.MetricWorkerAnalysisNum(int(a - anaSwp))
			logger.Debugf("analysis %d line in last 10s", tmp-swp)
			swp = tmp
		}
	}()

	for {
		select {
		case line := <-w.Stream:
			w.Analyzing = true
			cnt = cnt + 1
			w.analyze(line)
			w.Analyzing = false
		case <-w.Close:
			done <- 0
			return
		}
	}
}

func (w *Worker) analyze(line string) {
	defer func() {
		if err := recover(); err != nil {
			logger.Infof("%s[analysis panic]: %v", w.Mark, err)
		}
	}()

	stras := strategy.GetAll()
	for _, s := range stras {
		if s.FilePath == w.FilePath && s.ParseSucc {
			points, err := w.producer(line, s)

			if err != nil {
				logger.Errorf("%s: sid:[%d]; producer error: %v", w.Mark, s.ID, err)
				continue
			}
			if points != nil {
				toCounter(points, w.Mark)
			}
		}
	}
}

func (w *Worker) producer(line string, strategy *stra.Strategy) (*AnalysPoint, error) {
	defer func() {
		if err := recover(); err != nil {
			logger.Errorf("%s[producer panic]: %v", w.Mark, err)
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
	// 需干掉内部的多于空格, 如`Dec  7`，有的有一个空格，有的有两个，这里统一替换成一个
	if timeFormat == "Jan 2 15:04:05" || timeFormat == "0102 15:04:05" {
		timeFormat = fmt.Sprintf("2006 %s", timeFormat)
		t = fmt.Sprintf("%d %s", time.Now().Year(), t)
		reg := regexp.MustCompile(`\s+`)
		rep := " "
		t = reg.ReplaceAllString(t, rep)
	}

	// [风险]统一使用东八区
	// loc, err := time.LoadLocation("Asia/Shanghai")
	loc := time.FixedZone("CST", 8*3600)
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
		if len(v) != 0 {
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
		if len(v) != 0 {
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
		if len(t) > 1 {
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

// toCounter 将解析数据给 counter
func toCounter(points *AnalysPoint, mark string) {
	if err := PushToCount(points); err != nil {
		logger.Errorf("%s push to counter error: %v", mark, err)
	}
}
