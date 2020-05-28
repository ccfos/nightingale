package routes

import (
	"github.com/didi/nightingale/src/modules/monapi/http/middleware"
	"github.com/gin-gonic/gin"
)

// Config routes
func Config(r *gin.Engine) {
	r.Static("/pub", "./pub")
	r.StaticFile("/favicon.ico", "./pub/favicon.ico")

	hbs := r.Group("/api/hbs")
	{
		hbs.POST("/heartbeat", heartBeat)
		hbs.GET("/instances", instanceGets)
	}

	nolog := r.Group("/api/portal")
	{
		nolog.GET("/ping", ping)
		nolog.GET("/version", version)
		nolog.GET("/pid", pid)
		nolog.GET("/addr", addr)

		nolog.POST("/auth/login", login)
		nolog.GET("/auth/logout", logout)

		nolog.GET("/users/invite", userInviteGet)
		nolog.POST("/users/invite", userInvitePost)

		nolog.GET("/collects/:endpoint", collectGetByEndpoint)

		nolog.GET("/stras/effective", effectiveStrasGet)
		nolog.GET("/stras", strasAll)
	}

	login := r.Group("/api/portal").Use(middleware.Logined())
	{
		login.GET("/self/profile", selfProfileGet)
		login.PUT("/self/profile", selfProfilePut)
		login.PUT("/self/password", selfPasswordPut)

		login.GET("/user", userListGet)
		login.POST("/user", userAddPost)
		login.GET("/user/:id/profile", userProfileGet)
		login.PUT("/user/:id/profile", userProfilePut)
		login.PUT("/user/:id/password", userPasswordPut)
		login.DELETE("/user/:id", userDel)

		login.GET("/team", teamListGet)
		login.POST("/team", teamAddPost)
		login.PUT("/team/:id", teamPut)
		login.DELETE("/team/:id", teamDel)

		login.GET("/endpoint", endpointGets)
		login.POST("/endpoint", endpointImport)
		login.PUT("/endpoint/:id", endpointPut)
		login.DELETE("/endpoint", endpointDel)

		login.GET("/endpoints/bindings", endpointBindingsGet)
		login.GET("/endpoints/bynodeids", endpointByNodeIdsGets)

		login.GET("/tree", treeGet)
		login.GET("/tree/search", treeSearchGet)

		login.POST("/node", nodePost)
		login.PUT("/node/:id/name", nodeNamePut)
		login.DELETE("/node/:id", nodeDel)
		login.GET("/node/:id/endpoint", endpointsUnder)
		login.POST("/node/:id/endpoint-bind", endpointBind)
		login.POST("/node/:id/endpoint-unbind", endpointUnbind)
		login.GET("/node/:id/maskconf", maskconfGets)
		login.GET("/node/:id/screen", screenGets)
		login.POST("/node/:id/screen", screenPost)

		login.GET("/nodes/search", nodeSearchGet)
		login.GET("/nodes/leafids", nodeLeafIdsGet)
		login.GET("/nodes/pids", nodePidsGet)
		login.GET("/nodes/byids", nodesByIdsGets)

		login.POST("/maskconf", maskconfPost)
		login.PUT("/maskconf/:id", maskconfPut)
		login.DELETE("/maskconf/:id", maskconfDel)

		login.PUT("/screen/:id", screenPut)
		login.DELETE("/screen/:id", screenDel)
		login.GET("/screen/:id/subclass", screenSubclassGets)
		login.POST("/screen/:id/subclass", screenSubclassPost)

		login.PUT("/subclass", screenSubclassPut)
		login.DELETE("/subclass/:id", screenSubclassDel)
		login.GET("/subclass/:id/chart", chartGets)
		login.POST("/subclass/:id/chart", chartPost)

		login.PUT("/subclasses/loc", screenSubclassLocPut)

		login.PUT("/chart/:id", chartPut)
		login.DELETE("/chart/:id", chartDel)

		login.PUT("/charts/weights", chartWeightsPut)

		login.GET("/tmpchart", tmpChartGet)
		login.POST("/tmpchart", tmpChartPost)

		login.GET("/event/cur", eventCurGets)
		login.GET("/event/cur/:id", eventCurGetById)
		login.DELETE("/event/cur/:id", eventCurDel)
		login.GET("/event/his", eventHisGets)
		login.GET("/event/his/:id", eventHisGetById)
		login.POST("/event/curs/claim", eventCurClaim)

		login.POST("/collect", collectPost)
		login.GET("/collect/list", collectsGet)
		login.GET("/collect", collectGet)
		login.PUT("/collect", collectPut)
		login.DELETE("/collect", collectsDel)
		login.POST("/collect/check", regExpCheck)

		login.POST("/stra", straPost)
		login.PUT("/stra", straPut)
		login.DELETE("/stra", strasDel)
		login.GET("/stra", strasGet)
		login.GET("/stra/:sid", straGet)
	}

	v1 := r.Group("/v1/portal").Use(middleware.CheckHeaderToken())
	{
		v1.POST("/endpoint", endpointImport)
		v1.GET("/tree", treeGet)
		v1.GET("/endpoints/bynodeids", endpointByNodeIdsGets)
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
}
