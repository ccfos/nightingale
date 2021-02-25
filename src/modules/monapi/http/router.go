package http

import (
	"github.com/didi/nightingale/src/modules/monapi/config"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

// Config routes
func Config(r *gin.Engine) {
	r.Static("/pub", "./pub")
	r.StaticFile("/favicon.ico", "./pub/favicon.ico")

	sys := r.Group("/api/mon/sys")
	{
		sys.GET("/ping", ping)
		sys.GET("/version", version)
		sys.GET("/pid", pid)
		sys.GET("/addr", addr)
	}

	generic := r.Group("/api/mon").Use(GetCookieUser())
	{
		generic.GET("/regions", func(c *gin.Context) { renderData(c, config.Get().Region, nil) })
	}

	node := r.Group("/api/mon/node").Use(GetCookieUser())
	{
		node.GET("/:id/maskconf", maskconfGets)
		node.GET("/:id/screen", screenGets)
		node.POST("/:id/screen", screenPost)
	}

	maskconf := r.Group("/api/mon/maskconf").Use(GetCookieUser())
	{
		maskconf.POST("", maskconfPost)
		maskconf.PUT("/:id", maskconfPut)
		maskconf.DELETE("/:id", maskconfDel)
	}

	screen := r.Group("/api/mon/screen").Use(GetCookieUser())
	{
		screen.GET("/:id", screenGet)
		screen.PUT("/:id", screenPut)
		screen.DELETE("/:id", screenDel)
		screen.GET("/:id/subclass", screenSubclassGets)
		screen.POST("/:id/subclass", screenSubclassPost)
	}

	subclass := r.Group("/api/mon/subclass").Use(GetCookieUser())
	{
		subclass.PUT("", screenSubclassPut)
		subclass.DELETE("/:id", screenSubclassDel)
		subclass.GET("/:id/chart", chartGets)
		subclass.POST("/:id/chart", chartPost)
	}

	subclasses := r.Group("/api/mon/subclasses").Use(GetCookieUser())
	{
		subclasses.PUT("/loc", screenSubclassLocPut)
	}

	chart := r.Group("/api/mon/chart").Use(GetCookieUser())
	{
		chart.PUT("/:id", chartPut)
		chart.DELETE("/:id", chartDel)
	}

	charts := r.Group("/api/mon/charts").Use(GetCookieUser())
	{
		charts.PUT("/weights", chartWeightsPut)
	}

	tmpchart := r.Group("/api/mon/tmpchart").Use(GetCookieUser())
	{
		tmpchart.GET("", tmpChartGet)
		tmpchart.POST("", tmpChartPost)
	}

	event := r.Group("/api/mon/event").Use(GetCookieUser())
	{
		event.GET("/cur", eventCurGets)
		event.GET("/cur/:id", eventCurGetById)
		event.DELETE("/cur/:id", eventCurDel)
		event.GET("/his", eventHisGets)
		event.GET("/his/:id", eventHisGetById)
		event.POST("/cur/claim", eventCurClaim)
	}

	// TODO: merge to collect-rule
	collect := r.Group("/api/mon/collect").Use(GetCookieUser())
	{
		collect.POST("", collectRulePost)     // create a collect rule
		collect.GET("/list", collectRulesGet) // get collect rules
		collect.GET("", collectRuleGet)       // get collect rule by type & id
		collect.PUT("", collectRulePut)       // update collect rule by type & id
		collect.DELETE("", collectsRuleDel)   // delete collect rules by type & ids
		collect.POST("/check", regExpCheck)   // check collect rule
	}

	// TODO: merge to collect-rules, used by agent
	collects := r.Group("/api/mon/collects")
	{
		collects.GET("/:endpoint", collectRulesGetByLocalEndpoint) // get collect rules by endpoint, for agent
		collects.GET("", collectRulesGet)                          // get collect rules
	}

	collectRules := r.Group("/api/mon/collect-rules").Use(GetCookieUser())
	{
		collectRules.POST("", collectRulePost)                            // create a collect rule
		collectRules.GET("/list", collectRulesGetV2)                      // get collect rules
		collectRules.GET("", collectRuleGet)                              // get collect rule by type & id
		collectRules.PUT("", collectRulePut)                              // update collect rule by type & id
		collectRules.DELETE("", collectsRuleDel)                          // delete collect rules by type & ids
		collectRules.POST("/check", regExpCheck)                          // check collect rule
		collectRules.GET("/types", collectRuleTypesGet)                   // get collect types, category: local|remote
		collectRules.GET("/types/:type/template", collectRuleTemplateGet) // get collect teplate by type

	}

	collectRulesAnonymous := r.Group("/api/mon/collect-rules")
	{
		collectRulesAnonymous.GET("/endpoints/:endpoint/local", collectRulesGetByLocalEndpoint) // for agent
	}

	stra := r.Group("/api/mon/stra").Use(GetCookieUser())
	{
		stra.POST("", straPost)
		stra.PUT("", straPut)
		stra.DELETE("", strasDel)
		stra.GET("", strasGet)
		stra.GET("/:sid", straGet)
	}

	stras := r.Group("/api/mon/stras")
	{
		stras.GET("/effective", effectiveStrasGet)
		stras.GET("", strasAll)
	}

	aggr := r.Group("/api/mon/aggr").Use(GetCookieUser())
	{
		aggr.POST("", aggrCalcPost)
		aggr.PUT("", aggrCalcPut)
		aggr.DELETE("", aggrCalcsDel)
		aggr.GET("", aggrCalcsGet)
		aggr.GET("/:id", aggrCalcGet)
	}

	tpl := r.Group("/api/mon/tpl")
	{
		tpl.GET("", tplNameGets)
		tpl.GET("/content", tplGet)
	}

	aggrs := r.Group("/api/mon/aggrs").Use()
	{
		aggrs.GET("", aggrCalcsWithEndpointGet)
	}

	index := r.Group("/api/mon/index")
	{
		index.POST("/metrics", getMetrics)
		index.POST("/tagkv", getTagkvs)
	}

	transferProxy := r.Group("/api/transfer")
	{
		transferProxy.GET("/req", transferReq)
		transferProxy.POST("/data", transferReq)
		transferProxy.POST("/data/ui", transferReq)
		transferProxy.POST("/push", transferReq)
	}

	indexProxy := r.Group("/api/index")
	{
		indexProxy.POST("/metrics", indexReq)
		indexProxy.POST("/tagkv", indexReq)
		indexProxy.POST("/counter/fullmatch", indexReq)
		indexProxy.POST("/counter/clude", indexReq)
		indexProxy.POST("/counter/detail", indexReq)
	}

	v1 := r.Group("/v1/mon")
	{
		v1.GET("/collect-rules/endpoints/:endpoint/remote", collectRulesGetByRemoteEndpoint) // for prober
	}

	if config.Get().Logger.Level == "DEBUG" {
		pprof.Register(r, "/api/monapi/debug/pprof")
	}
}
