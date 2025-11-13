package center

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ccfos/nightingale/v6/dscache"

	"github.com/toolkits/pkg/logger"

	"github.com/ccfos/nightingale/v6/alert"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/dispatch"
	"github.com/ccfos/nightingale/v6/alert/process"
	alertrt "github.com/ccfos/nightingale/v6/alert/router"
	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/center/cconf/rsa"
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
	"github.com/ccfos/nightingale/v6/pkg/macros"
	"github.com/ccfos/nightingale/v6/pkg/version"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	pushgwrt "github.com/ccfos/nightingale/v6/pushgw/router"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
	"github.com/ccfos/nightingale/v6/storage"
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

	if config.Alert.Heartbeat.EngineName == "" {
		config.Alert.Heartbeat.EngineName = "default"
	}

	logxClean, err := logx.Init(config.Log)
	if err != nil {
		return nil, err
	}

	i18nx.Init(configDir)
	flashduty.Init(config.Center.FlashDuty)

	db, err := storage.New(config.DB)
	if err != nil {
		return nil, err
	}
	ctx := ctx.NewContext(context.Background(), db, true)
	migrate.Migrate(db)
	isRootInit := models.InitRoot(ctx)

	config.HTTP.JWTAuth.SigningKey = models.InitJWTSigningKey(ctx)

	err = rsa.InitRSAConfig(ctx, &config.HTTP.RSA)
	if err != nil {
		return nil, err
	}

	go integration.Init(ctx, config.Center.BuiltinIntegrationsDir)
	var redis storage.Redis
	redis, err = storage.NewRedis(config.Redis)
	if err != nil {
		return nil, err
	}

	metas := metas.New(redis)
	idents := idents.New(ctx, redis, config.Pushgw)

	syncStats := memsto.NewSyncStats()
	alertStats := astats.NewSyncStats()

	if config.Center.MigrateBusiGroupLabel || models.CanMigrateBg(ctx) {
		models.MigrateBg(ctx, config.Pushgw.BusiGroupLabelKey)
	}
	if models.CanMigrateEP(ctx) {
		models.MigrateEP(ctx)
	}

	// 初始化 siteUrl，如果为空则设置默认值
	InitSiteUrl(ctx, config.Alert.Heartbeat.IP, config.HTTP.Port)

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
	configCvalCache := memsto.NewCvalCache(ctx, syncStats)
	notifyRuleCache := memsto.NewNotifyRuleCache(ctx, syncStats)
	notifyChannelCache := memsto.NewNotifyChannelCache(ctx, syncStats)
	messageTemplateCache := memsto.NewMessageTemplateCache(ctx, syncStats)
	userTokenCache := memsto.NewUserTokenCache(ctx, syncStats)

	sso := sso.Init(config.Center, ctx, configCache)
	promClients := prom.NewPromClient(ctx)

	dispatch.InitRegisterQueryFunc(promClients)

	externalProcessors := process.NewExternalProcessors()

	macros.RegisterMacro(macros.MacroInVain)
	dscache.Init(ctx, false)
	alert.Start(config.Alert, config.Pushgw, syncStats, alertStats, externalProcessors, targetCache, busiGroupCache, alertMuteCache, alertRuleCache, notifyConfigCache, taskTplCache, dsCache, ctx, promClients, userCache, userGroupCache, notifyRuleCache, notifyChannelCache, messageTemplateCache, configCvalCache)

	writers := writer.NewWriters(config.Pushgw)

	go version.GetGithubVersion()

	go cron.CleanNotifyRecord(ctx, config.Center.CleanNotifyRecordDay)

	alertrtRouter := alertrt.New(config.HTTP, config.Alert, alertMuteCache, targetCache, busiGroupCache, alertStats, ctx, externalProcessors)
	centerRouter := centerrt.New(config.HTTP, config.Center, config.Alert, config.Ibex,
		cconf.Operations, dsCache, notifyConfigCache, promClients,
		redis, sso, ctx, metas, idents, targetCache, userCache, userGroupCache, userTokenCache)
	pushgwRouter := pushgwrt.New(config.HTTP, config.Pushgw, config.Alert, targetCache, busiGroupCache, idents, metas, writers, ctx)

	r := httpx.GinEngine(config.Global.RunMode, config.HTTP, configCvalCache.PrintBodyPaths, configCvalCache.PrintAccessLog)

	centerRouter.Config(r)
	alertrtRouter.Config(r)
	pushgwRouter.Config(r)
	dumper.ConfigRouter(r)

	if config.Ibex.Enable {
		migrate.MigrateIbexTables(db)
		ibex.ServerStart(true, db, redis, config.HTTP.APIForService.BasicAuth, config.Alert.Heartbeat, &config.CenterApi, r, centerRouter, config.Ibex, config.HTTP.Port)
	}

	httpClean := httpx.Init(config.HTTP, r)

	fmt.Printf("please view n9e at  http://%v:%v\n", config.Alert.Heartbeat.IP, config.HTTP.Port)
	if isRootInit {
		fmt.Println("username/password: root/root.2020")
	}

	return func() {
		logxClean()
		httpClean()
	}, nil
}

// initSiteUrl 初始化 site_info 中的 site_url，如果为空则使用服务器IP和端口设置默认值
func InitSiteUrl(ctx *ctx.Context, serverIP string, serverPort int) {
	// 构造默认的 SiteUrl
	defaultSiteUrl := fmt.Sprintf("http://%s:%d", serverIP, serverPort)

	// 获取现有的 site_info 配置
	siteInfoStr, err := models.ConfigsGet(ctx, "site_info")
	if err != nil {
		logger.Errorf("failed to get site_info config: %v", err)
		return
	}

	// 如果 site_info 不存在，创建新的
	if siteInfoStr == "" {
		newSiteInfo := memsto.SiteInfo{
			SiteUrl: defaultSiteUrl,
		}
		siteInfoBytes, err := json.Marshal(newSiteInfo)
		if err != nil {
			logger.Errorf("failed to marshal site_info: %v", err)
			return
		}

		err = models.ConfigsSet(ctx, "site_info", string(siteInfoBytes))
		if err != nil {
			logger.Errorf("failed to set site_info: %v", err)
			return
		}

		logger.Infof("initialized site_url with default value: %s", defaultSiteUrl)
		return
	}

	// 检查现有的 site_info 中的 site_url 字段
	var existingSiteInfo memsto.SiteInfo
	err = json.Unmarshal([]byte(siteInfoStr), &existingSiteInfo)
	if err != nil {
		logger.Errorf("failed to unmarshal site_info: %v", err)
		return
	}

	// 如果 site_url 已经有值，则不需要初始化
	if existingSiteInfo.SiteUrl != "" {
		return
	}

	// 设置 site_url
	existingSiteInfo.SiteUrl = defaultSiteUrl

	siteInfoBytes, err := json.Marshal(existingSiteInfo)
	if err != nil {
		logger.Errorf("failed to marshal updated site_info: %v", err)
		return
	}

	err = models.ConfigsSet(ctx, "site_info", string(siteInfoBytes))
	if err != nil {
		logger.Errorf("failed to update site_info: %v", err)
		return
	}

	logger.Infof("initialized site_url with default value: %s", defaultSiteUrl)
}
