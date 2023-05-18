package pushgw

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/logx"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/ccfos/nightingale/v6/pushgw/router"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
)

type PushgwProvider struct {
	Ident  *idents.Set
	Router *router.Router
}

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

	idents := idents.New(ctx)

	stats := memsto.NewSyncStats()

	busiGroupCache := memsto.NewBusiGroupCache(ctx, stats)
	targetCache := memsto.NewTargetCache(ctx, stats, nil)

	writers := writer.NewWriters(config.Pushgw)

	r := httpx.GinEngine(config.Global.RunMode, config.HTTP)
	rt := router.New(config.HTTP, config.Pushgw, targetCache, busiGroupCache, idents, writers, ctx)
	rt.Config(r)

	httpClean := httpx.Init(config.HTTP, r)

	return func() {
		logxClean()
		httpClean()
	}, nil
}
