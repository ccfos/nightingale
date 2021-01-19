// Copyright 2017 Xiaomi, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/models"
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

func GetStra(sid int64) (*models.Stra, bool) {
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

	linkedList, exists := historyMap.Get(key)
	if exists {
		needJudge := linkedList.PushFrontAndMaintain(val, stra.AlertDur)
		if !needJudge {
			return
		}
	} else {
		NL := list.New()
		NL.PushFront(val)
		linkedList = &cache.SafeLinkedList{L: NL}
		historyMap.Set(key, linkedList)
	}

	historyData := linkedList.HistoryData()
	if len(historyData) == 0 {
		return
	}

	historyArr := []dataobj.History{}
	statusArr := []bool{}
	eventInfo := ""
	value := ""

	if len(stra.Exprs) == 1 {
		for _, expr := range stra.Exprs {
			history, info, lastValue, status := Judge(stra, expr, historyData, val, now)
			statusArr = append(statusArr, status)

			if value == "" {
				value = fmt.Sprintf("%s: %s", expr.Metric, lastValue)
			} else {
				value += fmt.Sprintf("; %s: %s", expr.Metric, lastValue)
			}

			historyArr = append(historyArr, history)
			eventInfo += info
		}
	} else { //与条件
		for _, expr := range stra.Exprs {
			respData, err := GetData(stra, expr, val, now)
			if err != nil {
				logger.Errorf("stra:%+v get query data err:%v", stra, err)
				return
			}

			if len(respData) != 1 {
				logger.Errorf("stra:%+v get query data respData:%v err", stra, respData)
				return
			}

			history, info, lastValue, status := Judge(stra, expr, dataobj.RRDData2HistoryData(respData[0].Values), val, now)

			statusArr = append(statusArr, status)
			if value == "" {
				value = fmt.Sprintf("%s: %s", expr.Metric, lastValue)
			} else {
				value += fmt.Sprintf("; %s: %s", expr.Metric, lastValue)
			}

			historyArr = append(historyArr, history)
			eventInfo += info
		}

	}

	bs, err := json.Marshal(historyArr)
	if err != nil {
		logger.Errorf("Marshal history:%+v err:%v", historyArr, err)
	}

	event := &dataobj.Event{
		ID:        fmt.Sprintf("s_%d_%s", stra.Id, val.PrimaryKey()),
		Etime:     now,
		Endpoint:  val.Endpoint,
		CurNid:    val.Nid,
		Info:      eventInfo,
		Detail:    string(bs),
		Value:     value,
		Partition: redi.Config.Prefix + "/event/p" + strconv.Itoa(stra.Priority),
		Sid:       stra.Id,
		Hashid:    getHashId(stra.Id, val),
	}

	sendEventIfNeed(statusArr, event, stra)
}

func Judge(stra *models.Stra, exp models.Exp, historyData []*dataobj.HistoryData, firstItem *dataobj.JudgeItem, now int64) (history dataobj.History, info string, lastValue string, status bool) {
	stats.Counter.Set("running", 1)

	var leftValue dataobj.JsonFloat
	if exp.Func == "nodata" {
		info = fmt.Sprintf(" %s (%s,%ds)", exp.Metric, exp.Func, stra.AlertDur)
	} else if exp.Func == "stddev" {
		info = fmt.Sprintf(" %s (%s,%ds) %v", exp.Metric, exp.Func, stra.AlertDur, exp.Params)
	} else if exp.Func == "happen" {
		info = fmt.Sprintf(" %s (%s,%ds) %v %s %v", exp.Metric, exp.Func, stra.AlertDur, exp.Params, exp.Eopt, exp.Threshold)
	} else {
		info = fmt.Sprintf(" %s(%s,%ds) %s %v", exp.Metric, exp.Func, stra.AlertDur, exp.Eopt, exp.Threshold)
	}

	leftValue, status = judgeItemWithStrategy(stra, historyData, exp, firstItem, now)

	lastValue = "null"
	if !math.IsNaN(float64(leftValue)) {
		lastValue = strconv.FormatFloat(float64(leftValue), 'f', -1, 64)
	}

	history = dataobj.History{
		Metric:      exp.Metric,
		Tags:        firstItem.TagsMap,
		Granularity: int(firstItem.Step),
		Points:      historyData,
	}
	return
}

func judgeItemWithStrategy(stra *models.Stra, historyData []*dataobj.HistoryData, exp models.Exp, firstItem *dataobj.JudgeItem, now int64) (leftValue dataobj.JsonFloat, isTriggered bool) {
	straFunc := exp.Func

	var straParam []interface{}

	straParam = append(straParam, stra.AlertDur)

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

		respItems, err := GetData(stra, exp, firstItem, now-int64(exp.Params[0]))
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

func GetData(stra *models.Stra, exp models.Exp, firstItem *dataobj.JudgeItem, now int64) ([]*dataobj.TsdbQueryResponse, error) {
	var reqs []*dataobj.QueryData
	var respData []*dataobj.TsdbQueryResponse
	var err error
	if firstItem.Tags != "" && len(firstItem.TagsMap) == 0 {
		firstItem.TagsMap = str.DictedTagstring(firstItem.Tags)
	}

	//多查一些数据，防止由于查询不到最新点，导致点数不够
	start := now - int64(stra.AlertDur) - int64(firstItem.Step) - 60

	queryParam, err := query.NewQueryRequest(firstItem.Nid, firstItem.Endpoint, exp.Metric, firstItem.TagsMap, firstItem.Step, start, now)
	if err != nil {
		return respData, err
	}

	reqs = append(reqs, queryParam)

	if len(reqs) == 0 {
		return respData, err
	}

	respData = query.Query(reqs, stra, exp.Func)

	return respData, err
}

func GetReqs(stra *models.Stra, metric string, nids, endpoints []string, now int64) []*dataobj.QueryData {
	var reqs []*dataobj.QueryData
	stats.Counter.Set("query.index", 1)

	req := &query.IndexReq{
		Nids:      nids,
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
		if len(index.Tags) == 0 {
			hash := getHash(index, "")
			s := cache.Series{
				Nid:      index.Nid,
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
				hash := getHash(index, tag)
				s := cache.Series{
					Nid:      index.Nid,
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
			Counters:   []string{counter},
			Step:       series.Step,
			DsType:     series.Dstype,
		}

		if series.Nid != "" {
			queryParam.Nids = []string{series.Nid}
		} else {
			queryParam.Endpoints = []string{series.Endpoint}
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
			Counters:   []string{counter},
			Step:       series.Step,
			DsType:     series.Dstype,
		}

		if series.Nid != "" {
			queryParam.Nids = []string{series.Nid}
		} else {
			queryParam.Endpoints = []string{series.Endpoint}
		}

		reqs = append(reqs, queryParam)
	}

	return reqs
}

func sendEventIfNeed(status []bool, event *dataobj.Event, stra *models.Stra) {
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

func getHash(idx query.IndexData, tag string) string {
	if idx.Nid != "" {
		return str.MD5(idx.Nid, idx.Metric, tag)
	}

	return str.MD5(idx.Endpoint, idx.Metric, tag)
}
