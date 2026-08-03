package process

import (
	"bytes"
	"encoding/json"
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
	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/relabel"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/robfig/cron/v3"
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

type HandleEventFunc func(event *models.AlertCurEvent)

type Processor struct {
	datasourceId int64
	EngineName   string

	rule                 *models.AlertRule
	fires                *AlertCurEventMap
	pendings             *AlertCurEventMap
	pendingsUseByRecover *AlertCurEventMap
	inhibit              bool

	tagsMap   map[string]string
	tagsArr   []string
	groupName string

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

	ScheduleEntry    cron.Entry
	PromEvalInterval int

	// EvalRec 当前评估周期的执行记录，由 eval 侧每轮注入，Handle 各环节累加漏斗计数；
	// nil 时（evallog 未启用/外部事件路径）所有计数方法为 no-op。
	// 评估与事件处理在同一 goroutine 串行执行，无并发写。
	EvalRec *evallog.EvalRecord
}

func (p *Processor) Key() string {
	return common.RuleKey(p.datasourceId, p.rule.Id)
}

func (p *Processor) DatasourceId() int64 {
	return p.datasourceId
}

func (p *Processor) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%s_%s_%d",
		p.rule.Id,
		p.rule.CronPattern,
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
	}

	p.mayHandleGroup()
	return p
}

func (p *Processor) Handle(anomalyPoints []models.AnomalyPoint, from string, inhibit bool) {
	// 有可能rule的一些配置已经发生变化，比如告警接收人、callbacks等
	// 这些信息的修改是不会引起worker restart的，但是确实会影响告警处理逻辑
	// 所以，这里直接从memsto.AlertRuleCache中获取并覆盖
	p.inhibit = inhibit
	cachedRule := p.alertRuleCache.Get(p.rule.Id)
	if cachedRule == nil {
		logger.Warningf("alert_eval_%d datasource_%d handle error: rule not found, maybe rule has been deleted, anomalyPoints:%+v", p.rule.Id, p.datasourceId, anomalyPoints)
		p.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", p.DatasourceId()), "handle_event", p.BusiGroupCache.GetNameByBusiGroupId(p.rule.GroupId), fmt.Sprintf("%v", p.rule.Id)).Inc()
		return
	}

	// 在 rule 变化之前取到 ruleHash
	ruleHash := p.rule.Hash()

	p.rule = cachedRule
	now := time.Now().Unix()
	alertingKeys := map[string]struct{}{}

	// 根据 event 的 tag 将 events 分组，处理告警抑制的情况
	eventsMap := make(map[string][]*models.AlertCurEvent)
	for _, anomalyPoint := range anomalyPoints {
		event := p.BuildEvent(anomalyPoint, from, now, ruleHash)
		event.NotifyRuleIds = cachedRule.NotifyRuleIds
		// 如果 event 被 mute 了,本质也是 fire 的状态,这里无论如何都添加到 alertingKeys 中,防止 fire 的事件自动恢复了
		hash := event.Hash
		alertingKeys[hash] = struct{}{}

		// event processor
		eventCopy := event.DeepCopy()
		event = dispatch.HandleEventPipeline(cachedRule.PipelineConfigs, eventCopy, event, dispatch.EventProcessorCache, p.ctx, cachedRule.Id, "alert_rule")
		if event == nil {
			logger.Infof("alert_eval_%d datasource_%d is muted drop by pipeline event:%s", p.rule.Id, p.datasourceId, eventCopy.Hash)
			p.recEvent(eventCopy, evallog.StageDropByPipeline, "")
			continue
		}

		// event mute
		isMuted, detail, muteId, muteType := mute.IsMuted(cachedRule, event, p.TargetCache, p.alertMuteCache)
		if isMuted {
			if muteType == models.MuteTypeNotifyOnly {
				// 只屏蔽通知：事件照常产生并记录，仅打标，后续在通知阶段据此跳过发送并写通知记录
				logger.Infof("alert_eval_%d datasource_%d notify muted only, detail:%s event:%s", p.rule.Id, p.datasourceId, detail, event.Hash)
				event.NotifyMuted = 1
				event.MuteId = muteId
				p.recEvent(event, evallog.StageMutedNotifyOnly, fmt.Sprintf("mute_id:%d %s", muteId, detail))
			} else {
				logger.Infof("alert_eval_%d datasource_%d is muted, detail:%s event:%s", p.rule.Id, p.datasourceId, detail, event.Hash)
				p.recEvent(event, evallog.StageMuted, fmt.Sprintf("mute_id:%d %s", muteId, detail))
				p.Stats.CounterMuteTotal.WithLabelValues(
					fmt.Sprintf("%v", event.GroupName),
					fmt.Sprintf("%v", p.rule.Id),
					fmt.Sprintf("%v", muteId),
					fmt.Sprintf("%v", p.datasourceId),
				).Inc()
				continue
			}
		}

		if dispatch.EventMuteHook(event) {
			logger.Infof("alert_eval_%d datasource_%d is muted by hook event:%s", p.rule.Id, p.datasourceId, event.Hash)
			p.recEvent(event, evallog.StageMutedByHook, "")
			p.Stats.CounterMuteTotal.WithLabelValues(
				fmt.Sprintf("%v", event.GroupName),
				fmt.Sprintf("%v", p.rule.Id),
				fmt.Sprintf("%v", 0),
				fmt.Sprintf("%v", p.datasourceId),
			).Inc()
			continue
		}

		tagHash := TagHash(anomalyPoint)
		eventsMap[tagHash] = append(eventsMap[tagHash], event)
	}

	for _, events := range eventsMap {
		p.handleEvent(events)
	}

	if from == "inner" {
		p.HandleRecover(alertingKeys, now, inhibit)
	}
}

// fireStage 从 fireEvent 的裁决 message 推导轨迹阶段。
// 三类前缀与 fireEvent 内的 message 赋值一一对应，默认按 fired 计（firing 路径）。
func fireStage(message string) string {
	switch {
	case strings.HasPrefix(message, "notify muted"):
		return evallog.StageNotifyMuted
	case strings.HasPrefix(message, "stalled"):
		return evallog.StageStalled
	default:
		return evallog.StageFired
	}
}

// recEvent 记录一个事件在当前评估周期的裁决轨迹，并累加对应的漏斗计数。
// EvalRec 为 nil（evallog 未启用或外部事件路径）时为 no-op。
func (p *Processor) recEvent(event *models.AlertCurEvent, stage, detail string) {
	if p.EvalRec == nil || event == nil {
		return
	}
	p.EvalRec.AddEvent(evallog.EventTrail{
		Hash:     event.Hash,
		Tags:     event.Tags,
		Severity: event.Severity,
		Stage:    stage,
		Detail:   detail,
	})
}

func (p *Processor) BuildEvent(anomalyPoint models.AnomalyPoint, from string, now int64, ruleHash string) *models.AlertCurEvent {
	p.fillTags(anomalyPoint)

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
	event.TriggerValue = anomalyPoint.ReadableValue()
	event.TriggerValues = anomalyPoint.Values
	event.TriggerValuesJson = models.EventTriggerValues{ValuesWithUnit: anomalyPoint.ValuesUnit}
	event.TagsJSON = p.tagsArr
	event.Tags = strings.Join(p.tagsArr, ",,")
	event.IsRecovered = false
	event.Callbacks = p.rule.Callbacks
	event.CallbacksJSON = p.rule.CallbacksJSON
	event.Annotations = p.rule.Annotations
	event.RuleConfig = p.rule.RuleConfig
	event.RuleConfigJson = p.rule.RuleConfigJson
	event.Severity = anomalyPoint.Severity
	event.ExtraConfig = p.rule.ExtraConfigJSON
	event.PromQl = anomalyPoint.Query
	event.RecoverConfig = anomalyPoint.RecoverConfig
	event.RuleHash = ruleHash

	if anomalyPoint.TriggerType == models.TriggerTypeNodata {
		event.TriggerValue = "nodata"
		ruleConfig := models.RuleQuery{}
		json.Unmarshal([]byte(p.rule.RuleConfig), &ruleConfig)
		ruleConfig.TriggerType = anomalyPoint.TriggerType
		b, _ := json.Marshal(ruleConfig)
		event.RuleConfig = string(b)
	}

	if err := json.Unmarshal([]byte(p.rule.Annotations), &event.AnnotationsJSON); err != nil {
		event.AnnotationsJSON = make(map[string]string) // 解析失败时使用空 map
		logger.Warningf("alert_eval_%d datasource_%d unmarshal annotations json failed: %v", p.rule.Id, p.datasourceId, err)
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

	// 放到 Relabel(p.rule, event) 下面，为了处理 relabel 之后，标签里才出现 ident 的情况
	p.mayHandleIdent(event)

	if event.TargetIdent != "" {
		if pt, exist := p.TargetCache.Get(event.TargetIdent); exist {
			pt.GroupNames = p.BusiGroupCache.GetNamesByBusiGroupIds(pt.GroupIds)
			event.Target = pt
		} else {
			logger.Infof("alert_eval_%d datasource_%d fill event target error, ident: %s doesn't exist in cache.", p.rule.Id, p.datasourceId, event.TargetIdent)
		}
	}

	return event
}

func Relabel(rule *models.AlertRule, event *models.AlertCurEvent) {
	if rule == nil {
		return
	}

	// need to keep the original label
	event.OriginalTags = event.Tags
	event.OriginalTagsJSON = event.TagsJSON

	if len(rule.EventRelabelConfig) == 0 {
		return
	}

	relabel.EventRelabel(event, rule.EventRelabelConfig)
}

func (p *Processor) HandleRecover(alertingKeys map[string]struct{}, now int64, inhibit bool) {
	for _, hash := range p.pendings.Keys() {
		if _, has := alertingKeys[hash]; has {
			continue
		}
		p.pendings.Delete(hash)
	}

	hashArr := make([]string, 0, len(alertingKeys))
	for hash, _ := range p.fires.GetAll() {
		if _, has := alertingKeys[hash]; has {
			continue
		}

		hashArr = append(hashArr, hash)
	}
	p.HandleRecoverEvent(hashArr, now, inhibit)

}

// HandleRecoverEvent 处理需要恢复的事件。
// 恢复不做抑制合并：每个已 fire 的事件各自独立恢复，保证 fire/recover 对称。
// inhibit 形参保留以兼容调用方签名，恢复阶段不再使用。
func (p *Processor) HandleRecoverEvent(hashArr []string, now int64, inhibit bool) {
	cachedRule := p.rule
	if cachedRule == nil {
		return
	}

	for _, hash := range hashArr {
		p.RecoverSingle(false, hash, now, nil)
	}
}

func (p *Processor) RecoverSingle(byRecover bool, hash string, now int64, value *string, values ...string) {
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
			logger.Debugf("alert_eval_%d datasource_%d event:%s do not has pending event, not recover", p.rule.Id, p.datasourceId, event.Hash)
			return
		}

		if now-lastPendingEvent.LastEvalTime < cachedRule.RecoverDuration {
			logger.Debugf("alert_eval_%d datasource_%d event:%s not recover", p.rule.Id, p.datasourceId, event.Hash)
			return
		}
	}

	// 如果设置了恢复条件，则不能在此处恢复，必须依靠 recoverPoint 来恢复
	if event.RecoverConfig.JudgeType != models.Origin && !byRecover {
		logger.Debugf("alert_eval_%d datasource_%d event:%s not recover", p.rule.Id, p.datasourceId, event.Hash)
		return
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

	// 恢复通知是否屏蔽，按"恢复时刻"（clock=now）重新判定，而不是沿用触发期写入 p.fires 的陈旧 NotifyMuted：
	// 仅当此刻仍命中「只屏蔽通知」规则才继续静默恢复通知，屏蔽已到期/删除则正常发出恢复通知。
	event.NotifyMuted = 0
	event.MuteId = 0
	if hit, muteId, muteType := mute.EventMuteStrategy(event, p.alertMuteCache, now); hit && muteType == models.MuteTypeNotifyOnly {
		event.NotifyMuted = 1
		event.MuteId = muteId
	}

	p.HandleRecoverEventHook(event)
	p.recEvent(event, evallog.StageRecovered, fmt.Sprintf("recovered, trigger_value:%s", event.TriggerValue))
	p.pushEventToQueue(event)
}

func (p *Processor) handleEvent(events []*models.AlertCurEvent) {
	var fireEvents []*models.AlertCurEvent
	// severity 初始为最低优先级, 一定为遇到比自己优先级高的事件
	severity := models.SeverityLowest
	for _, event := range events {
		if event == nil {
			continue
		}

		if _, has := p.pendingsUseByRecover.Get(event.Hash); has {
			p.pendingsUseByRecover.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
		} else {
			p.pendingsUseByRecover.Set(event.Hash, event)
		}

		event.PromEvalInterval = p.PromEvalInterval
		if p.rule.PromForDuration == 0 {
			fireEvents = append(fireEvents, event)
			if severity > event.Severity {
				severity = event.Severity
			}
			continue
		}

		var preEvalTime int64 // 第一个 pending event 的检测时间
		preEvent, has := p.pendings.Get(event.Hash)
		if has {
			p.pendings.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
			preEvalTime = preEvent.FirstEvalTime
		} else {
			event.FirstEvalTime = event.LastEvalTime
			p.pendings.Set(event.Hash, event)
			preEvalTime = event.FirstEvalTime
		}

		if event.LastEvalTime-preEvalTime+int64(event.PromEvalInterval) >= int64(p.rule.PromForDuration) {
			fireEvents = append(fireEvents, event)
			if severity > event.Severity {
				severity = event.Severity
			}
			continue
		}

		// for 持续时长未满足，本轮不触发
		p.recEvent(event, evallog.StagePending, fmt.Sprintf("for=%ds elapsed=%ds",
			p.rule.PromForDuration, event.LastEvalTime-preEvalTime+int64(event.PromEvalInterval)))
	}

	p.inhibitEvent(fireEvents, severity)
}

func (p *Processor) inhibitEvent(events []*models.AlertCurEvent, highSeverity int) {
	for _, event := range events {
		if p.inhibit && event.Severity > highSeverity {
			logger.Debugf("alert_eval_%d datasource_%d event:%s inhibit highSeverity:%d", p.rule.Id, p.datasourceId, event.Hash, highSeverity)
			p.recEvent(event, evallog.StageInhibited, fmt.Sprintf("severity=%d inhibited by severity=%d", event.Severity, highSeverity))
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

	message := "unknown"
	defer func() {
		logger.Infof("alert_eval_%d datasource_%d event-hash-%s %s", p.rule.Id, p.datasourceId, event.Hash, message)
		// message 已完整描述本轮裁决原因（首次触发/重复间隔未到/达最大次数/屏蔽期快照），
		// 直接作为轨迹的裁决说明，stage 由其前缀推导
		p.recEvent(event, fireStage(message), message)
	}()

	// 只屏蔽通知：事件要照常产生并落库，落库节奏与 notify_max_number 截断都与不屏蔽时完全一致，
	// 仅不真正发送通知。为避免解除屏蔽后真实的 NotifyCurNumber 被"虚假发送"耗尽、LastSentTime 被推进，
	// 导致重复/恢复通知发不出去，这里用一套影子计数（LastMutedPersistTime / MutedPersistNumber）
	// 复刻正常重复逻辑来驱动落库快照，真实的 NotifyCurNumber、LastSentTime 全程保持不变。
	if event.NotifyMuted == 1 {
		if fired, has := p.fires.Get(event.Hash); has {
			event.FirstTriggerTime = fired.FirstTriggerTime
			// 与正常 fired 路径一致，每轮评估都触发 hook：下游（如 n9e-plus 抑制存储）
			// 按时间戳过期，依赖每轮刷新感知事件仍活跃；只屏蔽通知的事件仍应参与抑制联动
			p.HandleFireEventHook(event)

			// 对齐 notify_repeat_step == 0：不重复，仅保活，不再落新快照
			if cachedRule.NotifyRepeatStep == 0 {
				p.fires.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
				message = "notify muted, notify_repeat_step is 0, no repeat snapshot"
				return
			}

			// 影子发送时间：本屏蔽期首次落库前回退到进入屏蔽时的真实 LastSentTime
			mutedBase := fired.LastMutedPersistTime
			if mutedBase == 0 {
				mutedBase = fired.LastSentTime
			}
			if event.LastEvalTime >= mutedBase+int64(cachedRule.NotifyRepeatStep)*60 {
				// 影子通知次数：本屏蔽期首次落库前继承进入屏蔽时的真实 NotifyCurNumber，
				// 从而与不屏蔽时一样受 notify_max_number 截断
				mutedNum := fired.MutedPersistNumber
				if mutedNum == 0 {
					mutedNum = fired.NotifyCurNumber
				}
				if cachedRule.NotifyMaxNumber != 0 && mutedNum >= cachedRule.NotifyMaxNumber {
					p.fires.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
					message = fmt.Sprintf("notify muted, snapshot stalled by notify_max_number(%d >= %d)", mutedNum, cachedRule.NotifyMaxNumber)
					return
				}
				// 真实通知状态全程冻结，仅推进影子计数后落一条快照
				event.NotifyCurNumber = fired.NotifyCurNumber
				event.LastSentTime = fired.LastSentTime
				event.FirstEvalTime = fired.FirstEvalTime
				event.MutedPersistNumber = mutedNum + 1
				event.LastMutedPersistTime = event.LastEvalTime
				message = fmt.Sprintf("notify muted, periodic snapshot persisted(#%d)", event.MutedPersistNumber)
				p.pushEventToQueue(event)
				return
			}

			p.fires.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
			message = "notify muted, kept alive, next snapshot pending"
			return
		}
		// 屏蔽期内首次由静默转为 firing：立即落一条，并把影子计数作为后续落库节奏基准。
		// 影子基准用 LastEvalTime（=now，与 512 行后续快照及非屏蔽路径 LastSentTime 取值一致），
		// 不用 TriggerTime（数据时间戳），否则第一条重复快照会提前一个数据延迟窗口触发。
		event.NotifyCurNumber = 0
		event.FirstTriggerTime = event.TriggerTime
		event.MutedPersistNumber = 1
		event.LastMutedPersistTime = event.LastEvalTime
		message = "notify muted, event persisted without notifying"
		p.HandleFireEventHook(event)
		p.pushEventToQueue(event)
		return
	}

	if fired, has := p.fires.Get(event.Hash); has {
		p.fires.UpdateLastEvalTime(event.Hash, event.LastEvalTime)
		// 事件已解除屏蔽、以正常状态评估：清掉屏蔽期的影子计数，
		// 使下次再进入屏蔽时从当前真实通知状态重新起算（屏蔽反复开关不残留）
		p.fires.ResetMutedShadow(event.Hash)
		event.FirstTriggerTime = fired.FirstTriggerTime
		p.HandleFireEventHook(event)

		// 事件此前仅在「只屏蔽通知」期间产生过（fired.NotifyCurNumber==0，首次通知被 consume 跳过），
		// 屏蔽解除后应把这次当作首次真实通知补发，避免 NotifyRepeatStep==0 的规则永远收不到第一次通知。
		if fired.NotifyCurNumber == 0 {
			event.NotifyCurNumber = 1
			message = fmt.Sprintf("fired, first notify after unmute, first_trigger_time: %d", event.FirstTriggerTime)
			p.pushEventToQueue(event)
			return
		}

		if cachedRule.NotifyRepeatStep == 0 {
			message = "stalled, rule.notify_repeat_step is 0, no need to repeat notify"
			return
		}

		// 之前发送过告警了，这次是否要继续发送，要看是否过了通道静默时间
		if event.LastEvalTime >= fired.LastSentTime+int64(cachedRule.NotifyRepeatStep)*60 {
			if cachedRule.NotifyMaxNumber == 0 {
				// 最大可以发送次数如果是0，表示不想限制最大发送次数，一直发即可
				event.NotifyCurNumber = fired.NotifyCurNumber + 1
				message = fmt.Sprintf("fired, notify_repeat_step_matched(%d >= %d + %d * 60) notify_max_number_ignore(#%d / %d)", event.LastEvalTime, fired.LastSentTime, cachedRule.NotifyRepeatStep, event.NotifyCurNumber, cachedRule.NotifyMaxNumber)
				p.pushEventToQueue(event)
			} else {
				// 有最大发送次数的限制，就要看已经发了几次了，是否达到了最大发送次数
				if fired.NotifyCurNumber >= cachedRule.NotifyMaxNumber {
					message = fmt.Sprintf("stalled, notify_repeat_step_matched(%d >= %d + %d * 60) notify_max_number_not_matched(#%d / %d)", event.LastEvalTime, fired.LastSentTime, cachedRule.NotifyRepeatStep, fired.NotifyCurNumber, cachedRule.NotifyMaxNumber)
					return
				} else {
					event.NotifyCurNumber = fired.NotifyCurNumber + 1
					message = fmt.Sprintf("fired, notify_repeat_step_matched(%d >= %d + %d * 60) notify_max_number_matched(#%d / %d)", event.LastEvalTime, fired.LastSentTime, cachedRule.NotifyRepeatStep, event.NotifyCurNumber, cachedRule.NotifyMaxNumber)
					p.pushEventToQueue(event)
				}
			}
		} else {
			message = fmt.Sprintf("stalled, notify_repeat_step_not_matched(%d < %d + %d * 60)", event.LastEvalTime, fired.LastSentTime, cachedRule.NotifyRepeatStep)
		}
	} else {
		event.NotifyCurNumber = 1
		event.FirstTriggerTime = event.TriggerTime
		message = fmt.Sprintf("fired, first_trigger_time: %d", event.FirstTriggerTime)
		p.HandleFireEventHook(event)
		p.pushEventToQueue(event)
	}
}

func (p *Processor) pushEventToQueue(e *models.AlertCurEvent) {
	if !e.IsRecovered {
		// 只屏蔽通知的事件不算作已发送，保留原 LastSentTime，
		// 使解除屏蔽后按真正的上次发送时间计算重复通知间隔。
		if e.NotifyMuted != 1 {
			e.LastSentTime = e.LastEvalTime
		}
		p.fires.Set(e.Hash, e)
	}

	dispatch.LogEvent(e, "push_queue")

	// 入队的必须是快照，不能是 e 本身：p.fires 里存的就是同一个指针，
	// RecoverSingle 会把它取出来原地改成恢复事件（IsRecovered=true）再二次入队，
	// UpdateLastEvalTime / ResetMutedShadow 也每个评估周期原地写它。
	// 队列一旦有积压，还没被消费的那条触发事件就会被就地改写成恢复事件，
	// 结果是 alert_his_event 落两条恢复、零条触发，alert_cur_event 什么也不留；
	// 同时消费者 goroutine 正在读这个对象，构成 data race。
	if !queue.EventQueue.PushFront(e.DeepCopy()) {
		logger.Warningf("alert_eval_%d datasource_%d event_push_queue: queue is full, event:%s", p.rule.Id, p.datasourceId, e.Hash)
		p.recEvent(e, evallog.StagePushQueueFailed, "event queue is full")
		p.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", p.DatasourceId()), "push_event_queue", p.BusiGroupCache.GetNameByBusiGroupId(p.rule.GroupId), fmt.Sprintf("%v", p.rule.Id)).Inc()
	}
}

func (p *Processor) RecoverAlertCurEventFromDb() {
	p.pendings = NewAlertCurEventMap(nil)
	p.pendingsUseByRecover = NewAlertCurEventMap(nil)

	curEvents, err := models.AlertCurEventGetByRuleIdAndDsId(p.ctx, p.rule.Id, p.datasourceId)
	if err != nil {
		logger.Errorf("alert_eval_%d datasource_%d recover event from db failed, err:%s", p.rule.Id, p.datasourceId, err)
		p.Stats.CounterRuleEvalErrorTotal.WithLabelValues(fmt.Sprintf("%v", p.DatasourceId()), "get_recover_event", p.BusiGroupCache.GetNameByBusiGroupId(p.rule.GroupId), fmt.Sprintf("%v", p.rule.Id)).Inc()
		p.fires = NewAlertCurEventMap(nil)
		return
	}

	fireMap := make(map[string]*models.AlertCurEvent)
	pendingsUseByRecoverMap := make(map[string]*models.AlertCurEvent)
	for _, event := range curEvents {
		alertRule := p.alertRuleCache.Get(event.RuleId)
		if alertRule == nil {
			continue
		}
		event.NotifyRuleIds = alertRule.NotifyRuleIds

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
			target.GroupNames = p.BusiGroupCache.GetNamesByBusiGroupIds(target.GroupIds)
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

func (p *Processor) fillTags(anomalyPoint models.AnomalyPoint) {
	// handle series tags
	tagsMap := make(map[string]string)
	for label, value := range anomalyPoint.Labels {
		tagsMap[string(label)] = string(value)
	}

	var e = &models.AlertCurEvent{
		TagsMap: tagsMap,
	}

	// handle rule tags
	tags := p.rule.AppendTagsJSON
	tags = append(tags, "rulename="+p.rule.Name)
	for _, tag := range tags {
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
	p.tagsMap = tagsMap

	// handle tagsArr
	p.tagsArr = labelMapToArr(tagsMap)
}

func (p *Processor) mayHandleIdent(event *models.AlertCurEvent) {
	// handle ident
	if ident, has := event.TagsMap["ident"]; has {
		if target, exists := p.TargetCache.Get(ident); exists {
			event.TargetIdent = target.Ident
			event.TargetNote = target.Note
		} else {
			event.TargetIdent = ident
			event.TargetNote = ""
		}
	} else {
		event.TargetIdent = ""
		event.TargetNote = ""
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

func Hash(ruleId, datasourceId int64, vector models.AnomalyPoint) string {
	return str.MD5(fmt.Sprintf("%d_%s_%d_%d_%s", ruleId, vector.Labels.String(), datasourceId, vector.Severity, vector.Query))
}

func TagHash(vector models.AnomalyPoint) string {
	return str.MD5(vector.Labels.String())
}
