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
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/vos"
)

var (
	bufferPool = sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}

	EVENT_ALERT   = "alert"
	EVENT_RECOVER = "recovery"
)

func Send(points []*vos.MetricPoint) {
	for i := range points {
		alertRules := getMatchAlertRules(points[i])

		rulesCount := len(alertRules)
		if rulesCount == 0 {
			// 这个监控数据没有关联任何告警策略，省事了不用处理
			continue
		}
		logger.Debugf("[point_match_alertRules][point:%+v][alertRuleNum:%+v]", points[i], rulesCount)
		// 不同的告警规则，alert_duration字段大小不同，找到最大的，按照最大的值来缓存历史数据
		var maxAliveDuration = 0
		for j := range alertRules {
			if maxAliveDuration < alertRules[j].AlertDuration {
				maxAliveDuration = alertRules[j].AlertDuration
			}
		}
		if len(points[i].PK) < 2 {
			logger.Debugf("[point:%+v] len(pk)<2", points[i])
			continue
		}

		ll := PointCaches[points[i].PK[0:2]].PutPoint(points[i], int64(maxAliveDuration))

		for j := range alertRules {
			go ToJudge(ll, alertRules[j], points[i])
		}
	}
}

func getMatchAlertRules(point *vos.MetricPoint) []*models.AlertRule {
	alertRules := cache.AlertRulesByMetric.GetBy(point.Metric)
	matchRules := make([]*models.AlertRule, 0, len(alertRules))

	for i := range alertRules {
		if alertRules[i].Type == models.PULL {
			continue
		}

		if matchAlertRule(point, alertRules[i]) {
			matchRules = append(matchRules, alertRules[i])
		}
	}

	return matchRules
}

func matchAlertRule(item *vos.MetricPoint, alertRule *models.AlertRule) bool {
	//TODO 过滤方式待优化
	for _, filter := range alertRule.PushExpr.ResFilters {
		if !valueMatch(item.Ident, filter.Func, filter.Params) {
			return false
		}
	}

	for _, filter := range alertRule.PushExpr.TagFilters {
		value, exists := item.TagsMap[filter.Key]
		if !exists {
			return false
		}

		if !valueMatch(value, filter.Func, filter.Params) {
			return false
		}
	}

	return true
}

func valueMatch(value, f string, params []string) bool {
	switch f {

	case "InClasspath":
		for i := range params {
			if cache.ResClasspath.Exists(value, params[i]) {
				return true
			}
		}
		return false
	case "NotInClasspath":
		for i := range params {
			if cache.ResClasspath.Exists(value, params[i]) {
				return false
			}
		}
		return true
	case "InClasspathPrefix":
		classpaths := cache.ResClasspath.GetValues(value)
		for _, classpath := range classpaths {
			for i := range params {
				if strings.HasPrefix(classpath, params[i]) {
					return true
				}
			}
		}
		return false
	case "NotInClasspathPrefix":
		classpaths := cache.ResClasspath.GetValues(value)
		for _, classpath := range classpaths {
			for i := range params {
				if strings.HasPrefix(classpath, params[i]) {
					return false
				}
			}
		}
		return true
	case "InList":
		for i := range params {
			if value == params[i] {
				return true
			}
		}
		return false
	case "NotInList":
		for i := range params {
			if value == params[i] {
				return false
			}
		}
		return true
	case "InResourceList":
		for i := range params {
			if value == params[i] {
				return true
			}
		}
		return false
	case "NotInResourceList":
		for i := range params {
			if value == params[i] {
				return false
			}
		}
		return true
	case "HasPrefixString":
		for i := range params {
			if strings.HasPrefix(value, params[i]) {
				return true
			}
		}
		return false
	case "NoPrefixString":
		for i := range params {
			if strings.HasPrefix(value, params[i]) {
				return false
			}
		}
		return true
	case "HasSuffixString":
		for i := range params {
			if strings.HasSuffix(value, params[i]) {
				return true
			}
		}
		return false
	case "NoSuffixString":
		for i := range params {
			if strings.HasSuffix(value, params[i]) {
				return false
			}
		}
		return true
	case "ContainsString":
		for i := range params {
			if strings.Contains(value, params[i]) {
				return true
			}
		}
		return false
	case "NotContainsString":
		for i := range params {
			if strings.Contains(value, params[i]) {
				return false
			}
		}
		return true
	case "MatchRegexp":
		for i := range params {
			r, _ := regexp.Compile(params[i])
			if r.MatchString(value) {
				return true
			}
		}
		return false
	case "NotMatchRegexp":
		for i := range params {
			r, _ := regexp.Compile(params[i])
			if r.MatchString(value) {
				return false
			}
		}
		return true
	}

	return false
}

func ToJudge(linkedList *SafeLinkedList, stra *models.AlertRule, val *vos.MetricPoint) {
	logger.Debugf("[ToJudge.start][stra:%+v][val:%+v]", stra, val)
	now := val.Time

	hps := linkedList.HistoryPoints(now - int64(stra.AlertDuration))
	if len(hps) == 0 {
		return
	}

	historyArr := []vos.HistoryPoints{}
	statusArr := []bool{}
	eventInfo := ""
	value := ""

	if len(stra.PushExpr.Exps) == 1 {
		for _, expr := range stra.PushExpr.Exps {
			history, info, lastValue, status := Judge(stra, expr, hps, val, now)
			statusArr = append(statusArr, status)

			if value == "" {
				value = fmt.Sprintf("%s: %s", expr.Metric, lastValue)
			} else {
				value += fmt.Sprintf("; %s: %s", expr.Metric, lastValue)
			}

			historyArr = append(historyArr, history)
			eventInfo += info
		}
	} else { //多个条件
		for _, expr := range stra.PushExpr.Exps {

			respData, err := GetData(stra, expr, val, now)
			if err != nil {
				logger.Errorf("stra:%+v get query data err:%v", stra, err)
				return
			}
			if len(respData) <= 0 {
				logger.Errorf("stra:%+v get query data respData:%v err", stra, respData)
				return
			}

			history, info, lastValue, status := Judge(stra, expr, respData, val, now)

			statusArr = append(statusArr, status)
			if value == "" {
				value = fmt.Sprintf("%s: %s", expr.Metric, lastValue)
			} else {
				value += fmt.Sprintf("; %s: %s", expr.Metric, lastValue)
			}

			historyArr = append(historyArr, history)
			if eventInfo == "" {
				eventInfo = info
			} else {
				if stra.PushExpr.TogetherOrAny == 0 {
					eventInfo += fmt.Sprintf(" & %s", info)
				} else if stra.PushExpr.TogetherOrAny == 1 {
					eventInfo += fmt.Sprintf(" || %s", info)
				}

			}

		}

	}

	bs, err := json.Marshal(historyArr)
	if err != nil {
		logger.Errorf("Marshal history:%+v err:%v", historyArr, err)
	}

	event := &models.AlertEvent{
		RuleId:             stra.Id,
		RuleName:           stra.Name,
		RuleNote:           stra.Note,
		HashId:             str.MD5(fmt.Sprintf("%d_%s", stra.Id, val.PK)),
		ResIdent:           val.Ident,
		Priority:           stra.Priority,
		HistoryPoints:      bs,
		TriggerTime:        now,
		Values:             value,
		NotifyChannels:     stra.NotifyChannels,
		NotifyGroups:       stra.NotifyGroups,
		NotifyUsers:        stra.NotifyUsers,
		RunbookUrl:         stra.RunbookUrl,
		ReadableExpression: eventInfo,
		TagMap:             val.TagsMap,
	}
	logger.Debugf("[ToJudge.event.create][statusArr:%v][type=push][stra:%+v][val:%+v][event:%+v]", statusArr, stra, val, event)
	sendEventIfNeed(statusArr, event, stra)
}

func Judge(stra *models.AlertRule, exp models.Exp, historyData []*vos.HPoint, firstItem *vos.MetricPoint, now int64) (history vos.HistoryPoints, info string, lastValue string, status bool) {

	var leftValue vos.JsonFloat
	if exp.Func == "stddev" {
		info = fmt.Sprintf(" %s (%s,%ds) %v", exp.Metric, exp.Func, stra.AlertDuration, exp.Params)
	} else if exp.Func == "happen" {
		info = fmt.Sprintf(" %s (%s,%ds) %v %s %v", exp.Metric, exp.Func, stra.AlertDuration, exp.Params, exp.Optr, exp.Threshold)
	} else {
		info = fmt.Sprintf(" %s(%s,%ds) %s %v", exp.Metric, exp.Func, stra.AlertDuration, exp.Optr, exp.Threshold)
	}

	leftValue, status = judgeItemWithStrategy(stra, historyData, exp, firstItem, now)

	lastValue = "null"
	if !math.IsNaN(float64(leftValue)) {
		lastValue = strconv.FormatFloat(float64(leftValue), 'f', -1, 64)
	}

	history = vos.HistoryPoints{
		Metric: exp.Metric,
		Tags:   firstItem.TagsMap,
		Points: historyData,
	}
	return
}

func judgeItemWithStrategy(stra *models.AlertRule, historyData []*vos.HPoint, exp models.Exp, firstItem *vos.MetricPoint, now int64) (leftValue vos.JsonFloat, isTriggered bool) {
	straFunc := exp.Func

	var straParam []interface{}

	straParam = append(straParam, stra.AlertDuration)

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

		hisD, err := GetData(stra, exp, firstItem, now-int64(exp.Params[0]))
		if err != nil {
			logger.Errorf("stra:%v %+v get compare data err:%v", stra.Id, exp, err)
			return
		}

		if len(hisD) != 1 {
			logger.Errorf("stra:%d %+v get compare data err, respItems:%v", stra.Id, exp, hisD)
			return
		}

		var sum float64
		for _, i := range hisD {
			sum += float64(i.Value)
		}

		//环比数据的平均值
		straParam = append(straParam, sum/float64(len(hisD)))
	}

	fn, err := ParseFuncFromString(straFunc, straParam, exp.Optr, exp.Threshold)
	if err != nil {
		logger.Errorf("stra:%d %+v parse func fail: %v", stra.Id, exp, err)
		return
	}

	return fn.Compute(historyData)
}

func GetData(stra *models.AlertRule, exp models.Exp, firstItem *vos.MetricPoint, now int64) ([]*vos.HPoint, error) {
	var respData []*vos.HPoint
	var err error

	//多查一些数据，防止由于查询不到最新点，导致点数不够
	start := now - int64(stra.AlertDuration) - 2
	// 这里的参数肯定只有一个
	queryParam, err := NewQueryRequest(firstItem.Ident, exp.Metric, firstItem.TagsMap, start, now)

	if err != nil {
		return respData, err
	}
	respData = Query(queryParam)
	logger.Debugf("[exp:%+v][queryParam:%+v][respData:%+v]\n", exp, queryParam, respData)
	return respData, err
}

// 虽然最近的数据确实产生了事件(产生事件很频繁)，但是未必一定要发送，只有告警/恢复状态发生变化的时候才需发送
func sendEventIfNeed(status []bool, event *models.AlertEvent, stra *models.AlertRule) {
	if stra.Name == "system_cpu_util_test" {
		logger.Errorf("status:%+v", status)
		logger.Errorf("event:%+v", event)
		logger.Errorf("stra:%+v", stra)
	}

	isTriggered := true

	if stra.Type == 0 {
		// 只判断push型的
		switch stra.PushExpr.TogetherOrAny {

		case 0:
			// 全部触发
			for _, s := range status {
				isTriggered = isTriggered && s
			}

		case 1:
			// 任意一个触发
			isTriggered = false
			for _, s := range status {
				if s == true {
					isTriggered = true
					break
				}
			}

		}
	}

	now := time.Now().Unix()
	lastEvent, exists := LastEvents.Get(event.HashId)

	switch event.IsPromePull {
	case 0:
		//	push型的 && 与条件型的
		if exists && lastEvent.IsPromePull == 1 {
			// 之前内存中的事件是pull型的，先清空内存中的事件
			LastEvents.Del(event.HashId)
		}

		if isTriggered {
			// 新告警或者上次是恢复事件，都需要立即发送
			if !exists || lastEvent.IsRecov() {
				event.MarkAlert()
				SendEvent(event)
			}
		} else {
			// 上次是告警事件，现在恢复了，自然需要通知
			if exists && lastEvent.IsAlert() {
				event.MarkRecov()
				SendEvent(event)
			}
		}
	case 1:
		// pull型的，产生的事件一定是触发了阈值的，即这个case里不存在recovery的场景，recovery的场景用resolve_timeout的cron来处理
		if exists && lastEvent.IsPromePull == 0 {
			// 之前内存中的事件是push型的，先清空内存中的事件
			LastEvents.Del(event.HashId)
		}

		// 1. 第一次来，并且AlertDuration=0，直接发送
		// 2. 触发累计到AlertDuration时长后触发一条
		if !exists {
			// 这是个新事件，之前未曾产生过的
			if stra.AlertDuration == 0 {
				// 代表prometheus rule for 配置为0，直接发送
				event.LastSend = true
				event.MarkAlert()
				SendEvent(event)
			} else {
				// 只有一条事件，显然无法满足for AlertDuration的时间，放到内存里等待
				LastEvents.Set(event.HashId, event)
			}
			return
		}

		// 内存里有事件，虽然AlertDuration是0但是上次没有发过(可能是中间调整过AlertDuration，比如从某个大于0的值调整为0)
		if stra.AlertDuration == 0 && !lastEvent.LastSend {
			event.LastSend = true
			event.MarkAlert()
			SendEvent(event)
			return
		}

		// 内存里有事件，AlertDuration也是大于0的，需要判断Prometheus里的for的逻辑
		if now-lastEvent.TriggerTime < int64(stra.AlertDuration) {
			// 距离上次告警的时间小于告警统计周期，即不满足for的条件，不产生告警通知
			return
		}

		logger.Debugf("[lastEvent.LastSend:%+v][event.LastSend:%+v][now:%+v][lastEvent.TriggerTime:%+v][stra.AlertDuration:%+v][now-lastEvent.TriggerTime:%+v]\n",
			lastEvent.LastSend,
			event.LastSend,
			now,
			lastEvent.TriggerTime,
			stra.AlertDuration,
			now-lastEvent.TriggerTime,
		)

		// 满足for的条件了，应产生事件，但是未必一定要发送，上次没发送或者上次是恢复这次才发送，即保证只发一条
		if !lastEvent.LastSend || lastEvent.IsRecov() {
			event.LastSend = true
			event.MarkAlert()
			SendEvent(event)
		}
	}
}

func SendEvent(event *models.AlertEvent) {
	logger.Errorf("debug: SendEvent: %+v", event)
	// update last event
	LastEvents.Set(event.HashId, event)
	ok := EventQueue.PushFront(event)
	if !ok {
		logger.Errorf("push event:%v err", event)
	}
	logger.Debugf("[SendEvent.event.success][type:%+v][event:%+v]", event.IsPromePull, event)
}
