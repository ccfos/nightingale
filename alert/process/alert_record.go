package process

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/toolkits/pkg/logger"
)

var AlertRecordRedisKey = "alert::%d::%d::%d"
var AlertRecordMaxCount int64 = 100000
var defaultMaxLengthForList int64 = 1000
var defaultTimeDurationForList = 24 * time.Hour

// AlertRecordCount 内存中维护 alert record 的 key 以及对应 list 的长度
var AlertRecordCount sync.Map

// InitAlertRecordCount init alert record count map
func InitAlertRecordCount(ctx *ctx.Context, redis *storage.Redis) error {
	alertRules, err := models.AlertRuleGetsAll(ctx)
	if err != nil {
		return err
	}
	// 启动时构造 24 小时内所有 alertRecord 的 key，并从 redis 中查询长度
	now := time.Now()
	keys := make([]string, 0)
	for i := 0; i < 24; i++ {
		for _, rule := range alertRules {
			key := fmt.Sprintf(AlertRecordRedisKey, rule.Id, now.Day(), now.Hour())
			keys = append(keys, key)
		}
		now = now.Add(-time.Hour)
	}

	arc, err := storage.MLLen(ctx.GetContext(), *redis, keys)
	if err != nil {
		return err
	}
	AlertRecordCount = *mapToSyncMap(arc)
	return nil
}

func mapToSyncMap(m map[string]int64) *sync.Map {
	var sm sync.Map
	for k, v := range m {
		sm.Store(k, v)
	}
	return &sm
}

type NoEventReason string

const (
	NoAnomalyPoint    NoEventReason = "no anomaly point for this alert"
	InvalidType       NoEventReason = "rule type not support"
	InvalidRuleConfig NoEventReason = "rule config invalid"
	InvalidQuery      NoEventReason = "invalid promql query"
	EmptyPromClient   NoEventReason = "empty prom client"
	QueryErr          NoEventReason = "query error, query: %+v, err: %s"
	TargetNotFound    NoEventReason = "targets not found"
	EventRecovered    NoEventReason = "event recovered"
	RecoverMerge      NoEventReason = "recover event merged"
	RecoverDuration   NoEventReason = "within recover duration"
	RuleNotFound      NoEventReason = "rule not found"
	IsInhibit         NoEventReason = "alert is inhibited by high priority"
	Muted             NoEventReason = "alert is muted, detail: %s"
	MutedByHook       NoEventReason = "alert is muted by hook"
	FullQueue         NoEventReason = "alert queue is full"
	Interval          NoEventReason = "fail to reach alert interval"
	RepeatStep        NoEventReason = "fail to reach repeat step"
	NotifyNumber      NoEventReason = "reach max notify number"
)

type AlertRecord struct {
	AlertName        string        `json:"alert_name"`
	CreateAt         int64         `json:"create_at"`
	SendEvent        bool          `json:"send_event"`
	ReasonForNoEvent NoEventReason `json:"reason_for_no_event"`
	IsRecovery       bool          `json:"is_recovery"`
	Labels           string        `json:"labels"`
	Query            string        `json:"query"`
	Values           string        `json:"values"`
}

func (ar *AlertRecord) MarshalBinary() (data []byte, err error) {
	return json.Marshal(ar)
}

type AlertRecordRedis struct {
	Key            string        `json:"key"`
	Value          *AlertRecord  `json:"value"`
	ExpireDuration time.Duration `json:"expire_duration"`
	MaxLength      int64         `json:"max_length"`
}

func Record(ctx *ctx.Context, point *common.AnomalyPoint, event *models.AlertCurEvent, isRecovery bool, reason NoEventReason, ruleName string, ruleID int64, redis *storage.Redis) {
	// 开始调度后，告警检测有 4 种情况
	// 1. 没有生成 point
	// 2. 生成了 point 但没有生成 event
	// 3. 生成了 event 但没有投递到队列
	// 4. 生成了 event 且投递到队列

	var ar AlertRecord

	ar.ReasonForNoEvent = reason
	ar.IsRecovery = isRecovery
	ar.AlertName = ruleName
	now := time.Now()
	key := fmt.Sprintf(AlertRecordRedisKey, ruleID, now.Day(), now.Hour())

	if event != nil {
		// 生成了 event 但没有投递
		ar.CreateAt = event.TriggerTime
		ar.Labels = event.Tags
		ar.Query = event.PromQl
		ar.Values = event.TriggerValue

	} else if point != nil {
		// 生成了 point 但没有生成 event
		ar.CreateAt = point.Timestamp
		ar.Labels = point.Labels.String()
		ar.Query = point.Query
		ar.Values = point.ReadableValue()
		return
	} else {
		// 没有生成 point
	}

	if !ctx.IsCenter {
		err := poster.PostByUrls(ctx, "/v1/n9e/redis/lpush", AlertRecordRedis{
			Key:            key,
			Value:          &ar,
			ExpireDuration: defaultTimeDurationForList,
			MaxLength:      defaultMaxLengthForList,
		})
		if err != nil {
			logger.Errorf("fail to forward alert record, err:%s", err.Error())
		}
		return
	}
	err := PushAlertRecord(ctx, redis, key, &ar)
	if err != nil {
		logger.Errorf("fail to push alert record, err:%s", err.Error())
	}
}

func PushAlertRecord(ctx *ctx.Context, redis *storage.Redis, key string, ar *AlertRecord) error {
	err := storage.LPush(ctx.GetContext(), *redis, key, ar)
	if err != nil {
		return err
	}
	var count int64
	val, ok := AlertRecordCount.Load(key)
	if !ok || val.(int64) == 0 {
		err := storage.Expire(ctx.GetContext(), *redis, key, defaultTimeDurationForList)
		if err != nil {
			logger.Errorf("fail to set expire time for alert record, key :%s, err:%s", key, err.Error())
			return err
		}
		AlertRecordCount.Store(key, int64(1))
	} else {
		count = val.(int64) + 1
		AlertRecordCount.Store(key, count)
	}

	if count > AlertRecordMaxCount {
		err := storage.LTrim(ctx.GetContext(), *redis, key, 0, AlertRecordMaxCount-1)
		if err != nil {
			logger.Errorf("fail to trim alert record, key :%s, err:%s", key, err.Error())
			return err
		}
		AlertRecordCount.Store(key, AlertRecordMaxCount)
	}
	return nil
}

func AlertMutedReason(detail string) NoEventReason {
	return NoEventReason(fmt.Sprintf(string(Muted), detail))
}

func QueryError(query interface{}, err error) NoEventReason {
	return NoEventReason(fmt.Sprintf(string(QueryErr), query, err.Error()))
}

type keyAndLen struct {
	key    string
	length int64
}

// LimitAlertRecordCount drop keys when alert record's count exceed AlertRecordMaxCount
func LimitAlertRecordCount(ctx *ctx.Context, redis *storage.Redis) {
	var keys []string
	// 查出内存中维护的所有 key 对应的过期时间以及 list 长度
	AlertRecordCount.Range(func(k, v interface{}) bool {
		keys = append(keys, k.(string))
		return true
	})

	kal := make(map[int][]keyAndLen)
	var count int64
	kToLen, err := storage.MLLen(ctx.GetContext(), *redis, keys)
	if err != nil {
		logger.Errorf("fail to limit alert record's count, err:%s", err.Error())
		return
	}
	kToTTL, err := storage.MTTL(ctx.GetContext(), *redis, keys)
	if err != nil {
		logger.Errorf("fail to limit alert record's count, err:%s", err.Error())
		return
	}
	// 按照过期时间将 key 分组
	for k, v := range kToTTL {
		l := kToLen[k]
		if v < 0 {
			// 不存在/已经过期的 key 不再维护
			AlertRecordCount.Delete(k)
		}
		if l == 0 || v < 0 {
			continue
		}
		hour := int(v.Hours())
		if _, ok := kal[hour]; !ok {
			kal[hour] = make([]keyAndLen, 0)
		}
		count += l
		kal[hour] = append(kal[hour], keyAndLen{key: k, length: l})
	}

	if count <= AlertRecordMaxCount {
		return
	}
	// 如果阈值超过上限，以小时为粒度依次淘汰 key
	keyDel := make([]string, 0)
	for i := 0; i < 24; i++ {
		if count <= AlertRecordMaxCount {
			break
		}
		if len(kal[i]) == 0 {
			continue
		}
		for j := 0; j < len(kal[i]); j++ {
			keyDel = append(keyDel, kal[i][j].key)
			count -= kal[i][j].length
		}
	}

	err = storage.MDel(ctx.GetContext(), *redis, keyDel...)
	if err != nil {
		logger.Errorf("fail to limit alert record's count, err:%s", err.Error())
	}
	for i := range keyDel {
		AlertRecordCount.Delete(keyDel[i])
	}
}
