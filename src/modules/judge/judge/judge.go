package judge

import (
	"bytes"
	"container/list"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/judge/backend/query"
	"github.com/didi/nightingale/src/modules/judge/backend/redi"
	"github.com/didi/nightingale/src/modules/judge/cache"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/spaolacci/murmur3"
	"github.com/toolkits/pkg/logger"
)

var (
	bufferPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

	EVENT_ALERT   = "alert"
	EVENT_RECOVER = "recovery"
)

func GetStra(sid int64) (*model.Stra, bool) {
	if stra, exists := cache.Strategy.Get(sid); exists {
		return stra, exists
	}
	if stra, exists := cache.NodataStra.Get(sid); exists {
		return stra, exists
	}
	return nil, false
}

func ToJudge(historyMap *cache.JudgeItemMap, key string, val *dataobj.JudgeItem, now int64) {
	stra, exists := GetStra(val.Sid)
	if !exists {
		stats.Counter.Set("point.miss", 1)
		return
	}

	needCount := stra.AlertDur / int(val.Step)
	if needCount < 1 {
		needCount = 1
	}
	linkedList, exists := historyMap.Get(key)
	if exists {
		needJudge := linkedList.PushFrontAndMaintain(val, needCount)
		if !needJudge {
			return
		}
	} else {
		NL := list.New()
		NL.PushFront(val)
		linkedList = &cache.SafeLinkedList{L: NL}
		historyMap.Set(key, linkedList)
	}

	historyData, isEnough := linkedList.HistoryData(needCount)
	if !isEnough {
		return
	}
	history := []dataobj.History{}

	Judge(stra, stra.Exprs, historyData, val, now, history, "", "", "", []bool{})
}

func Judge(stra *model.Stra, exps []model.Exp, historyData []*dataobj.HistoryData, firstItem *dataobj.JudgeItem, now int64, history []dataobj.History, info string, value string, extra string, status []bool) {
	stats.Counter.Set("running", 1)

	if len(exps) < 1 {
		stats.Counter.Set("stra.illegal", 1)
		logger.Warningf("stra:%+v exp is null", stra)
		return
	}
	exp := exps[0]
	var leftValue dataobj.JsonFloat
	var isTriggered bool

	if exp.Func == "nodata" {
		info += fmt.Sprintf(" %s (%s,%ds)", exp.Metric, exp.Func, stra.AlertDur)
	} else {
		info += fmt.Sprintf(" %s(%s,%ds) %s %v", exp.Metric, exp.Func, stra.AlertDur, exp.Eopt, exp.Threshold)
	}

	h := dataobj.History{
		Metric:      exp.Metric,
		Tags:        firstItem.TagsMap,
		Granularity: int(firstItem.Step),
		Points:      historyData,
	}
	history = append(history, h)

	defer func() {
		if len(exps) == 1 {
			bs, err := json.Marshal(history)
			if err != nil {
				logger.Errorf("Marshal history:%+v err:%v", history, err)
			}
			event := &dataobj.Event{
				ID:        fmt.Sprintf("s_%d_%s", stra.Id, firstItem.PrimaryKey()),
				Etime:     now,
				Endpoint:  firstItem.Endpoint,
				Info:      info,
				Detail:    string(bs),
				Value:     value,
				Partition: redi.Config.Prefix + "/event/p" + strconv.Itoa(stra.Priority),
				Sid:       stra.Id,
				Hashid:    getHashId(stra.Id, firstItem),
			}

			sendEventIfNeed(historyData, status, event, stra)
		}
	}()

	leftValue, isTriggered = judgeItemWithStrategy(stra, historyData, exps[0], firstItem, now)
	lastValue := "null"
	if !math.IsNaN(float64(leftValue)) {
		lastValue = strconv.FormatFloat(float64(leftValue), 'f', -1, 64)
	}
	if value == "" {
		value = fmt.Sprintf("%s: %s", exp.Metric, lastValue)
	} else {
		value += fmt.Sprintf("; %s: %s", exp.Metric, lastValue)
	}
	status = append(status, isTriggered)

	//与条件情况下执行
	if len(exps) > 1 {
		if exps[1].Func == "nodata" { //nodata重新查询索引来进行告警判断
			respData, err := GetData(stra, exps[1], firstItem, now, false)
			if err != nil {
				logger.Errorf("stra:%v get query data err:%v", stra, err)

				judgeItem := &dataobj.JudgeItem{
					Endpoint: firstItem.Endpoint,
					Metric:   stra.Exprs[0].Metric,
					Tags:     "",
					DsType:   "GAUGE",
				}
				Judge(stra, exps[1:], []*dataobj.HistoryData{}, judgeItem, now, history, info, value, extra, status)
				return
			}

			for i := range respData {
				firstItem.Endpoint = respData[i].Endpoint
				firstItem.Tags = getTags(respData[i].Counter)
				firstItem.Step = respData[i].Step
				Judge(stra, exps[1:], dataobj.RRDData2HistoryData(respData[i].Values), firstItem, now, history, info, value, extra, status)
			}

		} else {
			var respData []*dataobj.TsdbQueryResponse
			var err error
			if firstItem.Step != 0 { //上报点的逻辑会走到这里，使用第一个exp上报点的索引进行告警判断
				respData, err = GetData(stra, exps[1], firstItem, now, true)
			} else { //上一个规则是nodata没有获取到索引数据，重新获取索引做计算
				respData, err = GetData(stra, exps[1], firstItem, now, false)
			}
			if err != nil {
				logger.Errorf("stra:%+v get query data err:%v", stra, err)
				return
			}
			for i := range respData {
				firstItem.Endpoint = respData[i].Endpoint
				firstItem.Tags = getTags(respData[i].Counter)
				firstItem.Step = respData[i].Step
				Judge(stra, exps[1:], dataobj.RRDData2HistoryData(respData[i].Values), firstItem, now, history, info, value, extra, status)
			}
		}
	}
}

func judgeItemWithStrategy(stra *model.Stra, historyData []*dataobj.HistoryData, exp model.Exp, firstItem *dataobj.JudgeItem, now int64) (leftValue dataobj.JsonFloat, isTriggered bool) {
	straFunc := exp.Func

	var straParam []interface{}
	if firstItem.Step == 0 {
		logger.Errorf("wrong step:%+v", firstItem)
		return
	}

	limit := stra.AlertDur / firstItem.Step
	if limit <= 0 {
		limit = 1
	}

	straParam = append(straParam, limit)

	switch straFunc {
	case "happen", "stddev":
		if len(exp.Params) < 1 {
			logger.Errorf("stra:%d exp:%+v stra param is null", stra.Id, exp)
			return
		}
		straParam = append(straParam, exp.Params[0])
	case "c_avg", "c_avg_abs", "c_avg_rate", "c_avg_rate_abs":
		if len(exp.Params) < 1 {
			logger.Errorf("stra:%d exp:%+v stra param is null", stra.Id, exp)
			return
		}

		if stra.AlertDur < 6*firstItem.Step {
			//查询之前的数据会被归档，保证能查到一个数据点
			stra.AlertDur = 7 * firstItem.Step
		}

		respItems, err := GetData(stra, exp, firstItem, now-int64(exp.Params[0]), true)
		if err != nil {
			logger.Errorf("stra:%v %+v get compare data err:%v", stra.Id, exp, err)
			return
		}

		if len(respItems) != 1 || len(respItems[0].Values) < 1 {
			logger.Errorf("stra:%d %+v get compare data err, respItems:%v", stra.Id, exp, respItems)
			return
		}

		var sum float64
		data := respItems[0]
		for i := range data.Values {
			sum += float64(data.Values[i].Value)
		}

		//环比数据的平均值
		straParam = append(straParam, sum/float64(len(data.Values)))
	}

	fn, err := ParseFuncFromString(straFunc, straParam, exp.Eopt, exp.Threshold)
	if err != nil {
		logger.Errorf("stra:%d %+v parse func fail: %v", stra.Id, exp, err)
		return
	}

	return fn.Compute(historyData)
}

func GetData(stra *model.Stra, exp model.Exp, firstItem *dataobj.JudgeItem, now int64, sameTag bool) ([]*dataobj.TsdbQueryResponse, error) {
	var reqs []*dataobj.QueryData
	var respData []*dataobj.TsdbQueryResponse
	var err error
	if sameTag { //与条件要求是相同tag的场景，不需要查询索引
		if firstItem.Tags != "" && len(firstItem.TagsMap) == 0 {
			firstItem.TagsMap = str.DictedTagstring(firstItem.Tags)
		}
		//+1 防止由于查询不到最新点，导致点数不够
		start := now - int64(stra.AlertDur) - int64(firstItem.Step) + 1

		queryParam, err := query.NewQueryRequest(firstItem.Endpoint, exp.Metric, firstItem.TagsMap, firstItem.Step, start, now)
		if err != nil {
			return respData, err
		}

		reqs = append(reqs, queryParam)
	} else if firstItem != nil { //点驱动告警策略的场景
		reqs = GetReqs(stra, exp.Metric, []string{firstItem.Endpoint}, now)
	} else { //nodata的场景
		reqs = GetReqs(stra, exp.Metric, stra.Endpoints, now)
	}

	if len(reqs) == 0 {
		return respData, err
	}

	respData = query.Query(reqs, stra.Id, exp.Func)

	if len(respData) < 1 {
		stats.Counter.Set("get.data.null", 1)
		err = fmt.Errorf("get query data is null")
	}
	return respData, err
}

func GetReqs(stra *model.Stra, metric string, endpoints []string, now int64) []*dataobj.QueryData {
	var reqs []*dataobj.QueryData
	stats.Counter.Set("query.index", 1)

	req := &query.IndexReq{
		Endpoints: endpoints,
		Metric:    metric,
	}
	for _, tag := range stra.Tags {
		if tag.Topt == "=" {
			req.Include = append(req.Include, query.XCludeStruct{
				Tagk: tag.Tkey,
				Tagv: tag.Tval,
			})
		} else if tag.Topt == "!=" {
			req.Exclude = append(req.Exclude, query.XCludeStruct{
				Tagk: tag.Tkey,
				Tagv: tag.Tval,
			})
		}
	}

	indexsData, err := query.Xclude(req)
	if err != nil {
		stats.Counter.Set("query.index.err", 1)
		logger.Warning("query index err:", err)
	}

	lostSeries := []cache.Series{}
	for _, index := range indexsData {
		if index.Step == 0 {
			//没有查到索引的 endpoint+metric 也要记录，给nodata处理
			s := cache.Series{
				Endpoint: index.Endpoint,
				Metric:   index.Metric,
				Tag:      "",
				Step:     10,
				Dstype:   "GAUGE",
				TS:       now,
			}
			lostSeries = append(lostSeries, s)
		} else {
			if len(index.Tags) == 0 {
				hash := str.MD5(index.Endpoint, index.Metric, "")
				s := cache.Series{
					Endpoint: index.Endpoint,
					Metric:   index.Metric,
					Tag:      "",
					Step:     index.Step,
					Dstype:   index.Dstype,
					TS:       now,
				}
				cache.SeriesMap.Set(stra.Id, hash, s)
			} else {
				for _, tag := range index.Tags {
					hash := str.MD5(index.Endpoint, index.Metric, tag)
					s := cache.Series{
						Endpoint: index.Endpoint,
						Metric:   index.Metric,
						Tag:      tag,
						Step:     index.Step,
						Dstype:   index.Dstype,
						TS:       now,
					}
					cache.SeriesMap.Set(stra.Id, hash, s)
				}

			}
		}
	}

	seriess := cache.SeriesMap.Get(stra.Id)
	if len(seriess) == 0 && err != nil {
		return reqs
	}

	step := 0
	if len(seriess) > 1 {
		step = seriess[0].Step
	}

	//防止由于查询不到最新点，导致点数不够
	start := now - int64(stra.AlertDur) - int64(step) + 1
	for _, series := range seriess {
		counter := series.Metric
		if series.Tag != "" {
			counter += "/" + series.Tag
		}
		queryParam := &dataobj.QueryData{
			Start:      start,
			End:        now,
			ConsolFunc: "AVERAGE", // 硬编码
			Endpoints:  []string{series.Endpoint},
			Counters:   []string{counter},
			Step:       series.Step,
			DsType:     series.Dstype,
		}
		reqs = append(reqs, queryParam)
	}

	for _, series := range lostSeries {
		counter := series.Metric
		if series.Tag != "" {
			counter += "/" + series.Tag
		}
		queryParam := &dataobj.QueryData{
			Start:      start,
			End:        now,
			ConsolFunc: "AVERAGE", // 硬编码
			Endpoints:  []string{series.Endpoint},
			Counters:   []string{counter},
			Step:       series.Step,
			DsType:     series.Dstype,
		}
		reqs = append(reqs, queryParam)
	}

	return reqs
}

func sendEventIfNeed(historyData []*dataobj.HistoryData, status []bool, event *dataobj.Event, stra *model.Stra) {
	isTriggered := true
	for _, s := range status {
		isTriggered = isTriggered && s
	}
	now := time.Now().Unix()
	lastEvent, exists := cache.LastEvents.Get(event.ID)
	if isTriggered {
		event.EventType = EVENT_ALERT
		if !exists || lastEvent.EventType[0] == 'r' {
			stats.Counter.Set("event.alert", 1)
			sendEvent(event)
			return
		}

		if now-lastEvent.Etime < int64(stra.AlertDur) {
			//距离上次告警的时间小于告警统计周期，不再进行告警判断
			return
		}

		stats.Counter.Set("event.alert", 1)
		sendEvent(event)
	} else {
		// 如果LastEvent是Problem，报OK，否则啥都不做
		if exists && lastEvent.EventType[0] == 'a' {
			// 如果配置了留观时长，则距离上一次故障时间要大于等于recoveryDur，才产生恢复事件
			if now-lastEvent.Etime < int64(stra.RecoveryDur) {
				return
			}

			event.EventType = EVENT_RECOVER
			sendEvent(event)
			stats.Counter.Set("event.recover", 1)
		}
	}
}

func sendEvent(event *dataobj.Event) {
	// update last event
	cache.LastEvents.Set(event.ID, event)

	err := redi.Push(event)
	if err != nil {
		stats.Counter.Set("redis.push.failed", 1)
		logger.Errorf("push event:%v err:%v", event, err)
	}
}

func getHashId(sid int64, item *dataobj.JudgeItem) uint64 {
	pk := bufferPool.Get().(*bytes.Buffer)
	pk.Reset()
	defer bufferPool.Put(pk)

	pk.WriteString(strconv.FormatInt(sid, 16))
	pk.WriteByte('/')
	pk.WriteString(item.Metric)
	pk.WriteByte('/')
	pk.WriteString(item.Endpoint)
	pk.WriteByte('/')
	pk.WriteString(item.Tags)
	pk.WriteByte('/')

	hashid := murmur3.Sum64(pk.Bytes())

	//因为xorm不支持uint64，为解决数据溢出的问题，此处将hashid转化为60位
	//具体细节：将64位值 高4位与低60位进行异或操作
	return (hashid >> 60) ^ (hashid & 0xFFFFFFFFFFFFFFF)
}

func getTags(counter string) (tags string) {
	idx := strings.IndexAny(counter, "/")
	if idx == -1 {
		return ""
	}
	return counter[idx+1:]
}
