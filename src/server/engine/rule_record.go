package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/common/conv"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/writer"
)

type RecordRuleContext struct {
	cluster string
	quit    chan struct{}

	rule *models.RecordingRule
}

func NewRecordRuleContext(rule *models.RecordingRule, cluster string) *RecordRuleContext {
	return &RecordRuleContext{
		cluster: cluster,
		quit:    make(chan struct{}),
		rule:    rule,
	}
}

func (rrc *RecordRuleContext) Key() string {
	return fmt.Sprintf("record-%s-%d", rrc.cluster, rrc.rule.Id)
}

func (rrc *RecordRuleContext) Hash() string {
	return str.MD5(fmt.Sprintf("%d_%d_%s_%s",
		rrc.rule.Id,
		rrc.rule.PromEvalInterval,
		rrc.rule.PromQl,
		rrc.cluster,
	))
}

func (rrc *RecordRuleContext) Prepare() {}

func (rrc *RecordRuleContext) Start() {
	logger.Infof("eval:%s started", rrc.Key())
	interval := rrc.rule.PromEvalInterval
	if interval <= 0 {
		interval = 10
	}
	go func() {
		for {
			select {
			case <-rrc.quit:
				return
			default:
				rrc.Eval()
				time.Sleep(time.Duration(interval) * time.Second)
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

	if config.ReaderClients.IsNil(rrc.cluster) {
		logger.Errorf("eval:%s reader client is nil", rrc.Key())
		return
	}

	value, warnings, err := config.ReaderClients.GetCli(rrc.cluster).Query(context.Background(), promql, time.Now())
	if err != nil {
		logger.Errorf("eval:%d promql:%s, error:%v", rrc.Key(), promql, err)
		return
	}

	if len(warnings) > 0 {
		logger.Errorf("eval:%d promql:%s, warnings:%v", rrc.Key(), promql, warnings)
		return
	}
	ts := conv.ConvertToTimeSeries(value, rrc.rule)
	if len(ts) != 0 {
		for _, v := range ts {
			writer.Writers.PushSample(rrc.rule.Name, v, rrc.cluster)
		}
	}
}

func (rrc *RecordRuleContext) Stop() {
	logger.Infof("%s stopped", rrc.Key())
	close(rrc.quit)
}
