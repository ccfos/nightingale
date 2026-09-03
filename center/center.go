package center

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"

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
	"github.com/ccfos/nightingale/v6/pkg/evallog"
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
	"github.com/ccfos/nightingale/v6/tsdb"
	tsdbrt "github.com/ccfos/nightingale/v6/tsdb/router"
	"github.com/flashcatcloud/ibex/src/cmd/ibex"
)

func Initialize(configDir string, cryptoKey string) (func(), error) {
	config, err := conf.InitCenterConfig(configDir, cryptoKey)
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
	models.InitDefaultAIAgent(ctx)

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

	macros.RegisterMacro(macros.ExpandTimeFilter)
	dscache.Init(ctx, false, config.Alert.Heartbeat.EngineName)
	alert.Start(config.Alert, config.Pushgw, syncStats, alertStats, externalProcessors, targetCache, busiGroupCache, alertMuteCache, alertRuleCache, notifyConfigCache, taskTplCache, dsCache, ctx, promClients, userCache, userGroupCache, notifyRuleCache, notifyChannelCache, messageTemplateCache, configCvalCache)

	writers := writer.NewWriters(config.Pushgw)

	var tsdbInstance *tsdb.Instance
	if config.EmbeddedTSDB.Enable {
		// 内置 tsdb 数据落在本实例的本地磁盘，多副本 center 部署时每个副本只有
		// 一部分数据，且会互相改写下面注册的数据源地址，查询结果静默缺失
		if others, err := models.AlertingEngineGetsInstances(ctx, "engine_cluster = ? and clock > ? and instance <> ?",
			config.Alert.Heartbeat.EngineName, time.Now().Unix()-90, config.Alert.Heartbeat.Endpoint); err == nil && len(others) > 0 {
			logger.Warningf("embedded tsdb: detected other active center instances %v, "+
				"embedded tsdb only fits single-instance deployment, metrics will be fragmented across replicas; "+
				"disable [EmbeddedTSDB] and use an external TSDB, or set EmbeddedTSDB.DatasourceUrl to a stable address", others)
		}

		tsdbInstance, err = tsdb.Open(config.EmbeddedTSDB)
		if err != nil {
			return nil, fmt.Errorf("failed to open embedded tsdb: %v", err)
		}

		// 样本经 pushgw 转发链路走 remote write 写入本实例的
		// /prometheus/api/v1/write（writer 配置在 conf.InitConfig 注入），
		// 这里只需挂查询/写入路由（见下方 tsdbrt）并注册数据源

		tsdbUrl := config.EmbeddedTSDB.DatasourceUrl
		skipTLSVerify := false
		if tsdbUrl == "" {
			// 与 httpx.Init 保持一致：配了证书就只监听 https；本机地址/探测 IP 通常
			// 不在证书 SAN 里，跳过校验，需要严格校验时显式配置 DatasourceUrl
			scheme := "http"
			if config.HTTP.CertFile != "" && config.HTTP.KeyFile != "" {
				scheme = "https"
				skipTLSVerify = true
			}

			// 未配置 BasicAuth 时 /prometheus 端点仅接受本机请求（见
			// tsdb/router 的 requireLocal），数据源相应指向本机地址——前端代理
			// 与告警引擎都从 center 进程发起，本机可达；配置了 BasicAuth 即视
			// 为允许远程访问，改用探测 IP 方便 edge / grafana 等远程消费
			host := config.Alert.Heartbeat.IP
			if config.EmbeddedTSDB.BasicAuthUser == "" {
				host = config.HTTP.Host
				if host == "" || host == "0.0.0.0" || host == "::" {
					host = "127.0.0.1"
				}
			}
			tsdbUrl = fmt.Sprintf("%s://%s/prometheus", scheme, net.JoinHostPort(host, fmt.Sprint(config.HTTP.Port)))
		}
		models.InitEmbeddedTSDBDatasource(ctx, tsdbUrl, config.EmbeddedTSDB.BasicAuthUser, config.EmbeddedTSDB.BasicAuthPass, skipTLSVerify)
	}

	go version.GetGithubVersion()

	go cron.CleanNotifyRecord(ctx, config.Center.CleanNotifyRecordDay)
	go cron.CleanPipelineExecution(ctx, config.Center.CleanPipelineExecutionDay)
	go cron.CleanAlertHisEvent(ctx, config.Center.CleanAlertHisEventDay)

	alertrtRouter := alertrt.New(config.HTTP, config.Alert, alertMuteCache, targetCache, busiGroupCache, alertStats, ctx, externalProcessors, config.Log.Dir)
	centerRouter := centerrt.New(config.HTTP, config.Center, config.Alert, config.Ibex,
		cconf.Operations, dsCache, notifyConfigCache, promClients,
		redis, sso, ctx, metas, idents, targetCache, userCache, userGroupCache, userTokenCache, config.Log.Dir)
	// categrafMeta 反查「机器指标写进了哪个数据源」要读 writer 地址；单独赋值而非
	// 加进 New 的参数表，避免动到嵌入方（企业版）调用的函数签名。
	centerRouter.Pushgw = config.Pushgw
	pushgwRouter := pushgwrt.New(config.HTTP, config.Pushgw, config.Alert, targetCache, busiGroupCache, idents, metas, writers, ctx)

	r := httpx.GinEngine(config.Global.RunMode, config.HTTP, configCvalCache.PrintBodyPaths, configCvalCache.PrintAccessLog)

	centerRouter.Config(r)
	alertrtRouter.Config(r)
	pushgwRouter.Config(r)
	if tsdbInstance != nil {
		tsdbrt.New(tsdbInstance, config.HTTP.Host).Config(r)
	}
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
		// httpClean 优雅关闭：等在途请求处理完才返回，之后不再有 remote write
		// 请求进到本地 tsdb，Close 不会与写入竞态
		httpClean()
		if tsdbInstance != nil {
			if err := tsdbInstance.Close(); err != nil {
				logger.Errorf("failed to close embedded tsdb: %v", err)
			}
		}
		// 同 alert.Initialize：排空 evallog 的写入队列与文件缓冲，且必须早于 logxClean
		evallog.Shutdown()
		logxClean()
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
