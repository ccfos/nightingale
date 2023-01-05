package engine

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/server/writer"
	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/prom"
	"github.com/didi/nightingale/v5/src/server/common/conv"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/didi/nightingale/v5/src/server/naming"
	promstat "github.com/didi/nightingale/v5/src/server/stat"
)

func loopFilterRules(ctx context.Context) {
	// wait for samples
	time.Sleep(time.Duration(config.C.EngineDelay) * time.Second)

	duration := time.Duration(9000) * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(duration):
			filterRules()
			filterRecordingRules()
		}
	}
}

// 一个规则可能会在多个集群中生效，所以这里要把规则拆分成多个，此结构记录 id 和 cluster 的对应关系
type RuleSimpleInfo struct {
	Id      int64
	Cluster string
}

func filterRules() {
	ids := memsto.AlertRuleCache.GetRuleIds()
	logger.Debugf("AlertRuleCache.GetRuleIds success, ids.len: %d", len(ids))

	count := len(ids)
	mines := make([]*RuleSimpleInfo, 0, count)

	for i := 0; i < count; i++ {
		rule := memsto.AlertRuleCache.Get(ids[i])
		if rule == nil {
			logger.Debugf("AlertRuleCache.Get(%d) failed", ids[i])
			continue
		}

		var clusters []string
		if rule.Cluster == models.ClusterAll {
			clusters = config.ReaderClients.GetClusterNames()
		} else {
			clusters = strings.Fields(rule.Cluster)
		}

		for _, cluster := range clusters {
			if config.ReaderClients.IsNil(cluster) {
				// 没有这个集群的配置，跳过
				continue
			}

			node, err := naming.ClusterHashRing.GetNode(cluster, fmt.Sprint(ids[i]))
			if err != nil {
				logger.Warningf("rid:%d cluster:%s failed to get node from hashring:%v", ids[i], cluster, err)
				continue
			}

			if node == config.C.Heartbeat.Endpoint {
				mines = append(mines, &RuleSimpleInfo{Id: ids[i], Cluster: cluster})
			}
		}
	}

	Workers.Build(mines)
	RuleEvalForExternal.Build()
}

type RuleEval struct {
	cluster  string
	rule     *models.AlertRule
	fires    *AlertCurEventMap
	pendings *AlertCurEventMap
	quit     chan struct{}
}

type AlertCurEventMap struct {
	sync.RWMutex
	Data map[string]*models.AlertCurEvent
}

func (a *AlertCurEventMap) SetAll(data map[string]*models.AlertCurEvent) {
	a.Lock()
	defer a.Unlock()
	a.Data = data
}

func (a *AlertCurEventMap) Set(key string, value *models.AlertCurEvent) {
	a.Lock()
	defer a.Unlock()
	a.Data[key] = value
}

func (a *AlertCurEventMap) Get(key string) (*models.AlertCurEvent, bool) {
	a.RLock()
	defer a.RUnlock()
	event, exists := a.Data[key]
	return event, exists
}

func (a *AlertCurEventMap) UpdateLastEvalTime(key string, lastEvalTime int64) {
	a.Lock()
	defer a.Unlock()
	event, exists := a.Data[key]
	if !exists {
		return
	}
	event.LastEvalTime = lastEvalTime
}

func (a *AlertCurEventMap) Delete(key string) {
	a.Lock()
	defer a.Unlock()
	delete(a.Data, key)
}

func (a *AlertCurEventMap) Keys() []string {
	a.RLock()
	defer a.RUnlock()
	keys := make([]string, 0, len(a.Data))
	for k := range a.Data {
		keys = append(keys, k)
	}
	return keys
}

func (a *AlertCurEventMap) GetAll() map[string]*models.AlertCurEvent {
	a.RLock()
	defer a.RUnlock()
	return a.Data
}

func NewAlertCurEventMap() *AlertCurEventMap {
	return &AlertCurEventMap{
		Data: make(map[string]*models.AlertCurEvent),
	}
}

func (r *RuleEval) Stop() {
	logger.Infof("rule_eval:%d stopping", r.RuleID())
	close(r.quit)
}

func (r *RuleEval) RuleID() int64 {
	return r.rule.Id
}

func (r *RuleEval) Start() {
	logger.Infof("rule_eval:%d started", r.RuleID())
	for {
		select {
		case <-r.quit:
			// logger.Infof("rule_eval:%d stopped", r.RuleID())
			return
		default:
			r.Work()
			logger.Debugf("rule executed, rule_eval:%d", r.RuleID())
			interval := r.rule.PromEvalInterval
			if interval <= 0 {
				interval = 10
			}
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}
}

func (r *RuleEval) Work() {
	promql := strings.TrimSpace(r.rule.PromQl)
	if promql == "" {
		logger.Errorf("rule_eval:%d promql is blank", r.RuleID())
		return
	}

	if config.ReaderClients.IsNil(r.cluster) {
		logger.Error("reader client is nil")
		return
	}

	readerClient := config.ReaderClients.GetCli(r.cluster)

	var value model.Value
	var err error
	if r.rule.Algorithm == "" && (r.rule.Cate == "" || strings.ToLower(r.rule.Cate) == "prometheus") {
		var warnings prom.Warnings
		value, warnings, err = readerClient.Query(context.Background(), promql, time.Now())
		if err != nil {
			logger.Errorf("rule_eval:%d cluster:%s promql:%s, error:%v", r.RuleID(), r.cluster, promql, err)
			//notifyToMaintainer(err, "failed to query prometheus")
			Report(QueryPrometheusError)
			return
		}

		if len(warnings) > 0 {
			logger.Errorf("rule_eval:%d cluster:%s promql:%s, warnings:%v", r.RuleID(), r.cluster, promql, warnings)
			return
		}
		logger.Debugf("rule_eval:%d cluster:%s promql:%s, value:%v", r.RuleID(), r.cluster, promql, value)
	}

	r.Judge(r.cluster, conv.ConvertVectors(value))
}

type WorkersType struct {
	rules       map[string]*RuleEval
	recordRules map[string]RecordingRuleEval
}

var Workers = &WorkersType{rules: make(map[string]*RuleEval), recordRules: make(map[string]RecordingRuleEval)}

func (ws *WorkersType) Build(ris []*RuleSimpleInfo) {
	rules := make(map[string]*RuleSimpleInfo)

	for i := 0; i < len(ris); i++ {
		rule := memsto.AlertRuleCache.Get(ris[i].Id)
		if rule == nil {
			continue
		}

		hash := str.MD5(fmt.Sprintf("%d_%d_%s_%s",
			rule.Id,
			rule.PromEvalInterval,
			rule.PromQl,
			ris[i].Cluster,
		))

		rules[hash] = ris[i]
	}

	// stop old
	for hash := range Workers.rules {
		if _, has := rules[hash]; !has {
			Workers.rules[hash].Stop()
			delete(Workers.rules, hash)
		}
	}

	// start new
	for hash := range rules {
		if _, has := Workers.rules[hash]; has {
			// already exists
			continue
		}

		elst, err := models.AlertCurEventGetByRuleIdAndCluster(rules[hash].Id, rules[hash].Cluster)
		if err != nil {
			logger.Errorf("worker_build: AlertCurEventGetByRule failed: %v", err)
			continue
		}

		firemap := make(map[string]*models.AlertCurEvent)
		for i := 0; i < len(elst); i++ {
			elst[i].DB2Mem()
			firemap[elst[i].Hash] = elst[i]
		}
		fires := NewAlertCurEventMap()
		fires.SetAll(firemap)
		re := &RuleEval{
			rule:     memsto.AlertRuleCache.Get(rules[hash].Id),
			quit:     make(chan struct{}),
			fires:    fires,
			pendings: NewAlertCurEventMap(),
			cluster:  rules[hash].Cluster,
		}

		go re.Start()
		Workers.rules[hash] = re
	}
}

func (ws *WorkersType) BuildRe(ris []*RuleSimpleInfo) {
	rules := make(map[string]*RuleSimpleInfo)

	for i := 0; i < len(ris); i++ {
		rule := memsto.RecordingRuleCache.Get(ris[i].Id)
		if rule == nil {
			continue
		}

		hash := str.MD5(fmt.Sprintf("%d_%d_%s_%s",
			rule.Id,
			rule.PromEvalInterval,
			rule.PromQl,
			ris[i].Cluster,
		))

		rules[hash] = ris[i]
	}

	// stop old
	for hash := range Workers.recordRules {
		if _, has := rules[hash]; !has {
			Workers.recordRules[hash].Stop()
			delete(Workers.recordRules, hash)
		}
	}

	// start new
	for hash := range rules {
		if _, has := Workers.recordRules[hash]; has {
			// already exists
			continue
		}
		re := RecordingRuleEval{
			rule:    memsto.RecordingRuleCache.Get(rules[hash].Id),
			quit:    make(chan struct{}),
			cluster: rules[hash].Cluster,
		}

		go re.Start()
		Workers.recordRules[hash] = re
	}
}

func (r *RuleEval) Judge(clusterName string, vectors []conv.Vector) {
	now := time.Now().Unix()

	alertingKeys, ruleExists := r.MakeNewEvent("inner", now, clusterName, vectors)
	if !ruleExists {
		return
	}

	// handle recovered events
	r.recoverRule(alertingKeys, now)
}

func (r *RuleEval) MakeNewEvent(from string, now int64, clusterName string, vectors []conv.Vector) (map[string]struct{}, bool) {
	// 有可能rule的一些配置已经发生变化，比如告警接收人、callbacks等
	// 这些信息的修改是不会引起worker restart的，但是确实会影响告警处理逻辑
	// 所以，这里直接从memsto.AlertRuleCache中获取并覆盖
	curRule := memsto.AlertRuleCache.Get(r.rule.Id)
	if curRule == nil {
		return map[string]struct{}{}, false
	}

	r.rule = curRule

	count := len(vectors)
	alertingKeys := make(map[string]struct{})
	for i := 0; i < count; i++ {
		// compute hash
		hash := str.MD5(fmt.Sprintf("%d_%s_%s", r.rule.Id, vectors[i].Key, r.cluster))
		alertingKeys[hash] = struct{}{}

		// rule disabled in this time span?
		if isNoneffective(vectors[i].Timestamp, r.rule) {
			logger.Debugf("event_disabled: rule_eval:%d cluster:%s rule:%v timestamp:%d", r.rule.Id, r.cluster, r.rule, vectors[i].Timestamp)
			continue
		}

		// handle series tags
		tagsMap := make(map[string]string)
		for label, value := range vectors[i].Labels {
			tagsMap[string(label)] = string(value)
		}

		// handle rule tags
		for _, tag := range r.rule.AppendTagsJSON {
			arr := strings.SplitN(tag, "=", 2)
			tagsMap[arr[0]] = arr[1]
		}

		tagsMap["rulename"] = r.rule.Name

		// handle target note
		targetIdent, has := tagsMap["ident"]
		targetNote := ""
		if has {
			target, exists := memsto.TargetCache.Get(string(targetIdent))
			if exists {
				targetNote = target.Note

				// 对于包含ident的告警事件，check一下ident所属bg和rule所属bg是否相同
				// 如果告警规则选择了只在本BG生效，那其他BG的机器就不能因此规则产生告警
				if r.rule.EnableInBG == 1 && target.GroupId != r.rule.GroupId {
					logger.Debugf("event_enable_in_bg: rule_eval:%d cluster:%s", r.rule.Id, r.cluster)
					continue
				}
			} else if strings.Contains(r.rule.PromQl, "target_up") {
				// target 已经不存在了，可能是被删除了
				continue
			}
		}

		event := &models.AlertCurEvent{
			TriggerTime: vectors[i].Timestamp,
			TagsMap:     tagsMap,
			GroupId:     r.rule.GroupId,
			RuleName:    r.rule.Name,
			Cluster:     clusterName,
		}

		bg := memsto.BusiGroupCache.GetByBusiGroupId(r.rule.GroupId)
		if bg != nil {
			event.GroupName = bg.Name
		}

		// isMuted need TriggerTime RuleName TagsMap and clusterName
		if IsMuted(event) {
			logger.Infof("event_muted: rule_id=%d %s cluster:%s", r.rule.Id, vectors[i].Key, r.cluster)
			continue
		}

		tagsArr := labelMapToArr(tagsMap)
		sort.Strings(tagsArr)

		event.Cate = r.rule.Cate
		event.Hash = hash
		event.RuleId = r.rule.Id
		event.RuleName = r.rule.Name
		event.RuleNote = r.rule.Note
		event.RuleProd = r.rule.Prod
		event.RuleAlgo = r.rule.Algorithm
		event.Severity = r.rule.Severity
		event.PromForDuration = r.rule.PromForDuration
		event.PromQl = r.rule.PromQl
		event.PromEvalInterval = r.rule.PromEvalInterval
		event.Callbacks = r.rule.Callbacks
		event.CallbacksJSON = r.rule.CallbacksJSON
		event.RunbookUrl = r.rule.RunbookUrl
		event.NotifyRecovered = r.rule.NotifyRecovered
		event.NotifyChannels = r.rule.NotifyChannels
		event.NotifyChannelsJSON = r.rule.NotifyChannelsJSON
		event.NotifyGroups = r.rule.NotifyGroups
		event.NotifyGroupsJSON = r.rule.NotifyGroupsJSON
		event.TargetIdent = string(targetIdent)
		event.TargetNote = targetNote
		event.TriggerValue = readableValue(vectors[i].Value)
		event.TagsJSON = tagsArr
		event.Tags = strings.Join(tagsArr, ",,")
		event.IsRecovered = false
		event.LastEvalTime = now
		if from != "inner" {
			event.LastEvalTime = event.TriggerTime
		}

		r.handleNewEvent(event)

	}

	return alertingKeys, true
}

func readableValue(value float64) string {
	ret := fmt.Sprintf("%.5f", value)
	ret = strings.TrimRight(ret, "0")
	return strings.TrimRight(ret, ".")
}

func labelMapToArr(m map[string]string) []string {
	numLabels := len(m)

	labelStrings := make([]string, 0, numLabels)
	for label, value := range m {
		labelStrings = append(labelStrings, fmt.Sprintf("%s=%s", label, value))
	}

	if numLabels > 1 {
		sort.Strings(labelStrings)
	}

	return labelStrings
}

func (r *RuleEval) handleNewEvent(event *models.AlertCurEvent) {
	if event.PromForDuration == 0 {
		r.fireEvent(event)
		return
	}

	var preTriggerTime int64
	preEvent, has := r.pendings.Get(event.Hash)
	if has {
		r.pendings.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
		preTriggerTime = preEvent.TriggerTime
	} else {
		r.pendings.Set(event.Hash, event)
		preTriggerTime = event.TriggerTime
	}

	if event.LastEvalTime-preTriggerTime+int64(event.PromEvalInterval) >= int64(event.PromForDuration) {
		r.fireEvent(event)
	}
}

func (r *RuleEval) fireEvent(event *models.AlertCurEvent) {
	if fired, has := r.fires.Get(event.Hash); has {
		r.fires.UpdateLastEvalTime(event.Hash, event.LastEvalTime)

		if r.rule.NotifyRepeatStep == 0 {
			// 说明不想重复通知，那就直接返回了，nothing to do
			return
		}

		// 之前发送过告警了，这次是否要继续发送，要看是否过了通道静默时间
		if event.LastEvalTime > fired.LastSentTime+int64(r.rule.NotifyRepeatStep)*60 {
			if r.rule.NotifyMaxNumber == 0 {
				// 最大可以发送次数如果是0，表示不想限制最大发送次数，一直发即可
				event.NotifyCurNumber = fired.NotifyCurNumber + 1
				event.FirstTriggerTime = fired.FirstTriggerTime
				r.pushEventToQueue(event)
			} else {
				// 有最大发送次数的限制，就要看已经发了几次了，是否达到了最大发送次数
				if fired.NotifyCurNumber >= r.rule.NotifyMaxNumber {
					return
				} else {
					event.NotifyCurNumber = fired.NotifyCurNumber + 1
					event.FirstTriggerTime = fired.FirstTriggerTime
					r.pushEventToQueue(event)
				}
			}

		}
	} else {
		event.NotifyCurNumber = 1
		event.FirstTriggerTime = event.TriggerTime
		r.pushEventToQueue(event)
	}
}

func (r *RuleEval) recoverRule(alertingKeys map[string]struct{}, now int64) {
	for _, hash := range r.pendings.Keys() {
		if _, has := alertingKeys[hash]; has {
			continue
		}
		r.pendings.Delete(hash)
	}

	for hash, event := range r.fires.GetAll() {
		if _, has := alertingKeys[hash]; has {
			continue
		}

		r.recoverEvent(hash, event, now)
	}
}

func (r *RuleEval) RecoverEvent(hash string, now int64, value float64) {
	curRule := memsto.AlertRuleCache.Get(r.rule.Id)
	if curRule == nil {
		return
	}
	r.rule = curRule

	r.pendings.Delete(hash)
	event, has := r.fires.Get(hash)
	if !has {
		return
	}

	event.TriggerValue = fmt.Sprintf("%.5f", value)
	r.recoverEvent(hash, event, now)
}

func (r *RuleEval) recoverEvent(hash string, event *models.AlertCurEvent, now int64) {
	// 如果配置了留观时长，就不能立马恢复了
	if r.rule.RecoverDuration > 0 && now-event.LastEvalTime < r.rule.RecoverDuration {
		return
	}

	// 没查到触发阈值的vector，姑且就认为这个vector的值恢复了
	// 我确实无法分辨，是prom中有值但是未满足阈值所以没返回，还是prom中确实丢了一些点导致没有数据可以返回，尴尬
	r.fires.Delete(hash)
	r.pendings.Delete(hash)

	event.IsRecovered = true
	event.LastEvalTime = now
	// 可能是因为调整了promql才恢复的，所以事件里边要体现最新的promql，否则用户会比较困惑
	// 当然，其实rule的各个字段都可能发生变化了，都更新一下吧
	event.RuleName = r.rule.Name
	event.RuleNote = r.rule.Note
	event.RuleProd = r.rule.Prod
	event.RuleAlgo = r.rule.Algorithm
	event.Severity = r.rule.Severity
	event.PromForDuration = r.rule.PromForDuration
	event.PromQl = r.rule.PromQl
	event.PromEvalInterval = r.rule.PromEvalInterval
	event.Callbacks = r.rule.Callbacks
	event.CallbacksJSON = r.rule.CallbacksJSON
	event.RunbookUrl = r.rule.RunbookUrl
	event.NotifyRecovered = r.rule.NotifyRecovered
	event.NotifyChannels = r.rule.NotifyChannels
	event.NotifyChannelsJSON = r.rule.NotifyChannelsJSON
	event.NotifyGroups = r.rule.NotifyGroups
	event.NotifyGroupsJSON = r.rule.NotifyGroupsJSON
	r.pushEventToQueue(event)
}

func (r *RuleEval) pushEventToQueue(event *models.AlertCurEvent) {
	if !event.IsRecovered {
		event.LastSentTime = event.LastEvalTime
		r.fires.Set(event.Hash, event)
	}

	promstat.CounterAlertsTotal.WithLabelValues(event.Cluster).Inc()
	LogEvent(event, "push_queue")
	if !EventQueue.PushFront(event) {
		logger.Warningf("event_push_queue: queue is full")
	}
}

func filterRecordingRules() {
	ids := memsto.RecordingRuleCache.GetRuleIds()

	count := len(ids)
	mines := make([]*RuleSimpleInfo, 0, count)

	for i := 0; i < count; i++ {
		rule := memsto.RecordingRuleCache.Get(ids[i])
		if rule == nil {
			logger.Debugf("rule %d not found", ids[i])
			continue
		}

		var clusters []string
		if rule.Cluster == models.ClusterAll {
			clusters = config.ReaderClients.GetClusterNames()
		} else {
			clusters = strings.Fields(rule.Cluster)
		}

		for _, cluster := range clusters {
			if config.ReaderClients.IsNil(cluster) {
				// 没有这个集群的配置，跳过
				continue
			}

			node, err := naming.ClusterHashRing.GetNode(cluster, fmt.Sprint(ids[i]))
			if err != nil {
				logger.Warning("failed to get node from hashring:", err)
				continue
			}

			if node == config.C.Heartbeat.Endpoint {
				mines = append(mines, &RuleSimpleInfo{Id: ids[i], Cluster: cluster})
			}
		}
	}

	Workers.BuildRe(mines)
}

type RecordingRuleEval struct {
	cluster string
	rule    *models.RecordingRule
	quit    chan struct{}
}

func (r RecordingRuleEval) Stop() {
	logger.Infof("recording_rule_eval:%d stopping", r.RuleID())
	close(r.quit)
}

func (r RecordingRuleEval) RuleID() int64 {
	return r.rule.Id
}

func (r RecordingRuleEval) Start() {
	logger.Infof("recording_rule_eval:%d started", r.RuleID())
	for {
		select {
		case <-r.quit:
			// logger.Infof("rule_eval:%d stopped", r.RuleID())
			return
		default:
			r.Work()
			interval := r.rule.PromEvalInterval
			if interval <= 0 {
				interval = 10
			}
			time.Sleep(time.Duration(interval) * time.Second)
		}
	}
}

func (r RecordingRuleEval) Work() {
	promql := strings.TrimSpace(r.rule.PromQl)
	if promql == "" {
		logger.Errorf("recording_rule_eval:%d promql is blank", r.RuleID())
		return
	}

	if config.ReaderClients.IsNil(r.cluster) {
		log.Println("reader client is nil")
		return
	}

	value, warnings, err := config.ReaderClients.GetCli(r.cluster).Query(context.Background(), promql, time.Now())
	if err != nil {
		logger.Errorf("recording_rule_eval:%d cluster:%s promql:%s, error:%v", r.RuleID(), r.cluster, promql, err)
		return
	}

	if len(warnings) > 0 {
		logger.Errorf("recording_rule_eval:%d cluster:%s promql:%s, warnings:%v", r.RuleID(), r.cluster, promql, warnings)
		return
	}
	ts := conv.ConvertToTimeSeries(value, r.rule)
	if len(ts) != 0 {
		for _, v := range ts {
			writer.Writers.PushSample(r.rule.Name, v, r.cluster)
		}
	}
}

type RuleEvalForExternalType struct {
	sync.RWMutex
	rules map[string]RuleEval // key: hash of ruleid_promevalinterval_promql_cluster
}

var RuleEvalForExternal = RuleEvalForExternalType{rules: make(map[string]RuleEval)}

func (re *RuleEvalForExternalType) Build() {
	rids := memsto.AlertRuleCache.GetRuleIds()
	rules := make(map[string]*RuleSimpleInfo)

	for i := 0; i < len(rids); i++ {
		rule := memsto.AlertRuleCache.Get(rids[i])
		if rule == nil {
			continue
		}

		var clusters []string
		if rule.Cluster == models.ClusterAll {
			clusters = config.ReaderClients.GetClusterNames()
		} else {
			clusters = strings.Fields(rule.Cluster)
		}

		for _, cluster := range clusters {
			hash := str.MD5(fmt.Sprintf("%d_%d_%s_%s",
				rule.Id,
				rule.PromEvalInterval,
				rule.PromQl,
				cluster,
			))
			re.Lock()
			rules[hash] = &RuleSimpleInfo{
				Id:      rule.Id,
				Cluster: cluster,
			}
			re.Unlock()
		}
	}

	// stop old
	for oldHash := range re.rules {
		if _, has := rules[oldHash]; !has {
			re.Lock()
			delete(re.rules, oldHash)
			re.Unlock()
		}
	}

	// start new
	re.Lock()
	defer re.Unlock()
	for hash, ruleSimple := range rules {
		if _, has := re.rules[hash]; has {
			// already exists
			continue
		}

		elst, err := models.AlertCurEventGetByRuleIdAndCluster(ruleSimple.Id, ruleSimple.Cluster)
		if err != nil {
			logger.Errorf("worker_build: AlertCurEventGetByRule failed: %v", err)
			continue
		}

		firemap := make(map[string]*models.AlertCurEvent)
		for i := 0; i < len(elst); i++ {
			elst[i].DB2Mem()
			firemap[elst[i].Hash] = elst[i]
		}
		fires := NewAlertCurEventMap()
		fires.SetAll(firemap)
		newRe := RuleEval{
			rule:     memsto.AlertRuleCache.Get(ruleSimple.Id),
			quit:     make(chan struct{}),
			fires:    fires,
			pendings: NewAlertCurEventMap(),
			cluster:  ruleSimple.Cluster,
		}

		re.rules[hash] = newRe
	}
}

func (re *RuleEvalForExternalType) Get(rid int64, cluster string) (RuleEval, bool) {
	re.RLock()
	defer re.RUnlock()
	rule := memsto.AlertRuleCache.Get(rid)
	if rule == nil {
		return RuleEval{}, false
	}

	hash := str.MD5(fmt.Sprintf("%d_%d_%s_%s",
		rule.Id,
		rule.PromEvalInterval,
		rule.PromQl,
		cluster,
	))

	if ret, has := re.rules[hash]; has {
		return ret, has
	}

	return RuleEval{}, false
}
