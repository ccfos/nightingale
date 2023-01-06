package engine

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/common/conv"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

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

func NewAlertCurEventMap(data map[string]*models.AlertCurEvent) *AlertCurEventMap {
	if data == nil {
		return &AlertCurEventMap{
			Data: make(map[string]*models.AlertCurEvent),
		}
	}
	return &AlertCurEventMap{
		Data: data,
	}
}

// AlertVector 包含一个告警事件的告警上下文
type AlertVector struct {
	Ctx    *AlertRuleContext
	Rule   *models.AlertRule
	Vector conv.Vector
	From   string

	tagsMap    map[string]string
	tagsArr    []string
	target     string
	targetNote string
	groupName  string
}

func NewAlertVector(ctx *AlertRuleContext, rule *models.AlertRule, vector conv.Vector, from string) *AlertVector {
	if rule == nil {
		rule = ctx.rule
	}
	av := &AlertVector{
		Ctx:    ctx,
		Rule:   rule,
		Vector: vector,
		From:   from,
	}
	av.fillTags()
	av.mayHandleIdent()
	av.mayHandleGroup()
	return av
}

func (av *AlertVector) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%s_%s", av.Rule.Id, av.Vector.Key, av.Ctx.cluster))
}

func (av *AlertVector) fillTags() {
	// handle series tags
	tagsMap := make(map[string]string)
	for label, value := range av.Vector.Labels {
		tagsMap[string(label)] = string(value)
	}

	// handle rule tags
	for _, tag := range av.Rule.AppendTagsJSON {
		arr := strings.SplitN(tag, "=", 2)
		tagsMap[arr[0]] = arr[1]
	}

	tagsMap["rulename"] = av.Rule.Name
	av.tagsMap = tagsMap

	// handle tagsArr
	av.tagsArr = labelMapToArr(tagsMap)
}

func (av *AlertVector) mayHandleIdent() {
	// handle ident
	if ident, has := av.tagsMap["ident"]; has {
		if target, exists := memsto.TargetCache.Get(ident); exists {
			av.target = target.Ident
			av.targetNote = target.Note
		}
	}
}

func (av *AlertVector) mayHandleGroup() {
	// handle bg
	bg := memsto.BusiGroupCache.GetByBusiGroupId(av.Rule.GroupId)
	if bg != nil {
		av.groupName = bg.Name
	}
}

func (av *AlertVector) BuildEvent(now int64) *models.AlertCurEvent {
	event := av.Rule.GenerateNewEvent()
	event.TriggerTime = av.Vector.Timestamp
	event.TagsMap = av.tagsMap
	event.Cluster = av.Ctx.cluster
	event.Hash = av.Hash()
	event.TargetIdent = av.target
	event.TargetNote = av.targetNote
	event.TriggerValue = av.Vector.ReadableValue()
	event.TagsJSON = av.tagsArr
	event.GroupName = av.groupName
	event.Tags = strings.Join(av.tagsArr, ",,")
	event.IsRecovered = false

	if av.From == "inner" {
		event.LastEvalTime = now
	} else {
		event.LastEvalTime = event.TriggerTime
	}
	return event
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
