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

	rrc.scheduler = cron.New(cron.WithSeconds())
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
	return str.MD5(fmt.Sprintf("%d_%s_%s_%d_%s",
		rrc.rule.Id,
		rrc.rule.CronPattern,
		rrc.rule.PromQl,
		rrc.datasourceId,
		rrc.rule.AppendTags,
	))
}

func (rrc *RecordRuleContext) Prepare() {}

func (rrc *RecordRuleContext) Start() {
	logger.Infof("eval:%s started", rrc.Key())
	rrc.scheduler.Start()
}

func (rrc *RecordRuleContext) Eval() {
	rrc.stats.CounterRecordEval.WithLabelValues(fmt.Sprintf("%d", rrc.datasourceId)).Inc()
	promql := strings.TrimSpace(rrc.rule.PromQl)
	if promql == "" {
		logger.Errorf("eval:%s promql is blank", rrc.Key())
		return
	}

	if rrc.promClients.IsNil(rrc.datasourceId) {
		logger.Errorf("eval:%s reader client is nil", rrc.Key())
		rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(fmt.Sprintf("%d", rrc.datasourceId)).Inc()
		return
	}

	value, warnings, err := rrc.promClients.GetCli(rrc.datasourceId).Query(context.Background(), promql, time.Now())
	if err != nil {
		logger.Errorf("eval:%s promql:%s, error:%v", rrc.Key(), promql, err)
		rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(fmt.Sprintf("%d", rrc.datasourceId)).Inc()
		return
	}

	if len(warnings) > 0 {
		logger.Errorf("eval:%s promql:%s, warnings:%v", rrc.Key(), promql, warnings)
		rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(fmt.Sprintf("%d", rrc.datasourceId)).Inc()
		return
	}

	ts := ConvertToTimeSeries(value, rrc.rule)
	if len(ts) != 0 {
		err := rrc.promClients.GetWriterCli(rrc.datasourceId).Write(ts)
		if err != nil {
			logger.Errorf("eval:%s promql:%s, error:%v", rrc.Key(), promql, err)
			rrc.stats.CounterRecordEvalErrorTotal.WithLabelValues(fmt.Sprintf("%d", rrc.datasourceId)).Inc()
		}
	}
}

func (rrc *RecordRuleContext) Stop() {
	logger.Infof("%s stopped", rrc.Key())

	c := rrc.scheduler.Stop()
	<-c.Done()
	close(rrc.quit)
}
