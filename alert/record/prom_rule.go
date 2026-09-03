package record

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
	"github.com/robfig/cron/v3"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

const (
	StageCheckQuery = "check_query"
	StageGetClient  = "get_client"
	StageQueryData  = "query_data"
	StageWriteData  = "write_data"
)

type RecordRuleContext struct {
	datasourceId int64
	quit         chan struct{}

	scheduler   *cron.Cron
	rule        *models.RecordingRule
	promClients *prom.PromClientMap
	stats       *astats.Stats
}

func NewRecordRuleContext(rule *models.RecordingRule, datasourceId int64, promClients *prom.PromClientMap, writers *writer.WritersType, stats *astats.Stats) *RecordRuleContext {
	rrc := &RecordRuleContext{
		datasourceId: datasourceId,
		quit:         make(chan struct{}),
		rule:         rule,
		promClients:  promClients,
		stats:        stats,
	}

	if rule.CronPattern == "" && rule.PromEvalInterval != 0 {
		rule.CronPattern = fmt.Sprintf("@every %ds", rule.PromEvalInterval)
	}

	rrc.scheduler = cron.New(cron.WithSeconds(), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	_, err := rrc.scheduler.AddFunc(rule.CronPattern, func() {
		rrc.Eval()
	})

	if err != nil {
		logger.Errorf("add cron pattern error: %v", err)
	}

	return rrc
}

func (rrc *RecordRuleContext) Key() string {
	return fmt.Sprintf("record-%d-%d", rrc.datasourceId, rrc.rule.Id)
}

func (rrc *RecordRuleContext) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%s_%s_%d_%s_%s",
		rrc.rule.Id,
		rrc.rule.CronPattern,
		rrc.rule.PromQl,
		rrc.datasourceId,
		rrc.rule.AppendTags,
		rrc.rule.Name,
	))
}

func (rrc *RecordRuleContext) Prepare() {}

func (rrc *RecordRuleContext) Start() {
	logger.Infof("eval:%s started", rrc.Key())
	rrc.scheduler.Start()
}

func (rrc *RecordRuleContext) Eval() {
	begin := time.Now()
	dsIdStr := fmt.Sprintf("%d", rrc.datasourceId)
	ruleIdStr := fmt.Sprintf("%d", rrc.rule.Id)
	var message string

	defer func() {
		rrc.stats.GaugeRecordEvalDuration.WithLabelValues(ruleIdStr, dsIdStr).Set(float64(time.Since(begin).Milliseconds()))
		if len(message) == 0 {
			logger.Infof("record_eval:%s finished, duration:%v", rrc.Key(), time.Since(begin))
		} else {
			logger.Warningf("record_eval:%s finished, duration:%v, message:%s", rrc.Key(), time.Since(begin), message)
		}
	}()

	rrc.stats.CounterRecordEval.WithLabelValues(dsIdStr, ruleIdStr).Inc()

	promql := strings.TrimSpace(rrc.rule.PromQl)
	if promql == "" {
		message = "promql is blank"
		logger.Errorf("record_eval:%s promql is blank", rrc.Key())
		rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(dsIdStr, dsIdStr, StageCheckQuery, ruleIdStr).Inc()
		return
	}

	if rrc.promClients.IsNil(rrc.datasourceId) {
		message = "reader client is nil"
		logger.Errorf("record_eval:%s reader client is nil", rrc.Key())
		rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(dsIdStr, dsIdStr, StageGetClient, ruleIdStr).Inc()
		rrc.stats.GaugeRecordSeriesCount.WithLabelValues(ruleIdStr, dsIdStr).Set(-2)
		return
	}

	value, warnings, err := rrc.promClients.GetCli(rrc.datasourceId).Query(context.Background(), promql, time.Now())
	if err != nil {
		message = fmt.Sprintf("query error: %v", err)
		logger.Errorf("record_eval:%s promql:%s, error:%v", rrc.Key(), promql, err)
		rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(dsIdStr, dsIdStr, StageQueryData, ruleIdStr).Inc()
		rrc.stats.GaugeRecordSeriesCount.WithLabelValues(ruleIdStr, dsIdStr).Set(-1)
		return
	}

	if len(warnings) > 0 {
		message = fmt.Sprintf("query warnings: %v", warnings)
		logger.Errorf("record_eval:%s promql:%s, warnings:%v", rrc.Key(), promql, warnings)
		rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(dsIdStr, dsIdStr, StageQueryData, ruleIdStr).Inc()
		rrc.stats.GaugeRecordSeriesCount.WithLabelValues(ruleIdStr, dsIdStr).Set(-1)
		return
	}

	ts := ConvertToTimeSeries(value, rrc.rule)
	rrc.stats.GaugeRecordSeriesCount.WithLabelValues(ruleIdStr, dsIdStr).Set(float64(len(ts)))

	if len(ts) != 0 {
		err := rrc.promClients.GetWriterCli(rrc.datasourceId).Write(ts)
		if err != nil {
			message = fmt.Sprintf("write error: %v", err)
			logger.Errorf("record_eval:%s promql:%s, error:%v", rrc.Key(), promql, err)
			rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(dsIdStr, dsIdStr, StageWriteData, ruleIdStr).Inc()
		}
	}
}

func (rrc *RecordRuleContext) Stop() {
	logger.Infof("%s stopped", rrc.Key())

	c := rrc.scheduler.Stop()
	<-c.Done()
	close(rrc.quit)
}
