package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/ccfos/nightingale/v6/alert"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/process"
	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
	"github.com/ccfos/nightingale/v6/tdengine"

	alertrt "github.com/ccfos/nightingale/v6/alert/router"
	pushgwrt "github.com/ccfos/nightingale/v6/pushgw/router"
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
	//check CenterApi is default value
	if len(config.CenterApi.Addrs) < 1 {
		return nil, errors.New("failed to init config: the CenterApi configuration is missing")
	}
	ctx := ctx.NewContext(context.Background(), nil, false, config.CenterApi)

	syncStats := memsto.NewSyncStats()

	targetCache := memsto.NewTargetCache(ctx, syncStats, nil)
	busiGroupCache := memsto.NewBusiGroupCache(ctx, syncStats)
	idents := idents.New(ctx)
	writers := writer.NewWriters(config.Pushgw)
	pushgwRouter := pushgwrt.New(config.HTTP, config.Pushgw, targetCache, busiGroupCache, idents, writers, ctx)
	r := httpx.GinEngine(config.Global.RunMode, config.HTTP)
	pushgwRouter.Config(r)

	if !config.Alert.Disable {
		configCache := memsto.NewConfigCache(ctx, syncStats, nil, "")
		alertStats := astats.NewSyncStats()
		dsCache := memsto.NewDatasourceCache(ctx, syncStats)
		alertMuteCache := memsto.NewAlertMuteCache(ctx, syncStats)
		alertRuleCache := memsto.NewAlertRuleCache(ctx, syncStats)
		notifyConfigCache := memsto.NewNotifyConfigCache(ctx, configCache)
		userCache := memsto.NewUserCache(ctx, syncStats)
		userGroupCache := memsto.NewUserGroupCache(ctx, syncStats)

		promClients := prom.NewPromClient(ctx)
		tdengineClients := tdengine.NewTdengineClient(ctx, config.Alert.Heartbeat)
		externalProcessors := process.NewExternalProcessors()

		alert.Start(config.Alert, config.Pushgw, syncStats, alertStats, externalProcessors, targetCache, busiGroupCache, alertMuteCache,
			alertRuleCache, notifyConfigCache, dsCache, ctx, promClients, tdengineClients, userCache, userGroupCache)

		alertrtRouter := alertrt.New(config.HTTP, config.Alert, alertMuteCache, targetCache, busiGroupCache, alertStats, ctx, externalProcessors)

		alertrtRouter.Config(r)
	}

	dumper.ConfigRouter(r)
	httpClean := httpx.Init(config.HTTP, r)

	return func() {
		logxClean()
		httpClean()
	}, nil
}
