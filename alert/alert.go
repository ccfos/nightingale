package alert

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/dscache"

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
	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/pkg/macros"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/flashcatcloud/ibex/src/cmd/ibex"
	"github.com/toolkits/pkg/logger"
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

	var redis storage.Redis
	redis, err = storage.NewRedis(config.Redis)
	if err != nil {
		return nil, err
	}

	syncStats := memsto.NewSyncStats()
	alertStats := astats.NewSyncStats()

	configCache := memsto.NewConfigCache(ctx, syncStats, nil, "")
	targetCache := memsto.NewTargetCache(ctx, syncStats, redis)
	busiGroupCache := memsto.NewBusiGroupCache(ctx, syncStats)
	alertMuteCache := memsto.NewAlertMuteCache(ctx, syncStats)
	alertRuleCache := memsto.NewAlertRuleCache(ctx, syncStats)
	notifyConfigCache := memsto.NewNotifyConfigCache(ctx, configCache)
	dsCache := memsto.NewDatasourceCache(ctx, syncStats)
	userCache := memsto.NewUserCache(ctx, syncStats)
	userGroupCache := memsto.NewUserGroupCache(ctx, syncStats)
	taskTplsCache := memsto.NewTaskTplCache(ctx)
	configCvalCache := memsto.NewCvalCache(ctx, syncStats)
	notifyRuleCache := memsto.NewNotifyRuleCache(ctx, syncStats)
	notifyChannelCache := memsto.NewNotifyChannelCache(ctx, syncStats)
	messageTemplateCache := memsto.NewMessageTemplateCache(ctx, syncStats)

	promClients := prom.NewPromClient(ctx)
	dispatch.InitRegisterQueryFunc(promClients)

	externalProcessors := process.NewExternalProcessors()

	macros.RegisterMacro(macros.ExpandTimeFilter)
	dscache.Init(ctx, false, config.Alert.Heartbeat.EngineName)
	Start(config.Alert, config.Pushgw, syncStats, alertStats, externalProcessors, targetCache, busiGroupCache, alertMuteCache, alertRuleCache, notifyConfigCache, taskTplsCache, dsCache, ctx, promClients, userCache, userGroupCache, notifyRuleCache, notifyChannelCache, messageTemplateCache, configCvalCache)

	r := httpx.GinEngine(config.Global.RunMode, config.HTTP,
		configCvalCache.PrintBodyPaths, configCvalCache.PrintAccessLog)
	rt := router.New(config.HTTP, config.Alert, alertMuteCache, targetCache, busiGroupCache, alertStats, ctx, externalProcessors, config.Log.Dir)

	if config.Ibex.Enable {
		ibex.ServerStart(false, nil, redis, config.HTTP.APIForService.BasicAuth, config.Alert.Heartbeat, &config.CenterApi, r, nil, config.Ibex, config.HTTP.Port)
	}

	rt.Config(r)
	dumper.ConfigRouter(r)

	httpClean := httpx.Init(config.HTTP, r)

	return func() {
		// evallog 的写入队列与各文件的 64KB 缓冲只有 Shutdown 会排空，
		// 不调用就等于每次正常退出都丢掉最近一批评估记录——而重启前后恰恰最需要现场。
		// 必须排在 logxClean 之前，否则刷盘阶段的告警日志也一并丢了。
		evallog.Shutdown()
		logxClean()
		httpClean()
	}, nil
}

func Start(alertc aconf.Alert, pushgwc pconf.Pushgw, syncStats *memsto.Stats, alertStats *astats.Stats, externalProcessors *process.ExternalProcessorsType, targetCache *memsto.TargetCacheType, busiGroupCache *memsto.BusiGroupCacheType,
	alertMuteCache *memsto.AlertMuteCacheType, alertRuleCache *memsto.AlertRuleCacheType, notifyConfigCache *memsto.NotifyConfigCacheType, taskTplsCache *memsto.TaskTplCache, datasourceCache *memsto.DatasourceCacheType, ctx *ctx.Context,
	promClients *prom.PromClientMap, userCache *memsto.UserCacheType, userGroupCache *memsto.UserGroupCacheType, notifyRuleCache *memsto.NotifyRuleCacheType, notifyChannelCache *memsto.NotifyChannelCacheType, messageTemplateCache *memsto.MessageTemplateCacheType, configCvalCache *memsto.CvalCache) {
	alertSubscribeCache := memsto.NewAlertSubscribeCache(ctx, syncStats)
	recordingRuleCache := memsto.NewRecordingRuleCache(ctx, syncStats)
	targetsOfAlertRulesCache := memsto.NewTargetOfAlertRuleCache(ctx, alertc.Heartbeat.EngineName, syncStats)

	// 评估执行记录：本地文件存储，支持按规则+时间范围查询评估现场
	if err := evallog.Init(alertc.EvalLog, evallog.Hooks{
		OnDrop:        func() { alertStats.CounterEvalLogDropTotal.Inc() },
		OnQueryReject: func() { alertStats.CounterEvalLogQueryRejectTotal.Inc() },
	}); err != nil {
		logger.Errorf("failed to init evallog: %v", err)
	}

	go models.InitNotifyConfig(ctx, alertc.Alerting.TemplatesDir)
	go models.InitMessageTemplate(ctx)
	go models.InitNotifyChannel(ctx)
	models.VerifyByProvider = provider.VerifyChannelConfig

	// 通知媒介的 URL / 请求头 / 查询参数 / 请求体可以引用「变量配置」里的变量（{{.my_token}}），
	// 凭证因此不必明文写在媒介配置里。这里把变量缓存挂给 provider 包，避免为传递变量去改
	// BuildNotifyContext / SendByNotifyRule 的签名（后者 n9e-plus 侧有自己的实现）。
	// 开源的 center / 独立 alert / edge 都经由本函数启动，注册一次即覆盖；n9e-plus 不调用
	// 本函数，需在其 plus.go 与 alert.Start 里各自注册，否则变量静默渲染成空串。
	if notifyConfigCache != nil && notifyConfigCache.ConfigCache != nil {
		provider.UserVariableGetter = notifyConfigCache.ConfigCache.Get
	}

	naming := naming.NewNaming(ctx, alertc.Heartbeat, alertStats)
	// TODO(dingtalkapp): 钉钉应用本次不上线，先屏蔽 Stream 主备选举入口，避免启动回调长连接；待上线时恢复本行。
	// notifyChannelCache.SetDingtalkLeaderNaming(naming)

	writers := writer.NewWriters(pushgwc)
	record.NewScheduler(alertc, recordingRuleCache, promClients, writers, alertStats, datasourceCache)

	eval.NewScheduler(alertc, externalProcessors, alertRuleCache, targetCache, targetsOfAlertRulesCache,
		busiGroupCache, alertMuteCache, datasourceCache, promClients, naming, ctx, alertStats)

	eventProcessorCache := memsto.NewEventProcessorCache(ctx, syncStats)

	sender.InitStaticGlobalWebhook(alertc.Alerting.GlobalWebhook)

	dp := dispatch.NewDispatch(alertRuleCache, userCache, userGroupCache, alertSubscribeCache, targetCache, notifyConfigCache, taskTplsCache, notifyRuleCache, notifyChannelCache, messageTemplateCache, eventProcessorCache, configCvalCache, alertc.Alerting, ctx, alertStats)
	consumer := dispatch.NewConsumer(alertc.Alerting, ctx, dp, promClients, alertMuteCache)

	notifyRecordConsumer := sender.NewNotifyRecordConsumer(ctx)

	go dp.ReloadTpls()
	go consumer.LoopConsume()
	go notifyRecordConsumer.LoopConsume()

	go queue.ReportQueueSize(alertStats)
	go sender.ReportNotifyRecordQueueSize(alertStats)
	go sender.InitEmailSender(ctx, notifyConfigCache)
}
