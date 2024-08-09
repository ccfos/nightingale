package center

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/alert"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/process"
	alertrt "github.com/ccfos/nightingale/v6/alert/router"
	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/center/cconf/rsa"
	"github.com/ccfos/nightingale/v6/center/cstats"
	"github.com/ccfos/nightingale/v6/center/integration"
	"github.com/ccfos/nightingale/v6/center/metas"
	centerrt "github.com/ccfos/nightingale/v6/center/router"
	"github.com/ccfos/nightingale/v6/center/sso"
	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/cron"
	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/models/migrate"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/flashduty"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/i18nx"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/pkg/version"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	pushgwrt "github.com/ccfos/nightingale/v6/pushgw/router"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/ccfos/nightingale/v6/tdengine"

	"github.com/flashcatcloud/ibex/src/cmd/ibex"
)

func Initialize(configDir string, cryptoKey string) (func(), error) {
	config, err := conf.InitConfig(configDir, cryptoKey)
	if err != nil {
		return nil, fmt.Errorf("failed to init config: %v", err)
	}

	cconf.LoadMetricsYaml(configDir, config.Center.MetricsYamlFile)
	cconf.LoadOpsYaml(configDir, config.Center.OpsYamlFile)

	cconf.MergeOperationConf()

	logxClean, err := logx.Init(config.Log)
	if err != nil {
		return nil, err
	}

	i18nx.Init(configDir)
	cstats.Init()
	flashduty.Init(config.Center.FlashDuty)

	db, err := storage.New(config.DB)
	if err != nil {
		return nil, err
	}
	ctx := ctx.NewContext(context.Background(), db, true)
	migrate.Migrate(db)
	models.InitRoot(ctx)

	config.HTTP.JWTAuth.SigningKey = models.InitJWTSigningKey(ctx)

	err = rsa.InitRSAConfig(ctx, &config.HTTP.RSA)
	if err != nil {
		return nil, err
	}

	integration.Init(ctx, config.Center.BuiltinIntegrationsDir)
	var redis storage.Redis
	redis, err = storage.NewRedis(config.Redis)
	if err != nil {
		return nil, err
	}

	metas := metas.New(redis)
	idents := idents.New(ctx, redis)

	syncStats := memsto.NewSyncStats()
	alertStats := astats.NewSyncStats()

	configCache := memsto.NewConfigCache(ctx, syncStats, config.HTTP.RSA.RSAPrivateKey, config.HTTP.RSA.RSAPassWord)
	busiGroupCache := memsto.NewBusiGroupCache(ctx, syncStats)
	targetCache := memsto.NewTargetCache(ctx, syncStats, redis)
	dsCache := memsto.NewDatasourceCache(ctx, syncStats)
	alertMuteCache := memsto.NewAlertMuteCache(ctx, syncStats)
	alertRuleCache := memsto.NewAlertRuleCache(ctx, syncStats)
	notifyConfigCache := memsto.NewNotifyConfigCache(ctx, configCache)
	userCache := memsto.NewUserCache(ctx, syncStats)
	userGroupCache := memsto.NewUserGroupCache(ctx, syncStats)
	taskTplCache := memsto.NewTaskTplCache(ctx)

	sso := sso.Init(config.Center, ctx, configCache)
	promClients := prom.NewPromClient(ctx)
	tdengineClients := tdengine.NewTdengineClient(ctx, config.Alert.Heartbeat)

	externalProcessors := process.NewExternalProcessors()
	alert.Start(config.Alert, config.Pushgw, syncStats, alertStats, externalProcessors, targetCache, busiGroupCache, alertMuteCache, alertRuleCache, notifyConfigCache, taskTplCache, dsCache, ctx, promClients, tdengineClients, userCache, userGroupCache)

	writers := writer.NewWriters(config.Pushgw)

	go version.GetGithubVersion()

	go cron.CleanNotifyRecord(ctx, config.Center.CleanNotifyRecordDay)

	alertrtRouter := alertrt.New(config.HTTP, config.Alert, alertMuteCache, targetCache, busiGroupCache, alertStats, ctx, externalProcessors)
	centerRouter := centerrt.New(config.HTTP, config.Center, config.Alert, config.Ibex, cconf.Operations, dsCache, notifyConfigCache, promClients, tdengineClients,
		redis, sso, ctx, metas, idents, targetCache, userCache, userGroupCache)
	pushgwRouter := pushgwrt.New(config.HTTP, config.Pushgw, config.Alert, targetCache, busiGroupCache, idents, metas, writers, ctx)

	r := httpx.GinEngine(config.Global.RunMode, config.HTTP)

	centerRouter.Config(r)
	alertrtRouter.Config(r)
	pushgwRouter.Config(r)
	dumper.ConfigRouter(r)

	if config.Ibex.Enable {
		migrate.MigrateIbexTables(db)
		ibex.ServerStart(true, db, redis, config.HTTP.APIForService.BasicAuth, config.Alert.Heartbeat, &config.CenterApi, r, centerRouter, config.Ibex, config.HTTP.Port)
	}

	httpClean := httpx.Init(config.HTTP, r)

	return func() {
		logxClean()
		httpClean()
	}, nil
}
