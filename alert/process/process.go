package process

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"reflect"
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
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/mohae/deepcopy"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

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

type Processor struct {
	datasourceId int64

	rule     *models.AlertRule
	fires    *AlertCurEventMap
	pendings *AlertCurEventMap
	inhibit  bool

	tagsMap    map[string]string
	tagsArr    []string
	target     string
	targetNote string
	groupName  string

	atertRuleCache  *memsto.AlertRuleCacheType
	TargetCache     *memsto.TargetCacheType
	BusiGroupCache  *memsto.BusiGroupCacheType
	alertMuteCache  *memsto.AlertMuteCacheType
	datasourceCache *memsto.DatasourceCacheType

	promClients *prom.PromClientMap
	ctx         *ctx.Context
	stats       *astats.Stats
	customeRule map[int]*models.AlertRule //有道添加，存放rule.RuleConfig 下 自定义的规则
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

// 有道添加，参考全局监控配置，重新按告警等级为索引，生成一整套监控配置
func (p *Processor) makeCustomeRule() {
	var prom_rule_config models.PromRuleConfig
	if err := json.Unmarshal([]byte(p.rule.RuleConfig), &prom_rule_config); err != nil {
		logger.Errorf("rule_eval:%s makeCustomeRule rule_config:%s, error:%v", p.Key(), p.rule.RuleConfig, err)
	} else {
		for severity, alertconfig := range prom_rule_config.AlertConfigs {
			alertconfig.FE2DB()
			p.customeRule[severity] = p.mergCustomeRule(alertconfig)
		}
	}
}

// 有道添加，将自定义的监控告警配置合并到全局监控配置中，生成一个新的alertrule，此时用了deepcopy，防止指针传递出现篡改情况
func (p *Processor) mergCustomeRule(alertconfig models.AlertConfig) *models.AlertRule {
	alerRule := deepcopy.Copy(p.rule).(*models.AlertRule)
	bVal := reflect.ValueOf(alerRule).Elem()
	vVal := reflect.ValueOf(&alertconfig).Elem()
	vTypeOfT := vVal.Type()
	for i := 0; i < vVal.NumField(); i++ {
		name := vTypeOfT.Field(i).Name
		if ok := bVal.FieldByName(name).IsValid(); ok {
			bVal.FieldByName(name).Set(reflect.ValueOf(vVal.Field(i).Interface()))
		}
	}
	return alerRule
}

// 有道添加，获取告警规则
func (p *Processor) getRule(customNotify bool, severity int) *models.AlertRule {
	if customNotify {
		if customeRule, ok := p.customeRule[severity]; ok {
			return customeRule
		}
	}
	return p.rule
}

func NewProcessor(rule *models.AlertRule, datasourceId int64, atertRuleCache *memsto.AlertRuleCacheType, targetCache *memsto.TargetCacheType,
	busiGroupCache *memsto.BusiGroupCacheType, alertMuteCache *memsto.AlertMuteCacheType, datasourceCache *memsto.DatasourceCacheType, promClients *prom.PromClientMap, ctx *ctx.Context,
	stats *astats.Stats) *Processor {

	p := &Processor{
		datasourceId: datasourceId,
		rule:         rule,

		TargetCache:     targetCache,
		BusiGroupCache:  busiGroupCache,
		alertMuteCache:  alertMuteCache,
		atertRuleCache:  atertRuleCache,
		datasourceCache: datasourceCache,

		promClients: promClients,
		ctx:         ctx,
		stats:       stats,
		customeRule: map[int]*models.AlertRule{},
	}

	p.mayHandleGroup()
	return p
}
func (p *Processor) Handle(anomalyPoints []common.AnomalyPoint, from string, inhibit bool) {
	// 有可能rule的一些配置已经发生变化，比如告警接收人、callbacks等
	// 这些信息的修改是不会引起worker restart的，但是确实会影响告警处理逻辑
	// 所以，这里直接从memsto.AlertRuleCache中获取并覆盖
	p.inhibit = inhibit
	p.rule = p.atertRuleCache.Get(p.rule.Id)
	p.makeCustomeRule()
	cachedRule := p.rule
	if cachedRule == nil {
		logger.Errorf("rule not found %+v", anomalyPoints)
		return
	}

	now := time.Now().Unix()
	alertingKeys := map[string]struct{}{}

	// 根据 event 的 tag 将 events 分组，处理告警抑制的情况
	eventsMap := make(map[string][]*models.AlertCurEvent)
	for _, anomalyPoint := range anomalyPoints {
		event := p.BuildEvent(anomalyPoint, from, now)
		// 如果 event 被 mute 了,本质也是 fire 的状态,这里无论如何都添加到 alertingKeys 中,防止 fire 的事件自动恢复了
		hash := event.Hash
		alertingKeys[hash] = struct{}{}
		//有道添加，根据事件是否使用全局配置和等级来获取告警规则
		cachedRule = p.getRule(event.CustomNotify, event.Severity)
		if mute.IsMuted(cachedRule, event, p.TargetCache, p.alertMuteCache) {
			logger.Debugf("rule_eval:%s event:%v is muted", p.Key(), event)
			continue
		}
		tagHash := TagHash(anomalyPoint)
		eventsMap[tagHash] = append(eventsMap[tagHash], event)
	}

	for _, events := range eventsMap {
		p.handleEvent(events)
	}

	p.HandleRecover(alertingKeys, now)
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
	//有道添加，根据事件是否使用全局配置和等级来获取告警规则
	rule := p.getRule(anomalyPoint.Promquery.CustomNotify, anomalyPoint.Promquery.Severity)
	event := rule.GenerateNewEvent(p.ctx)
	event.TriggerTime = anomalyPoint.Timestamp
	event.TagsMap = p.tagsMap
	event.DatasourceId = p.datasourceId
	event.Cluster = dsName
	event.Hash = hash
	event.TargetIdent = p.target
	event.TargetNote = p.targetNote
	event.TriggerValue = anomalyPoint.ReadableValue()
	event.TagsJSON = p.tagsArr
	event.GroupName = p.groupName
	event.Tags = strings.Join(p.tagsArr, ",,")
	event.IsRecovered = false
	event.Callbacks = rule.Callbacks
	event.CallbacksJSON = rule.CallbacksJSON
	event.Annotations = rule.Annotations
	event.AnnotationsJSON = make(map[string]string)
	event.RuleConfig = rule.RuleConfig
	event.RuleConfigJson = rule.RuleConfigJson
	//event.Severity = anomalyPoint.Severity
	//有道添加
	event.Severity = anomalyPoint.Promquery.Severity
	event.PromQl = anomalyPoint.Promquery.PromQl
	event.CustomNotify = anomalyPoint.Promquery.CustomNotify
	if len(event.RuleNote) > 0 {
		if len(anomalyPoint.Promquery.Description) > 0 {
			event.RuleNote = fmt.Sprintf("%s | %s", event.RuleNote, anomalyPoint.Promquery.Description)
		}
	} else {
		event.RuleNote = anomalyPoint.Promquery.Description
	}
	if from == "inner" {
		event.LastEvalTime = now
	} else {
		event.LastEvalTime = event.TriggerTime
	}
	return event
}

func (p *Processor) HandleRecover(alertingKeys map[string]struct{}, now int64) {
	for _, hash := range p.pendings.Keys() {
		if _, has := alertingKeys[hash]; has {
			continue
		}
		p.pendings.Delete(hash)
	}

	for hash := range p.fires.GetAll() {
		if _, has := alertingKeys[hash]; has {
			continue
		}
		p.RecoverSingle(hash, now, nil)
	}
}

func (p *Processor) RecoverSingle(hash string, now int64, value *string) {
	cachedRule := p.rule
	if cachedRule == nil {
		return
	}
	event, has := p.fires.Get(hash)
	if !has {
		return
	}
	//有道添加，根据事件是否使用全局配置和等级来获取告警规则
	cachedRule = p.getRule(event.CustomNotify, event.Severity)
	// 如果配置了留观时长，就不能立马恢复了
	if cachedRule.RecoverDuration > 0 && now-event.LastEvalTime < cachedRule.RecoverDuration {
		logger.Debugf("rule_eval:%s event:%v not recover", p.Key(), event)
		return
	}
	if value != nil {
		event.TriggerValue = *value
	}

	// 没查到触发阈值的vector，姑且就认为这个vector的值恢复了
	// 我确实无法分辨，是prom中有值但是未满足阈值所以没返回，还是prom中确实丢了一些点导致没有数据可以返回，尴尬
	p.fires.Delete(hash)
	p.pendings.Delete(hash)

	// 可能是因为调整了promql才恢复的，所以事件里边要体现最新的promql，否则用户会比较困惑
	// 当然，其实rule的各个字段都可能发生变化了，都更新一下吧
	//cachedRule.UpdateEvent(event) //有道添加，此时不能更新event，否则收到的恢复告警用户比较懵逼，尤其是一个规则组下面配置了多个规则情况下，最好还是让恢复事件保持原样吧
	event.IsRecovered = true
	event.LastEvalTime = now
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
		if p.rule.PromForDuration == 0 {
			fireEvents = append(fireEvents, event)
			if severity > event.Severity {
				severity = event.Severity
			}
			continue
		}

		var preTriggerTime int64
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
	//有道添加，根据事件是否使用全局配置和等级来获取告警规则
	cachedRule = p.getRule(event.CustomNotify, event.Severity)
	logger.Debugf("rule_eval:%s event:%+v fire", p.Key(), event)
	if fired, has := p.fires.Get(event.Hash); has {
		p.fires.UpdateLastEvalTime(event.Hash, event.LastEvalTime)

		if cachedRule.NotifyRepeatStep == 0 {
			logger.Debugf("rule_eval:%s event:%+v repeat is zero nothing to do", p.Key(), event)
			// 说明不想重复通知，那就直接返回了，nothing to do
			// do not need to send alert again
			return
		}

		// 之前发送过告警了，这次是否要继续发送，要看是否过了通道静默时间
		if event.LastEvalTime > fired.LastSentTime+int64(cachedRule.NotifyRepeatStep)*60 {
			if cachedRule.NotifyMaxNumber == 0 {
				// 最大可以发送次数如果是0，表示不想限制最大发送次数，一直发即可
				event.NotifyCurNumber = fired.NotifyCurNumber + 1
				event.FirstTriggerTime = fired.FirstTriggerTime
				p.pushEventToQueue(event)
			} else {
				// 有最大发送次数的限制，就要看已经发了几次了，是否达到了最大发送次数
				if fired.NotifyCurNumber >= cachedRule.NotifyMaxNumber {
					logger.Debugf("rule_eval:%s event:%+v reach max number", p.Key(), event)
					return
				} else {
					event.NotifyCurNumber = fired.NotifyCurNumber + 1
					event.FirstTriggerTime = fired.FirstTriggerTime
					p.pushEventToQueue(event)
				}
			}
		}
	} else {
		event.NotifyCurNumber = 1
		event.FirstTriggerTime = event.TriggerTime
		p.pushEventToQueue(event)
	}
}

func (p *Processor) pushEventToQueue(e *models.AlertCurEvent) {
	if !e.IsRecovered {
		e.LastSentTime = e.LastEvalTime
		p.fires.Set(e.Hash, e)
	}

	p.stats.CounterAlertsTotal.WithLabelValues(fmt.Sprintf("%d", e.DatasourceId)).Inc()
	dispatch.LogEvent(e, "push_queue")
	if !queue.EventQueue.PushFront(e) {
		logger.Warningf("event_push_queue: queue is full, event:%+v", e)
	}
}

func (p *Processor) RecoverAlertCurEventFromDb() {
	p.pendings = NewAlertCurEventMap(nil)

	curEvents, err := models.AlertCurEventGetByRuleIdAndCluster(p.ctx, p.rule.Id, p.datasourceId)
	if err != nil {
		logger.Errorf("recover event from db for rule:%s failed, err:%s", p.Key(), err)
		p.fires = NewAlertCurEventMap(nil)
		return
	}

	fireMap := make(map[string]*models.AlertCurEvent)
	for _, event := range curEvents {
		event.DB2Mem()
		fireMap[event.Hash] = event
	}

	p.fires = NewAlertCurEventMap(fireMap)
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
		}
	}
}

func (p *Processor) mayHandleGroup() {
	// handle bg
	bg := p.BusiGroupCache.GetByBusiGroupId(p.rule.GroupId)
	if bg != nil {
		p.groupName = bg.Name
	}
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
	return str.MD5(fmt.Sprintf("%d_%s_%d_%d", ruleId, vector.Labels.String(), datasourceId, vector.Promquery.Severity))
}

func TagHash(vector common.AnomalyPoint) string {
	return str.MD5(vector.Labels.String())
}
