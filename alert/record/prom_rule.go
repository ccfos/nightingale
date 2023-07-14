package record

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/pushgw/writer"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
)

type RecordRuleContext struct {
	datasourceId int64
	quit         chan struct{}

	rule *models.RecordingRule
	// writers     *writer.WritersType
	promClients *prom.PromClientMap
}

func NewRecordRuleContext(rule *models.RecordingRule, datasourceId int64, promClients *prom.PromClientMap, writers *writer.WritersType) *RecordRuleContext {
	return &RecordRuleContext{
		datasourceId: datasourceId,
		quit:         make(chan struct{}),
		rule:         rule,
		promClients:  promClients,
		//writers:      writers,
	}
}

func (rrc *RecordRuleContext) Key() string {
	return fmt.Sprintf("record-%d-%d", rrc.datasourceId, rrc.rule.Id)
}

func (rrc *RecordRuleContext) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%d_%s_%d",
		rrc.rule.Id,
		rrc.rule.PromEvalInterval,
		rrc.rule.PromQl,
		rrc.datasourceId,
	))
}

func (rrc *RecordRuleContext) Prepare() {}

func (rrc *RecordRuleContext) Start() {
	logger.Infof("eval:%s started", rrc.Key())
	interval := rrc.rule.PromEvalInterval
	if interval <= 0 {
		interval = 10
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-rrc.quit:
				return
			case <-ticker.C:
				rrc.Eval()
			}
		}
	}()
}

func (rrc *RecordRuleContext) Eval() {
	promql := strings.TrimSpace(rrc.rule.PromQl)
	if promql == "" {
		logger.Errorf("eval:%s promql is blank", rrc.Key())
		return
	}

	if rrc.promClients.IsNil(rrc.datasourceId) {
		logger.Errorf("eval:%s reader client is nil", rrc.Key())
		return
	}

	value, warnings, err := rrc.promClients.GetCli(rrc.datasourceId).Query(context.Background(), promql, time.Now())
	if err != nil {
		logger.Errorf("eval:%s promql:%s, error:%v", rrc.Key(), promql, err)
		return
	}

	if len(warnings) > 0 {
		logger.Errorf("eval:%s promql:%s, warnings:%v", rrc.Key(), promql, warnings)
		return
	}

	ts := ConvertToTimeSeries(value, rrc.rule)
	if len(ts) != 0 {
		rrc.promClients.GetWriterCli(rrc.datasourceId).Write(ts)
	}
}

func (rrc *RecordRuleContext) Stop() {
	logger.Infof("%s stopped", rrc.Key())
	close(rrc.quit)
}
