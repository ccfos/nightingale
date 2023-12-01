package alert

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/dispatch"
	"github.com/ccfos/nightingale/v6/alert/eval"
	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/alert/record"
	"github.com/ccfos/nightingale/v6/alert/router"
	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
	"github.com/ccfos/nightingale/v6/tdengine"
)

func Initialize(configDir string, cryptoKey string) (func(), error) {
	config, err := conf.InitConfig(configDir, cryptoKey)
	if err != nil {
		return nil, fmt.Errorf("failed to init config: %v", err)
	}

	logxClean, err := logx.Init(config.Log)
	if err != nil {
		return nil, err
	}

	ctx := ctx.NewContext(context.Background(), nil, false, config.CenterApi)

	syncStats := memsto.NewSyncStats()
	alertStats := astats.NewSyncStats()

	configCache := memsto.NewConfigCache(ctx, syncStats, nil, "")
	targetCache := memsto.NewTargetCache(ctx, syncStats, nil)
	busiGroupCache := memsto.NewBusiGroupCache(ctx, syncStats)
	alertMuteCache := memsto.NewAlertMuteCache(ctx, syncStats)
	alertRuleCache := memsto.NewAlertRuleCache(ctx, syncStats)
	notifyConfigCache := memsto.NewNotifyConfigCache(ctx, configCache)
	dsCache := memsto.NewDatasourceCache(ctx, syncStats)
	userCache := memsto.NewUserCache(ctx, syncStats)
	userGroupCache := memsto.NewUserGroupCache(ctx, syncStats)

	promClients := prom.NewPromClient(ctx)
	tdengineClients := tdengine.NewTdengineClient(ctx, config.Alert.Heartbeat)

	externalProcessors := process.NewExternalProcessors()

	Start(config.Alert, config.Pushgw, syncStats, alertStats, externalProcessors, targetCache, busiGroupCache, alertMuteCache, alertRuleCache, notifyConfigCache, dsCache, ctx, promClients, tdengineClients, userCache, userGroupCache)

	r := httpx.GinEngine(config.Global.RunMode, config.HTTP)
	rt := router.New(config.HTTP, config.Alert, alertMuteCache, targetCache, busiGroupCache, alertStats, ctx, externalProcessors)
	rt.Config(r)
	dumper.ConfigRouter(r)

	httpClean := httpx.Init(config.HTTP, r)

	return func() {
		logxClean()
		httpClean()
	}, nil
}

func Start(alertc aconf.Alert, pushgwc pconf.Pushgw, syncStats *memsto.Stats, alertStats *astats.Stats, externalProcessors *process.ExternalProcessorsType, targetCache *memsto.TargetCacheType, busiGroupCache *memsto.BusiGroupCacheType,
	alertMuteCache *memsto.AlertMuteCacheType, alertRuleCache *memsto.AlertRuleCacheType, notifyConfigCache *memsto.NotifyConfigCacheType, datasourceCache *memsto.DatasourceCacheType, ctx *ctx.Context,
	promClients *prom.PromClientMap, tdendgineClients *tdengine.TdengineClientMap, userCache *memsto.UserCacheType, userGroupCache *memsto.UserGroupCacheType) {
	alertSubscribeCache := memsto.NewAlertSubscribeCache(ctx, syncStats)
	recordingRuleCache := memsto.NewRecordingRuleCache(ctx, syncStats)

	go models.InitNotifyConfig(ctx, alertc.Alerting.TemplatesDir)

	naming := naming.NewNaming(ctx, alertc.Heartbeat)

	writers := writer.NewWriters(pushgwc)
	record.NewScheduler(alertc, recordingRuleCache, promClients, writers, alertStats)

	eval.NewScheduler(alertc, externalProcessors, alertRuleCache, targetCache, busiGroupCache, alertMuteCache, datasourceCache, promClients, tdendgineClients, naming, ctx, alertStats)

	dp := dispatch.NewDispatch(alertRuleCache, userCache, userGroupCache, alertSubscribeCache, targetCache, notifyConfigCache, alertc.Alerting, ctx, alertStats)
	consumer := dispatch.NewConsumer(alertc.Alerting, ctx, dp)

	go dp.ReloadTpls()
	go consumer.LoopConsume()

	go queue.ReportQueueSize(alertStats)
	go sender.InitEmailSender(notifyConfigCache.GetSMTP())
}
