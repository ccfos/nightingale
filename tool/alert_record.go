package tool

import (
	"encoding/json"
	"fmt"
	"github.com/ccfos/nightingale/v6/alert/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/toolkits/pkg/logger"
	"time"
)

var AlertRecordRedisKeyPrefix = "alert::*"
var AlertRecordRedisKey = "alert::%d::%d::%d"
var AlertRecordMaxCount int64 = 300
var defaultMaxLengthForList int64 = 1000
var defaultTimeDurationForList = 24 * time.Hour

type NoEventReason string

const (
	NoAnomalyPoint  NoEventReason = "no anomaly point for this alert"
	InvalidType     NoEventReason = "rule type not support"
	EventRecovered  NoEventReason = "event recovered"
	RecoverMerge    NoEventReason = "recover event merged"
	RecoverDuration NoEventReason = "within recover duration"
	RuleNotFound    NoEventReason = "rule not found"
	IsInhibit       NoEventReason = "alert is inhibited by high priority"
	Muted           NoEventReason = "alert is muted, detail: %s"
	MutedByHook     NoEventReason = "alert is muted by hook"
	FullQueue       NoEventReason = "alert queue is full"
	Interval        NoEventReason = "fail to reach alert interval"
	RepeatStep      NoEventReason = "fail to reach repeat step"
	NotifyNumber    NoEventReason = "reach max notify number"
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

func Record(ctx *ctx.Context, point *common.AnomalyPoint, event *models.AlertCurEvent, isRecovery bool, reason NoEventReason, ruleName string, ruleID int64) {
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
	defer func(ar *AlertRecord) {
		// redis
		if !ctx.IsCenter {
			err := poster.PostByUrls(ctx, "/v1/n9e/redis/lpush", AlertRecordRedis{
				Key:            key,
				Value:          ar,
				ExpireDuration: defaultTimeDurationForList,
				MaxLength:      defaultMaxLengthForList,
			})
			if err != nil {
				logger.Errorf("fail to forward alert record, err:%s", err.Error())
			}
			return
		}
		err := storage.LPush(ctx.GetContext(), *ctx.Redis, defaultMaxLengthForList, defaultTimeDurationForList, key, ar)
		if err != nil {
			logger.Errorf("fail to add alert record, err:%s", err.Error())
		}
	}(&ar)

	if event != nil {
		// 生成了 event 但没有投递
		ar.CreateAt = event.TriggerTime
		ar.Labels = event.Tags
		ar.Query = event.PromQl
		ar.Values = event.TriggerValue
		return
	}

	if point != nil {
		// 生成了 point 但没有生成 event
		ar.CreateAt = point.Timestamp
		ar.Labels = point.Labels.String()
		ar.Query = point.Query
		ar.Values = point.ReadableValue()
		return
	}

	// 没有生成 point
}

func AlertMutedReason(detail string) NoEventReason {
	return NoEventReason(fmt.Sprintf(string(Muted), detail))
}

type keyAndLen struct {
	key    string
	length int64
}

// LimitAlertRecordCount drop keys when alert record's count exceed AlertRecordMaxCount
func LimitAlertRecordCount(ctx *ctx.Context) {
	var cursor uint64
	var keys []string
	for {
		scanKeys, cursor, err := storage.Scan(ctx.GetContext(), *ctx.Redis, cursor, AlertRecordRedisKeyPrefix, 100)
		if err != nil {
			logger.Errorf("fail to limit alert record's count, err:%s", err.Error())
			return
		}
		keys = append(keys, scanKeys...)
		if cursor == 0 {
			break
		}
	}

	kal := make(map[int][]keyAndLen)
	var count int64
	for i := range keys {
		lLen, err := storage.LLen(ctx.GetContext(), *ctx.Redis, keys[i])
		if err != nil {
			logger.Errorf("failed to get length of key %s: %v", keys[i], err)
		}
		count += lLen
		ttl, err := storage.TTL(ctx.GetContext(), *ctx.Redis, keys[i])
		if err != nil {
			logger.Errorf("failed to get ttl of key %s: %v", keys[i], err)
		}
		ttlHour := int(ttl.Hours())
		if _, ok := kal[ttlHour]; !ok {
			kal[ttlHour] = make([]keyAndLen, 0)
		}
		kal[ttlHour] = append(kal[ttlHour], keyAndLen{
			key:    keys[i],
			length: lLen,
		})
	}

	if count <= AlertRecordMaxCount {
		return
	}

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

	err := storage.MDel(ctx.GetContext(), *ctx.Redis, keyDel...)
	if err != nil {
		logger.Errorf("fail to limit alert record's count, err:%s", err.Error())
	}
}
