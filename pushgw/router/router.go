package router

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/center/metas"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/pstat"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
)

type HandleTSFunc func(pt *prompb.TimeSeries) *prompb.TimeSeries

type Router struct {
	HTTP           httpx.Config
	Pushgw         pconf.Pushgw
	Aconf          aconf.Alert
	TargetCache    *memsto.TargetCacheType
	BusiGroupCache *memsto.BusiGroupCacheType
	IdentSet       *idents.Set
	MetaSet        *metas.Set
	Writers        *writer.WritersType
	Ctx            *ctx.Context
	HandleTS       HandleTSFunc
	HeartbeatApi   string

	// 预编译的 DropSample 过滤器
	dropByNameOnly map[string]struct{} // 仅 __name__ 条件的快速匹配
	dropComplex    []map[string]string // 多条件的复杂匹配
}

func stat() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		code := fmt.Sprintf("%d", c.Writer.Status())
		method := c.Request.Method
		labels := []string{"pushgw", code, c.FullPath(), method}

		pstat.RequestDuration.WithLabelValues(labels...).Observe(float64(time.Since(start).Seconds()))
	}
}

func New(httpConfig httpx.Config, pushgw pconf.Pushgw, aconf aconf.Alert, tc *memsto.TargetCacheType, bg *memsto.BusiGroupCacheType,
	idents *idents.Set, metas *metas.Set,
	writers *writer.WritersType, ctx *ctx.Context) *Router {
	rt := &Router{
		HTTP:           httpConfig,
		Pushgw:         pushgw,
		Aconf:          aconf,
		Writers:        writers,
		Ctx:            ctx,
		TargetCache:    tc,
		BusiGroupCache: bg,
		IdentSet:       idents,
		MetaSet:        metas,
		HandleTS:       func(pt *prompb.TimeSeries) *prompb.TimeSeries { return pt },
	}

	// 预编译 DropSample 过滤器
	rt.initDropSampleFilters()

	return rt
}

// initDropSampleFilters 预编译 DropSample 过滤器，将单条件 __name__ 过滤器
// 放入 map 实现 O(1) 查找，多条件过滤器保留原有逻辑
func (rt *Router) initDropSampleFilters() {
	rt.dropByNameOnly = make(map[string]struct{})
	rt.dropComplex = make([]map[string]string, 0)

	for _, filter := range rt.Pushgw.DropSample {
		if len(filter) == 0 {
			continue
		}

		// 如果只有一个条件且是 __name__，放入快速匹配 map
		if len(filter) == 1 {
			if name, ok := filter["__name__"]; ok {
				rt.dropByNameOnly[name] = struct{}{}
				continue
			}
		}

		// 其他情况放入复杂匹配列表
		rt.dropComplex = append(rt.dropComplex, filter)
	}

	logger.Infof("DropSample filters initialized: %d name-only, %d complex",
		len(rt.dropByNameOnly), len(rt.dropComplex))
}

func (rt *Router) Config(r *gin.Engine) {
	basePath := rt.HTTP.NormalizedBasePath()

	service := r.Group(basePath + "/v1/n9e")
	if len(rt.HTTP.APIForService.BasicAuth) > 0 {
		service.Use(gin.BasicAuth(rt.HTTP.APIForService.BasicAuth))
	}
	service.POST("/target-update", rt.targetUpdate)

	if !rt.HTTP.APIForAgent.Enable {
		return
	}

	r.Use(stat())
	// datadog url: http://n9e-pushgw.foo.com/datadog
	// use apiKey not basic auth
	r.POST(basePath+"/datadog/api/v1/series", rt.datadogSeries)
	r.POST(basePath+"/datadog/api/v1/check_run", datadogCheckRun)
	r.GET(basePath+"/datadog/api/v1/validate", datadogValidate)
	r.POST(basePath+"/datadog/api/v1/metadata", datadogMetadata)
	r.POST(basePath+"/datadog/intake/", datadogIntake)

	if len(rt.HTTP.APIForAgent.BasicAuth) > 0 {
		// enable basic auth
		accounts := make(ginx.Accounts, 0)
		for username, password := range rt.HTTP.APIForAgent.BasicAuth {
			accounts = append(accounts, ginx.Account{
				User:     username,
				Password: password,
			})
		}

		for username, password := range rt.HTTP.APIForService.BasicAuth {
			accounts = append(accounts, ginx.Account{
				User:     username,
				Password: password,
			})
		}

		auth := ginx.BasicAuth(accounts)
		r.POST(basePath+"/opentsdb/put", auth, rt.openTSDBPut)
		r.POST(basePath+"/openfalcon/push", auth, rt.falconPush)
		r.POST(basePath+"/prometheus/v1/write", auth, rt.remoteWrite)
		r.POST(basePath+"/proxy/v1/write", auth, rt.proxyRemoteWrite)
		r.POST(basePath+"/v1/n9e/edge/heartbeat", auth, rt.heartbeat)

		if len(rt.Ctx.CenterApi.Addrs) > 0 {
			r.POST(basePath+"/v1/n9e/heartbeat", auth, rt.heartbeat)
		}
	} else {
		// no need basic auth
		r.POST(basePath+"/opentsdb/put", rt.openTSDBPut)
		r.POST(basePath+"/openfalcon/push", rt.falconPush)
		r.POST(basePath+"/prometheus/v1/write", rt.remoteWrite)
		r.POST(basePath+"/proxy/v1/write", rt.proxyRemoteWrite)
		r.POST(basePath+"/v1/n9e/edge/heartbeat", rt.heartbeat)

		if len(rt.Ctx.CenterApi.Addrs) > 0 {
			r.POST(basePath+"/v1/n9e/heartbeat", rt.heartbeat)
		}
	}
}
