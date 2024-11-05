package process

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/alert/dispatch"
	"github.com/ccfos/nightingale/v6/alert/mute"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/tplx"
	"github.com/ccfos/nightingale/v6/pushgw/writer"

	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type EventMuteHookFunc func(event *models.AlertCurEvent) bool

type ExternalProcessorsType struct {
	ExternalLock sync.RWMutex
	Processors   map[string]*Processor
}

var ExternalProcessors ExternalProcessorsType

func NewExternalProcessors() *ExternalProcessorsType {
	return &ExternalProcessorsType{
		Processors: make(map[string]*Processor),
	}
}

func (e *ExternalProcessorsType) GetExternalAlertRule(datasourceId, id int64) (*Processor, bool) {
	e.ExternalLock.RLock()
	defer e.ExternalLock.RUnlock()
	processor, has := e.Processors[common.RuleKey(datasourceId, id)]
	return processor, has
}

type HandleEventFunc func(event *models.AlertCurEvent)

type Processor struct {
	datasourceId int64
	EngineName   string

	rule                 *models.AlertRule
	fires                *AlertCurEventMap
	pendings             *AlertCurEventMap
	pendingsUseByRecover *AlertCurEventMap
	inhibit              bool

	tagsMap    map[string]string
	tagsArr    []string
	target     string
	targetNote string
	groupName  string

	alertRuleCache          *memsto.AlertRuleCacheType
	TargetCache             *memsto.TargetCacheType
	TargetsOfAlertRuleCache *memsto.TargetsOfAlertRuleCacheType
	BusiGroupCache          *memsto.BusiGroupCacheType
	alertMuteCache          *memsto.AlertMuteCacheType
	datasourceCache         *memsto.DatasourceCacheType

	ctx   *ctx.Context
	Stats *astats.Stats

	HandleFireEventHook    HandleEventFunc
	HandleRecoverEventHook HandleEventFunc
	EventMuteHook          EventMuteHookFunc
}

func (p *Processor) Key() string {
	return common.RuleKey(p.datasourceId, p.rule.Id)
}

func (p *Processor) DatasourceId() int64 {
	return p.datasourceId
}

func (p *Processor) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%d_%s_%d",
		p.rule.Id,
		p.rule.PromEvalInterval,
		p.rule.RuleConfig,
		p.datasourceId,
	))
}

func NewProcessor(engineName string, rule *models.AlertRule, datasourceId int64, alertRuleCache *memsto.AlertRuleCacheType,
	targetCache *memsto.TargetCacheType, targetsOfAlertRuleCache *memsto.TargetsOfAlertRuleCacheType,
	busiGroupCache *memsto.BusiGroupCacheType, alertMuteCache *memsto.AlertMuteCacheType, datasourceCache *memsto.DatasourceCacheType, ctx *ctx.Context,
	stats *astats.Stats) *Processor {

	p := &Processor{
		EngineName:   engineName,
		datasourceId: datasourceId,
		rule:         rule,

		TargetCache:             targetCache,
		TargetsOfAlertRuleCache: targetsOfAlertRuleCache,
		BusiGroupCache:          busiGroupCache,
		alertMuteCache:          alertMuteCache,
		alertRuleCache:          alertRuleCache,
		datasourceCache:         datasourceCache,

		ctx:   ctx,
		Stats: stats,

		HandleFireEventHook:    func(event *models.AlertCurEvent) {},
		HandleRecoverEventHook: func(event *models.AlertCurEvent) {},
		EventMuteHook:          func(event *models.AlertCurEvent) bool { return false },
	}

	p.mayHandleGroup()
	return p
}

func (p *Processor) Handle(anomalyPoints []common.AnomalyPoint, from string, inhibit bool) {
	// 有可能rule的一些配置已经发生变化，比如告警接收人、callbacks等
	// 这些信息的修改是不会引起worker restart的，但是确实会影响告警处理逻辑
	// 所以，这里直接从memsto.AlertRuleCache中获取并覆盖
	p.inhibit = inhibit
	cachedRule := p.alertRuleCache.Get(p.rule.Id)
	if cachedRule == nil {
		logger.Errorf("rule not found %+v", anomalyPoints)
		p.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", p.DatasourceId()), "handle_event").Inc()
		return
	}

	p.rule = cachedRule
	now := time.Now().Unix()
	alertingKeys := map[string]struct{}{}

	// 根据 event 的 tag 将 events 分组，处理告警抑制的情况
	eventsMap := make(map[string][]*models.AlertCurEvent)
	for _, anomalyPoint := range anomalyPoints {
		event := p.BuildEvent(anomalyPoint, from, now)
		// 如果 event 被 mute 了,本质也是 fire 的状态,这里无论如何都添加到 alertingKeys 中,防止 fire 的事件自动恢复了
		hash := event.Hash
		alertingKeys[hash] = struct{}{}
		isMuted, detail := mute.IsMuted(cachedRule, event, p.TargetCache, p.alertMuteCache)
		if isMuted {
			p.Stats.CounterMuteTotal.WithLabelValues(event.GroupName).Inc()
			logger.Debugf("rule_eval:%s event:%v is muted, detail:%s", p.Key(), event, detail)
			continue
		}

		if p.EventMuteHook(event) {
			p.Stats.CounterMuteTotal.WithLabelValues(event.GroupName).Inc()
			logger.Debugf("rule_eval:%s event:%v is muted by hook", p.Key(), event)
			continue
		}

		tagHash := TagHash(anomalyPoint)
		eventsMap[tagHash] = append(eventsMap[tagHash], event)
	}

	for _, events := range eventsMap {
		p.handleEvent(events)
	}

	p.HandleRecover(alertingKeys, now, inhibit)
}

func (p *Processor) BuildEvent(anomalyPoint common.AnomalyPoint, from string, now int64) *models.AlertCurEvent {
	p.fillTags(anomalyPoint)
	p.mayHandleIdent()
	hash := Hash(p.rule.Id, p.datasourceId, anomalyPoint)
	ds := p.datasourceCache.GetById(p.datasourceId)
	var dsName string
	if ds != nil {
		dsName = ds.Name
	}

	event := p.rule.GenerateNewEvent(p.ctx)

	bg := p.BusiGroupCache.GetByBusiGroupId(p.rule.GroupId)
	if bg != nil {
		event.GroupName = bg.Name
	}

	event.TriggerTime = anomalyPoint.Timestamp
	event.TagsMap = p.tagsMap
	event.DatasourceId = p.datasourceId
	event.Cluster = dsName
	event.Hash = hash
	event.TargetIdent = p.target
	event.TargetNote = p.targetNote
	event.TriggerValue = anomalyPoint.ReadableValue()
	event.TriggerValues = anomalyPoint.Values
	event.TagsJSON = p.tagsArr
	event.Tags = strings.Join(p.tagsArr, ",,")
	event.IsRecovered = false
	event.Callbacks = p.rule.Callbacks
	event.CallbacksJSON = p.rule.CallbacksJSON
	event.Annotations = p.rule.Annotations
	event.AnnotationsJSON = make(map[string]string)
	event.RuleConfig = p.rule.RuleConfig
	event.RuleConfigJson = p.rule.RuleConfigJson
	event.Severity = anomalyPoint.Severity
	event.ExtraConfig = p.rule.ExtraConfigJSON
	event.PromQl = anomalyPoint.Query

	if p.target != "" {
		if pt, exist := p.TargetCache.Get(p.target); exist {
			event.Target = pt
		} else {
			logger.Infof("Target[ident: %s] doesn't exist in cache.", p.target)
		}
	}

	if event.TriggerValues != "" && strings.Count(event.TriggerValues, "$") > 1 {
		// TriggerValues 有多个变量，将多个变量都放到 TriggerValue 中
		event.TriggerValue = event.TriggerValues
	}

	if from == "inner" {
		event.LastEvalTime = now
	} else {
		event.LastEvalTime = event.TriggerTime
	}

	// 生成事件之后，立马进程 relabel 处理
	Relabel(p.rule, event)
	return event
}

func Relabel(rule *models.AlertRule, event *models.AlertCurEvent) {
	if rule == nil {
		return
	}

	if len(rule.EventRelabelConfig) == 0 {
		return
	}

	// need to keep the original label
	event.OriginalTags = event.Tags
	event.OriginalTagsJSON = make([]string, len(event.TagsJSON))

	labels := make([]prompb.Label, len(event.TagsJSON))
	for i, tag := range event.TagsJSON {
		label := strings.SplitN(tag, "=", 2)
		event.OriginalTagsJSON[i] = tag
		labels[i] = prompb.Label{Name: label[0], Value: label[1]}
	}

	for i := 0; i < len(rule.EventRelabelConfig); i++ {
		if rule.EventRelabelConfig[i].Replacement == "" {
			rule.EventRelabelConfig[i].Replacement = "$1"
		}

		if rule.EventRelabelConfig[i].Separator == "" {
			rule.EventRelabelConfig[i].Separator = ";"
		}

		if rule.EventRelabelConfig[i].Regex == "" {
			rule.EventRelabelConfig[i].Regex = "(.*)"
		}
	}

	// relabel process
	relabels := writer.Process(labels, rule.EventRelabelConfig...)
	event.TagsJSON = make([]string, len(relabels))
	event.TagsMap = make(map[string]string, len(relabels))
	for i, label := range relabels {
		event.TagsJSON[i] = fmt.Sprintf("%s=%s", label.Name, label.Value)
		event.TagsMap[label.Name] = label.Value
	}
	event.Tags = strings.Join(event.TagsJSON, ",,")
}

func (p *Processor) HandleRecover(alertingKeys map[string]struct{}, now int64, inhibit bool) {
	for _, hash := range p.pendings.Keys() {
		if _, has := alertingKeys[hash]; has {
			continue
		}
		p.pendings.Delete(hash)
	}

	hashArr := make([]string, 0, len(alertingKeys))
	for hash := range p.fires.GetAll() {
		if _, has := alertingKeys[hash]; has {
			continue
		}

		hashArr = append(hashArr, hash)
	}
	p.HandleRecoverEvent(hashArr, now, inhibit)

}

func (p *Processor) HandleRecoverEvent(hashArr []string, now int64, inhibit bool) {
	cachedRule := p.rule
	if cachedRule == nil {
		return
	}

	if !inhibit {
		for _, hash := range hashArr {
			p.RecoverSingle(hash, now, nil)
		}
		return
	}

	eventMap := make(map[string]models.AlertCurEvent)
	for _, hash := range hashArr {
		event, has := p.fires.Get(hash)
		if !has {
			continue
		}

		e, exists := eventMap[event.Tags]
		if !exists {
			eventMap[event.Tags] = *event
			continue
		}

		if e.Severity > event.Severity {
			// hash 对应的恢复事件的被抑制了，把之前的事件删除
			p.fires.Delete(e.Hash)
			p.pendings.Delete(e.Hash)
			models.AlertCurEventDelByHash(p.ctx, e.Hash)
			eventMap[event.Tags] = *event
		}
	}

	for _, event := range eventMap {
		p.RecoverSingle(event.Hash, now, nil)
	}
}

func (p *Processor) RecoverSingle(hash string, now int64, value *string, values ...string) {
	cachedRule := p.rule
	if cachedRule == nil {
		return
	}

	event, has := p.fires.Get(hash)
	if !has {
		return
	}

	// 如果配置了留观时长，就不能立马恢复了
	if cachedRule.RecoverDuration > 0 {
		lastPendingEvent, has := p.pendingsUseByRecover.Get(hash)
		if !has {
			// 说明没有产生过异常点，就不需要恢复了
			logger.Debugf("rule_eval:%s event:%v do not has pending event, not recover", p.Key(), event)
			return
		}

		if now-lastPendingEvent.LastEvalTime < cachedRule.RecoverDuration {
			logger.Debugf("rule_eval:%s event:%v not recover", p.Key(), event)
			return
		}
	}

	if value != nil {
		event.TriggerValue = *value
		if len(values) > 0 {
			event.TriggerValues = values[0]
		}
	}

	// 没查到触发阈值的vector，姑且就认为这个vector的值恢复了
	// 我确实无法分辨，是prom中有值但是未满足阈值所以没返回，还是prom中确实丢了一些点导致没有数据可以返回，尴尬
	p.fires.Delete(hash)
	p.pendings.Delete(hash)
	p.pendingsUseByRecover.Delete(hash)

	// 可能是因为调整了promql才恢复的，所以事件里边要体现最新的promql，否则用户会比较困惑
	// 当然，其实rule的各个字段都可能发生变化了，都更新一下吧
	cachedRule.UpdateEvent(event)
	event.IsRecovered = true
	event.LastEvalTime = now

	p.HandleRecoverEventHook(event)
	p.pushEventToQueue(event)
}

func (p *Processor) handleEvent(events []*models.AlertCurEvent) {
	var fireEvents []*models.AlertCurEvent
	// severity 初始为 4, 一定为遇到比自己优先级高的事件
	severity := 4
	for _, event := range events {
		if event == nil {
			continue
		}

		if _, has := p.pendingsUseByRecover.Get(event.Hash); has {
			p.pendingsUseByRecover.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
		} else {
			p.pendingsUseByRecover.Set(event.Hash, event)
		}

		if p.rule.PromForDuration == 0 {
			fireEvents = append(fireEvents, event)
			if severity > event.Severity {
				severity = event.Severity
			}
			continue
		}

		var preTriggerTime int64 // 第一个 pending event 的触发时间
		preEvent, has := p.pendings.Get(event.Hash)
		if has {
			p.pendings.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
			preTriggerTime = preEvent.TriggerTime
		} else {
			p.pendings.Set(event.Hash, event)
			preTriggerTime = event.TriggerTime
		}

		if event.LastEvalTime-preTriggerTime+int64(event.PromEvalInterval) >= int64(p.rule.PromForDuration) {
			fireEvents = append(fireEvents, event)
			if severity > event.Severity {
				severity = event.Severity
			}
			continue
		}
	}

	p.inhibitEvent(fireEvents, severity)
}

func (p *Processor) inhibitEvent(events []*models.AlertCurEvent, highSeverity int) {
	for _, event := range events {
		if p.inhibit && event.Severity > highSeverity {
			logger.Debugf("rule_eval:%s event:%+v inhibit highSeverity:%d", p.Key(), event, highSeverity)
			continue
		}
		p.fireEvent(event)
	}
}

func (p *Processor) fireEvent(event *models.AlertCurEvent) {
	// As p.rule maybe outdated, use rule from cache
	cachedRule := p.rule
	if cachedRule == nil {
		return
	}

	logger.Debugf("rule_eval:%s event:%+v fire", p.Key(), event)
	if fired, has := p.fires.Get(event.Hash); has {
		p.fires.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
		event.FirstTriggerTime = fired.FirstTriggerTime
		p.HandleFireEventHook(event)

		if cachedRule.NotifyRepeatStep == 0 {
			logger.Debugf("rule_eval:%s event:%+v repeat is zero nothing to do", p.Key(), event)
			// 说明不想重复通知，那就直接返回了，nothing to do
			// do not need to send alert again
			return
		}

		// 之前发送过告警了，这次是否要继续发送，要看是否过了通道静默时间
		if event.LastEvalTime >= fired.LastSentTime+int64(cachedRule.NotifyRepeatStep)*60 {
			if cachedRule.NotifyMaxNumber == 0 {
				// 最大可以发送次数如果是0，表示不想限制最大发送次数，一直发即可
				event.NotifyCurNumber = fired.NotifyCurNumber + 1
				p.pushEventToQueue(event)
			} else {
				// 有最大发送次数的限制，就要看已经发了几次了，是否达到了最大发送次数
				if fired.NotifyCurNumber >= cachedRule.NotifyMaxNumber {
					logger.Debugf("rule_eval:%s event:%+v reach max number", p.Key(), event)
					return
				} else {
					event.NotifyCurNumber = fired.NotifyCurNumber + 1
					p.pushEventToQueue(event)
				}
			}
		}
	} else {
		event.NotifyCurNumber = 1
		event.FirstTriggerTime = event.TriggerTime
		p.HandleFireEventHook(event)
		p.pushEventToQueue(event)
	}
}

func (p *Processor) pushEventToQueue(e *models.AlertCurEvent) {
	if !e.IsRecovered {
		e.LastSentTime = e.LastEvalTime
		p.fires.Set(e.Hash, e)
	}

	dispatch.LogEvent(e, "push_queue")
	if !queue.EventQueue.PushFront(e) {
		logger.Warningf("event_push_queue: queue is full, event:%+v", e)
		p.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", p.DatasourceId()), "push_event_queue").Inc()
	}
}

func (p *Processor) RecoverAlertCurEventFromDb() {
	p.pendings = NewAlertCurEventMap(nil)
	p.pendingsUseByRecover = NewAlertCurEventMap(nil)

	curEvents, err := models.AlertCurEventGetByRuleIdAndDsId(p.ctx, p.rule.Id, p.datasourceId)
	if err != nil {
		logger.Errorf("recover event from db for rule:%s failed, err:%s", p.Key(), err)
		p.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", p.DatasourceId()), "get_recover_event").Inc()
		p.fires = NewAlertCurEventMap(nil)
		return
	}

	fireMap := make(map[string]*models.AlertCurEvent)
	pendingsUseByRecoverMap := make(map[string]*models.AlertCurEvent)
	for _, event := range curEvents {
		if event.Cate == models.HOST {
			target, exists := p.TargetCache.Get(event.TargetIdent)
			if exists && target.EngineName != p.EngineName && !(p.ctx.IsCenter && target.EngineName == "") {
				// 如果是 host rule，且 target 的 engineName 不是当前的 engineName 或者是中心机房 target EngineName 为空，就跳过
				continue
			}
		}

		event.DB2Mem()
		target, exists := p.TargetCache.Get(event.TargetIdent)
		if exists {
			event.Target = target
		}

		fireMap[event.Hash] = event
		e := *event
		pendingsUseByRecoverMap[event.Hash] = &e
	}

	p.fires = NewAlertCurEventMap(fireMap)

	// 修改告警规则，或者进程重启之后，需要重新加载 pendingsUseByRecover
	p.pendingsUseByRecover = NewAlertCurEventMap(pendingsUseByRecoverMap)
}

func (p *Processor) fillTags(anomalyPoint common.AnomalyPoint) {
	// handle series tags
	tagsMap := make(map[string]string)
	for label, value := range anomalyPoint.Labels {
		tagsMap[string(label)] = string(value)
	}

	var e = &models.AlertCurEvent{
		TagsMap: tagsMap,
	}

	// handle rule tags
	for _, tag := range p.rule.AppendTagsJSON {
		arr := strings.SplitN(tag, "=", 2)

		var defs = []string{
			"{{$labels := .TagsMap}}",
			"{{$value := .TriggerValue}}",
		}
		tagValue := arr[1]
		text := strings.Join(append(defs, tagValue), "")
		t, err := template.New(fmt.Sprint(p.rule.Id)).Funcs(template.FuncMap(tplx.TemplateFuncMap)).Parse(text)
		if err != nil {
			tagValue = fmt.Sprintf("parse tag value failed, err:%s", err)
			tagsMap[arr[0]] = tagValue
			continue
		}

		var body bytes.Buffer
		err = t.Execute(&body, e)
		if err != nil {
			tagValue = fmt.Sprintf("parse tag value failed, err:%s", err)
			tagsMap[arr[0]] = tagValue
			continue
		}

		tagsMap[arr[0]] = body.String()
	}

	tagsMap["rulename"] = p.rule.Name
	p.tagsMap = tagsMap

	// handle tagsArr
	p.tagsArr = labelMapToArr(tagsMap)
}

func (p *Processor) mayHandleIdent() {
	// handle ident
	if ident, has := p.tagsMap["ident"]; has {
		if target, exists := p.TargetCache.Get(ident); exists {
			p.target = target.Ident
			p.targetNote = target.Note
		} else {
			p.target = ident
			p.targetNote = ""
		}
	} else {
		p.target = ""
		p.targetNote = ""
	}
}

func (p *Processor) mayHandleGroup() {
	// handle bg
	bg := p.BusiGroupCache.GetByBusiGroupId(p.rule.GroupId)
	if bg != nil {
		p.groupName = bg.Name
	}
}

func (p *Processor) DeleteProcessEvent(hash string) {
	p.fires.Delete(hash)
	p.pendings.Delete(hash)
	p.pendingsUseByRecover.Delete(hash)
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

func Hash(ruleId, datasourceId int64, vector common.AnomalyPoint) string {
	return str.MD5(fmt.Sprintf("%d_%s_%d_%d_%s", ruleId, vector.Labels.String(), datasourceId, vector.Severity, vector.Query))
}

func TagHash(vector common.AnomalyPoint) string {
	return str.MD5(vector.Labels.String())
}
