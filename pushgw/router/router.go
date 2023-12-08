package router

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/prometheus/prompb"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
)

type HandleTSFunc func(pt *prompb.TimeSeries)

type Router struct {
	HTTP           httpx.Config
	Pushgw         pconf.Pushgw
	TargetCache    *memsto.TargetCacheType
	BusiGroupCache *memsto.BusiGroupCacheType
	IdentSet       *idents.Set
	Writers        *writer.WritersType
	Ctx            *ctx.Context
	HandleTS       HandleTSFunc
}

func New(httpConfig httpx.Config, pushgw pconf.Pushgw, tc *memsto.TargetCacheType, bg *memsto.BusiGroupCacheType, idents *idents.Set, writers *writer.WritersType, ctx *ctx.Context) *Router {
	return &Router{
		HTTP:           httpConfig,
		Pushgw:         pushgw,
		Writers:        writers,
		Ctx:            ctx,
		TargetCache:    tc,
		BusiGroupCache: bg,
		IdentSet:       idents,
		HandleTS:       func(pt *prompb.TimeSeries) {},
	}
}

func (rt *Router) Config(r *gin.Engine) {
	if !rt.HTTP.APIForAgent.Enable {
		return
	}

	registerMetrics()
	go rt.ReportIdentStats()

	// datadog url: http://n9e-pushgw.foo.com/datadog
	// use apiKey not basic auth
	r.POST("/datadog/api/v1/series", rt.datadogSeries)
	r.POST("/datadog/api/v1/check_run", datadogCheckRun)
	r.GET("/datadog/api/v1/validate", datadogValidate)
	r.POST("/datadog/api/v1/metadata", datadogMetadata)
	r.POST("/datadog/intake/", datadogIntake)

	if len(rt.HTTP.APIForAgent.BasicAuth) > 0 {
		// enable basic auth
		auth := gin.BasicAuth(rt.HTTP.APIForAgent.BasicAuth)
		r.POST("/opentsdb/put", auth, rt.openTSDBPut)
		r.POST("/openfalcon/push", auth, rt.falconPush)
		r.POST("/prometheus/v1/write", auth, rt.remoteWrite)
		r.POST("/v1/n9e/target-update", auth, rt.targetUpdate)
		r.POST("/v1/n9e/edge/heartbeat", auth, rt.heartbeat)

		if len(rt.Ctx.CenterApi.Addrs) > 0 {
			r.POST("/v1/n9e/heartbeat", auth, rt.heartbeat)
		}
	} else {
		// no need basic auth
		r.POST("/opentsdb/put", rt.openTSDBPut)
		r.POST("/openfalcon/push", rt.falconPush)
		r.POST("/prometheus/v1/write", rt.remoteWrite)
		r.POST("/v1/n9e/target-update", rt.targetUpdate)
		r.POST("/v1/n9e/edge/heartbeat", rt.heartbeat)

		if len(rt.Ctx.CenterApi.Addrs) > 0 {
			r.POST("/v1/n9e/heartbeat", rt.heartbeat)
		}
	}
}
