package judge

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/prometheus/promql"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/backend"
	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/vos"
)

const (
	DEFAULT_PULL_ALERT_INTERVAL = 15
	LABEL_NAME                  = "__name__"
)

type RuleManager struct {
	targetMtx   sync.Mutex
	activeRules map[string]RuleEval
}

var pullRuleManager = NewRuleManager()

func NewRuleManager() *RuleManager {
	return &RuleManager{
		activeRules: make(map[string]RuleEval),
	}
}

type RuleEval struct {
	R         models.AlertRule
	quiteChan chan struct{}
	ctx       context.Context
}

func (re RuleEval) start() {
	go func(re RuleEval) {
		logger.Debugf("[prome_pull_alert_start][RuleEval: %+v]", re)
		if re.R.PullExpr.EvaluationInterval <= 0 {
			re.R.PullExpr.EvaluationInterval = DEFAULT_PULL_ALERT_INTERVAL
		}

		sleepDuration := time.Duration(re.R.PullExpr.EvaluationInterval) * time.Second

		for {
			select {
			case <-re.ctx.Done():
				return
			case <-re.quiteChan:
				return
			default:
			}

			// 获取backend的prometheus DataSource
			pb, err := backend.GetDataSourceFor("prometheus")
			if err != nil {
				logger.Errorf("[pull_alert][get_prome_datasource_error][err: %v]", err)
				return
			}

			// 调prometheus instance query 查询数据
			promVector := pb.QueryVector(re.R.PullExpr.PromQl)

			handlePromqlVector(promVector, re.R)

			time.Sleep(sleepDuration)
		}
	}(re)
}

func (r RuleEval) stop() {
	logger.Debugf("[prome_pull_alert_stop][RuleEval: %+v]", r)
	close(r.quiteChan)
}

func (rm *RuleManager) SyncRules(ctx context.Context, rules []models.AlertRule) {

	thisNewRules := make(map[string]RuleEval)
	thisAllRules := make(map[string]RuleEval)

	rm.targetMtx.Lock()
	for _, r := range rules {
		newR := RuleEval{
			R:         r,
			quiteChan: make(chan struct{}, 1),
			ctx:       ctx,
		}
		hash := str.MD5(fmt.Sprintf("rid_%d_%d_%d_%s",
			r.Id,
			r.AlertDuration,
			r.PullExpr.EvaluationInterval,
			r.PullExpr.PromQl,
		))
		thisAllRules[hash] = newR
		if _, loaded := rm.activeRules[hash]; !loaded {
			thisNewRules[hash] = newR
			rm.activeRules[hash] = newR
		}
	}

	// 停止旧的
	for hash := range rm.activeRules {
		if _, loaded := thisAllRules[hash]; !loaded {
			rm.activeRules[hash].stop()
			delete(rm.activeRules, hash)
		}
	}
	rm.targetMtx.Unlock()

	// 开启新的
	for hash := range thisNewRules {
		thisNewRules[hash].start()
	}
}

func handlePromqlVector(pv promql.Vector, r models.AlertRule) {
	toKeepKeys := map[string]struct{}{}
	if len(pv) == 0 {
		// 说明没触发，或者没查询到，删掉rule-id开头的所有event
		LastEvents.DeleteOrSendRecovery(r.Id, toKeepKeys)

		return
	}

	for _, s := range pv {
		readableStr := s.Metric.String()

		value := fmt.Sprintf("[vector=%s]: [value=%f]", readableStr, s.Point.V)
		hashId := str.MD5(fmt.Sprintf("s_%d_%s", r.Id, readableStr))
		toKeepKeys[hashId] = struct{}{}
		tags := ""
		tagm := make(map[string]string)
		metricsName := ""
		for _, l := range s.Metric {
			if l.Name == LABEL_NAME {
				metricsName = l.Value
				continue
			}
			tags += fmt.Sprintf("%s=%s,", l.Name, l.Value)
			tagm[l.Name] = l.Value

		}

		tags = strings.TrimRight(tags, ",")
		// prometheus查询返回 13位时间戳
		triggerTs := s.T / 1e3
		//triggerTs := time.Now().Unix()
		historyArr := make([]vos.HistoryPoints, 0)

		hp := &vos.HPoint{
			Timestamp: triggerTs,
			Value:     vos.JsonFloat(s.V),
		}
		historyArr = append(historyArr, vos.HistoryPoints{
			Metric: metricsName,
			Tags:   tagm,
			Points: []*vos.HPoint{hp},
		})
		bs, err := json.Marshal(historyArr)
		if err != nil {
			logger.Errorf("[pull_alert][historyArr_json_Marshal_error][historyArr:%+v][err: %v]", historyArr, err)
			return
		}
		logger.Debugf("[proml.historyArr][metricsName:%v][Tags:%v]\n", metricsName, tagm)

		event := &models.AlertEvent{
			RuleId:             r.Id,
			RuleName:           r.Name,
			RuleNote:           r.Note,
			HashId:             hashId,
			IsPromePull:        1,
			IsRecovery:         0,
			Priority:           r.Priority,
			HistoryPoints:      bs,
			TriggerTime:        triggerTs,
			Values:             value,
			NotifyChannels:     r.NotifyChannels,
			NotifyGroups:       r.NotifyGroups,
			NotifyUsers:        r.NotifyUsers,
			RunbookUrl:         r.RunbookUrl,
			ReadableExpression: r.PullExpr.PromQl,
			Tags:               tags,
			AlertDuration:      int64(r.AlertDuration),
			TagMap:             tagm,
		}

		logger.Debugf("[handlePromqlVector_has_value][event:%+v]\n", event)
		sendEventIfNeed([]bool{true}, event, &r)
	}
	LastEvents.DeleteOrSendRecovery(r.Id, toKeepKeys)

}
