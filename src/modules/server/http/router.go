package http

// Config routes
import (
	"github.com/didi/nightingale/v4/src/modules/server/config"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
)

func Config(r *gin.Engine) {
	r.Static("/pub", "./pub")
	r.Static("/static", "./pub/static")
	r.Static("/layout", "./pub/layout")
	r.Static("/ams/", "./pub/ams")
	r.Static("/rdb/", "./pub/rdb")
	r.Static("/job/", "./pub/job")
	r.Static("/mon/", "./pub/mon")
	r.StaticFile("/ams", "./pub/layout/index.html")
	r.StaticFile("/mon", "./pub/layout/index.html")
	r.StaticFile("/job", "./pub/layout/index.html")
	r.StaticFile("/rdb", "./pub/layout/index.html")
	r.StaticFile("/", "./pub/layout/index.html")
	r.StaticFile("/favicon.ico", "./pub/favicon.ico")

	pprof.Register(r, "/api/server/debug/pprof")

	sys := r.Group("/api/server")
	{
		sys.GET("/ping", ping)
		sys.GET("/pid", pid)
		sys.GET("/addr", addr)
	}

	hbs := r.Group("/api/hbs")
	{
		hbs.POST("/heartbeat", heartBeat)
		hbs.GET("/instances", instanceGets)
	}

	jobNotLogin := r.Group("/api/job-ce")
	{
		jobNotLogin.GET("/ping", ping)
		jobNotLogin.POST("/callback", taskCallback)
		jobNotLogin.GET("/task/:id/stdout", taskStdout)
		jobNotLogin.GET("/task/:id/stderr", taskStderr)
		jobNotLogin.GET("/task/:id/state", apiTaskState)
		jobNotLogin.GET("/task/:id/result", apiTaskResult)
		jobNotLogin.GET("/task/:id/host/:host/output", taskHostOutput)
		jobNotLogin.GET("/task/:id/host/:host/stdout", taskHostStdout)
		jobNotLogin.GET("/task/:id/host/:host/stderr", taskHostStderr)
		jobNotLogin.GET("/task/:id/stdout.txt", taskStdoutTxt)
		jobNotLogin.GET("/task/:id/stderr.txt", taskStderrTxt)
		jobNotLogin.GET("/task/:id/stdout.json", apiTaskJSONStdouts)
		jobNotLogin.GET("/task/:id/stderr.json", apiTaskJSONStderrs)
	}

	jobUserLogin := r.Group("/api/job-ce").Use(shouldBeLogin())
	{
		jobUserLogin.GET("/task-tpls", taskTplGets)
		jobUserLogin.POST("/task-tpls", taskTplPost)
		jobUserLogin.GET("/task-tpl/:id", taskTplGet)
		jobUserLogin.PUT("/task-tpl/:id", taskTplPut)
		jobUserLogin.DELETE("/task-tpl/:id", taskTplDel)
		jobUserLogin.POST("/task-tpl/:id/run", taskTplRun)
		jobUserLogin.PUT("/task-tpls/tags", taskTplTagsPut)
		jobUserLogin.PUT("/task-tpls/node", taskTplNodePut)

		jobUserLogin.POST("/tasks", taskPost)
		jobUserLogin.GET("/tasks", taskGets)
		jobUserLogin.GET("/task/:id", taskView)
		jobUserLogin.PUT("/task/:id/action", taskActionPut)
		jobUserLogin.PUT("/task/:id/host", taskHostPut)

		// 专门针对工单系统开发的接口
		jobUserLogin.POST("/run/:id", taskRunForTT)
	}

	rdbNotLogin := r.Group("/api/rdb")
	{
		rdbNotLogin.GET("/ping", ping)
		rdbNotLogin.GET("/ldap/used", ldapUsed)
		rdbNotLogin.GET("/ops/global", globalOpsGet)
		rdbNotLogin.GET("/ops/local", localOpsGet)
		rdbNotLogin.GET("/roles/global", globalRoleGet)
		rdbNotLogin.GET("/roles/local", localRoleGet)
		rdbNotLogin.POST("/users/invite", userInvitePost)

		rdbNotLogin.POST("/auth/send-login-code", sendLoginCode)
		rdbNotLogin.POST("/auth/send-rst-code", sendRstCode)
		rdbNotLogin.POST("/auth/rst-password", rstPassword)
		rdbNotLogin.GET("/auth/captcha", captchaGet)

		rdbNotLogin.GET("/v2/nodes", nodeGets)
		rdbNotLogin.GET("/pwd-rules", pwdRulesGet)
		rdbNotLogin.GET("/counter", counterGet)

		rdbNotLogin.PUT("/self/password", selfPasswordPut)

	}

	rdbRootLogin := r.Group("/api/rdb").Use(shouldBeRoot())
	{
		rdbRootLogin.GET("/configs/smtp", smtpConfigsGet)
		rdbRootLogin.POST("/configs/smtp/test", smtpTest)
		rdbRootLogin.PUT("/configs/smtp", smtpConfigsPut)

		rdbRootLogin.GET("/configs/auth", authConfigsGet)
		rdbRootLogin.PUT("/configs/auth", authConfigsPut)
		rdbRootLogin.POST("/auth/white-list", whiteListPost)
		rdbRootLogin.GET("/auth/white-list", whiteListsGet)
		rdbRootLogin.GET("/auth/white-list/:id", whiteListGet)
		rdbRootLogin.PUT("/auth/white-list/:id", whiteListPut)
		rdbRootLogin.DELETE("/auth/white-list/:id", whiteListDel)

		rdbRootLogin.GET("/log/login", loginLogGets)
		rdbRootLogin.GET("/log/operation", operationLogGets)

		rdbRootLogin.POST("/roles", roleAddPost)
		rdbRootLogin.PUT("/role/:id", rolePut)
		rdbRootLogin.DELETE("/role/:id", roleDel)
		rdbRootLogin.GET("/role/:id", roleDetail)
		rdbRootLogin.GET("/role/:id/users", roleGlobalUsersGet)
		rdbRootLogin.PUT("/role/:id/users/bind", roleGlobalUsersBind)
		rdbRootLogin.PUT("/role/:id/users/unbind", roleGlobalUsersUnbind)

		rdbRootLogin.POST("/users", userAddPost)
		rdbRootLogin.GET("/user/:id/profile", userProfileGet)
		rdbRootLogin.PUT("/user/:id/profile", userProfilePut)
		rdbRootLogin.PUT("/user/:id/password", userPasswordPut)
		rdbRootLogin.DELETE("/user/:id", userDel)

		rdbRootLogin.POST("/node-cates", nodeCatePost)
		rdbRootLogin.PUT("/node-cate/:id", nodeCatePut)
		rdbRootLogin.DELETE("/node-cate/:id", nodeCateDel)
		rdbRootLogin.POST("/node-cates/fields", nodeCateFieldNew)
		rdbRootLogin.PUT("/node-cates/field/:id", nodeCateFieldPut)
		rdbRootLogin.DELETE("/node-cates/field/:id", nodeCateFieldDel)

		rdbRootLogin.GET("/nodes/trash", nodeTrashGets)
		rdbRootLogin.PUT("/nodes/trash/recycle", nodeTrashRecycle)
		rdbRootLogin.PATCH("/node/:id/move", nodeMove)

		rdbRootLogin.POST("/sso/clients", ssoClientsPost)
		rdbRootLogin.GET("/sso/clients", ssoClientsGet)
		rdbRootLogin.GET("/sso/clients/:clientId", ssoClientGet)
		rdbRootLogin.PUT("/sso/clients/:clientId", ssoClientPut)
		rdbRootLogin.DELETE("/sso/clients/:clientId", ssoClientDel)

		rdbRootLogin.GET("/resources/tenant-rank", tenantResourcesCountRank)
		rdbRootLogin.GET("/resources/project-rank", projectResourcesCountRank)

		rdbRootLogin.GET("/root/users", userListGet)
		rdbRootLogin.GET("/root/teams/all", teamAllGet)
		rdbRootLogin.GET("/root/node-cates", nodeCateGets)
	}

	rdbUserLogin := r.Group("/api/rdb").Use(shouldBeLogin())
	{
		rdbUserLogin.GET("/resoplogs", operationLogResGets)

		rdbUserLogin.GET("/self/profile", selfProfileGet)
		rdbUserLogin.PUT("/self/profile", selfProfilePut)
		rdbUserLogin.GET("/self/token", selfTokenGets)
		rdbUserLogin.POST("/self/token", selfTokenPost)
		rdbUserLogin.PUT("/self/token", selfTokenPut)
		rdbUserLogin.GET("/self/perms/global", permGlobalOps)
		rdbUserLogin.GET("/self/perms/local/node/:id", permLocalOps)

		rdbUserLogin.GET("/users", userListGet)
		rdbUserLogin.GET("/users/invite", userInviteGet)

		rdbUserLogin.GET("/teams/all", teamAllGet)
		rdbUserLogin.GET("/teams/mine", teamMineGet)
		rdbUserLogin.POST("/teams", teamAddPost)
		rdbUserLogin.PUT("/team/:id", teamPut)
		rdbUserLogin.GET("/team/:id", teamDetail)
		rdbUserLogin.PUT("/team/:id/users/bind", teamUserBind)
		rdbUserLogin.PUT("/team/:id/users/unbind", teamUserUnbind)
		rdbUserLogin.DELETE("/team/:id", teamDel)

		rdbUserLogin.GET("/node-cates", nodeCateGets)
		rdbUserLogin.GET("/node-cates/fields", nodeCateFieldGets)
		rdbUserLogin.GET("/node-cates/field/:id", nodeCateFieldGet)

		rdbUserLogin.POST("/nodes", nodePost)
		rdbUserLogin.GET("/nodes", nodeGets)
		rdbUserLogin.GET("/node/:id", nodeGet)
		rdbUserLogin.PUT("/node/:id", nodePut)
		rdbUserLogin.DELETE("/node/:id", nodeDel)
		rdbUserLogin.GET("/node/:id/fields", nodeFieldGets)
		rdbUserLogin.PUT("/node/:id/fields", nodeFieldPuts)
		rdbUserLogin.GET("/node/:id/roles", rolesUnderNodeGets)
		rdbUserLogin.POST("/node/:id/roles", rolesUnderNodePost)
		rdbUserLogin.DELETE("/node/:id/roles", rolesUnderNodeDel)
		rdbUserLogin.DELETE("/node/:id/roles/try", rolesUnderNodeDelTry)
		rdbUserLogin.GET("/node/:id/resources", resourceUnderNodeGet)
		rdbUserLogin.GET("/node/:id/resources/cate-count", renderNodeResourcesCountByCate)
		rdbUserLogin.POST("/node/:id/resources/bind", resourceBindNode)
		rdbUserLogin.POST("/node/:id/resources/unbind", resourceUnbindNode)
		rdbUserLogin.PUT("/node/:id/resources/note", resourceUnderNodeNotePut)
		rdbUserLogin.PUT("/node/:id/resources/labels", resourceUnderNodeLabelsPut)

		rdbUserLogin.GET("/tree", treeUntilLeafGets)
		rdbUserLogin.GET("/tree/projs", treeUntilProjectGets)
		rdbUserLogin.GET("/tree/orgs", treeUntilOrganizationGets)

		rdbUserLogin.GET("/resources/search", resourceSearchGet)
		rdbUserLogin.PUT("/resources/note", resourceNotePut)
		rdbUserLogin.PUT("/resources/note/try", resourceNotePutTry)
		rdbUserLogin.GET("/resources/bindings", resourceBindingsGet)
		rdbUserLogin.GET("/resources/orphan", resourceOrphanGet)

		rdbUserLogin.GET("/resources/cate-count", renderAllResourcesCountByCate)

		// 是否在某个节点上有权限做某个操作(即资源权限点)
		rdbUserLogin.GET("/can-do-node-op", v1CandoNodeOp)
		// 同时校验多个操作权限点
		rdbUserLogin.GET("/can-do-node-ops", v1CandoNodeOps)
	}

	sessionStarted := r.Group("/api/rdb").Use(shouldStartSession())
	{
		sessionStarted.POST("/auth/login", login)
		sessionStarted.GET("/auth/logout", logout)
		sessionStarted.GET("/auth/v2/authorize", authAuthorizeV2)
		sessionStarted.GET("/auth/v2/callback", authCallbackV2)
		sessionStarted.GET("/auth/v2/logout", logoutV2)
	}

	transfer := r.Group("/api/transfer")
	{
		transfer.POST("/stra", getStra)
		transfer.POST("/which-tsdb", tsdbInstance)
		transfer.POST("/which-judge", judgeInstance)
		transfer.GET("/alive-judges", judges)

		transfer.POST("/push", PushData)
		transfer.POST("/data", QueryData)
		transfer.POST("/data/ui", QueryDataForUI)
	}

	index := r.Group("/api/index")
	{
		index.POST("/metrics", GetMetrics)
		index.POST("/tagkv", GetTagPairs)
		index.POST("/counter/clude", GetIndexByClude)
		index.POST("/counter/fullmatch", GetIndexByFullTags)
	}

	generic := r.Group("/api/mon").Use(shouldBeLogin())
	{
		generic.GET("/regions", func(c *gin.Context) { renderData(c, config.Config.Monapi.Region, nil) })
	}

	node := r.Group("/api/mon/node").Use(shouldBeLogin())
	{
		node.GET("/:id/maskconf", maskconfGets)
		node.GET("/:id/screen", screenGets)
		node.POST("/:id/screen", screenPost)
	}

	maskconf := r.Group("/api/mon/maskconf").Use(shouldBeLogin())
	{
		maskconf.POST("", maskconfPost)
		maskconf.PUT("/:id", maskconfPut)
		maskconf.DELETE("/:id", maskconfDel)
	}

	screen := r.Group("/api/mon/screen").Use(shouldBeLogin())
	{
		screen.GET("/:id", screenGet)
		screen.PUT("/:id", screenPut)
		screen.DELETE("/:id", screenDel)
		screen.GET("/:id/subclass", screenSubclassGets)
		screen.POST("/:id/subclass", screenSubclassPost)
	}

	subclass := r.Group("/api/mon/subclass").Use(shouldBeLogin())
	{
		subclass.PUT("", screenSubclassPut)
		subclass.DELETE("/:id", screenSubclassDel)
		subclass.GET("/:id/chart", chartGets)
		subclass.POST("/:id/chart", chartPost)
	}

	subclasses := r.Group("/api/mon/subclasses").Use(shouldBeLogin())
	{
		subclasses.PUT("/loc", screenSubclassLocPut)
	}

	chart := r.Group("/api/mon/chart").Use(shouldBeLogin())
	{
		chart.PUT("/:id", chartPut)
		chart.DELETE("/:id", chartDel)
	}

	charts := r.Group("/api/mon/charts").Use(shouldBeLogin())
	{
		charts.PUT("/weights", chartWeightsPut)
	}

	tmpchart := r.Group("/api/mon/tmpchart").Use(shouldBeLogin())
	{
		tmpchart.GET("", tmpChartGet)
		tmpchart.POST("", tmpChartPost)
	}

	event := r.Group("/api/mon/event").Use(shouldBeLogin())
	{
		event.GET("/cur", eventCurGets)
		event.GET("/cur/:id", eventCurGetById)
		event.DELETE("/cur/:id", eventCurDel)
		event.GET("/his", eventHisGets)
		event.GET("/his/:id", eventHisGetById)
		event.POST("/cur/claim", eventCurClaim)
	}

	// TODO: merge to collect-rule
	collect := r.Group("/api/mon/collect").Use(shouldBeLogin())
	{
		collect.POST("", collectRulePost)     // create a collect rule
		collect.GET("/list", collectRulesGet) // get collect rules
		collect.GET("", collectRuleGet)       // get collect rule by type & id
		collect.PUT("", collectRulePut)       // update collect rule by type & id
		collect.DELETE("", collectsRuleDel)   // delete collect rules by type & ids
		collect.POST("/check", regExpCheck)   // check collect rule
	}

	collectEE := r.Group("/api/mon-ee/collect").Use(shouldBeLogin())
	{
		collectEE.POST("", collectRulePost)
		collectEE.GET("/list", collectRulesGet)
		collectEE.GET("", collectRuleGet)
		collectEE.PUT("", collectRulePut)
		collectEE.DELETE("", collectsRuleDel)
		collectEE.GET("/region", apiRegionGet)
	}

	apicollects := r.Group("/api/mon-ee/apicollects")
	{
		apicollects.GET("", apiCollectsGet)
	}

	snmpcollects := r.Group("/api/mon-ee/snmp")
	{
		snmpcollects.GET("/collects", snmpCollectsGet)
		snmpcollects.GET("/hw", snmpHWsGet)
		snmpcollects.GET("/mib/module", mibModuleGet)
		snmpcollects.GET("/mib/metric", mibMetricGet)
		snmpcollects.GET("/mib", mibGet)
		snmpcollects.GET("/mibs", mibGets)
	}

	// TODO: merge to collect-rules, used by agent
	collects := r.Group("/api/mon/collects")
	{
		collects.GET("/:endpoint", collectRulesGetByLocalEndpoint) // get collect rules by endpoint, for agent
		collects.GET("", collectRulesGet)                          // get collect rules
	}

	collectRules := r.Group("/api/mon/collect-rules").Use(shouldBeLogin())
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

	stra := r.Group("/api/mon/stra").Use(shouldBeLogin())
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

	aggr := r.Group("/api/mon/aggr").Use(shouldBeLogin())
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

	monIndex := r.Group("/api/mon/index")
	{
		monIndex.POST("/metrics", getMetrics)
		monIndex.POST("/tagkv", getTagkvs)
	}

	judge := r.Group("/api/judge")
	{
		judge.GET("/stra/:id", getStraInJudge)
		judge.POST("/data", getData)
	}

	monV1 := r.Group("/v1/mon")
	{
		monV1.GET("/collect-rules/endpoints/:endpoint/remote", collectRulesGetByRemoteEndpoint) // for prober
	}

	nemsLogin := r.Group("/api/ams-ee").Use(shouldBeLogin())
	{
		nemsLogin.POST("/nethws", networkHardwarePost)
		nemsLogin.GET("/nethws", networkHardwareGets)
		nemsLogin.DELETE("/nethws", networkHardwareDel)
		nemsLogin.PUT("/nethw/obj/:id", networkHardwarePut)
		nemsLogin.PUT("/nethw/note", mgrHWNotePut)
		nemsLogin.PUT("/nethw/cate", mgrHWCatePut)
		nemsLogin.PUT("/nethw/tenant", mgrHWTenantPut)
		nemsLogin.GET("/nethw/cate", hwCateGet)
		nemsLogin.GET("/nethw/region", snmpRegionGet)
		nemsLogin.GET("/nethws/search", nwSearchGets)

		//md-20201223
		nemsLogin.GET("/nws", nwGets)
		nemsLogin.PUT("/nws/back", nwBackPut)
		nemsLogin.GET("/nws/search", nwSearchGets)
		nemsLogin.DELETE("/nws", nwDel)

		nemsLogin.POST("/mibs", mibPost)
		nemsLogin.GET("/mibs", mibGetsByQuery)
		nemsLogin.DELETE("/mibs", mibDel)
		nemsLogin.GET("/mib", mibGet)
	}

	nemsV1 := r.Group("/v1/ams-ee").Use(shouldBeService())
	{
		nemsV1.POST("/get-hw-by-ip", networkHardwareByIP)
		nemsV1.GET("/get-hw", networkHardwareGetAll)
		nemsV1.PUT("/nethws", networkHardwaresPut)
		nemsV1.GET("/mib", mibGet)
		nemsV1.GET("/mibs", mibGets)
		nemsV1.GET("/mib/module", mibModuleGet)
		nemsV1.GET("/mib/metric", mibMetricGet)
	}

	v1 := r.Group("/v1/rdb").Use(shouldBeService())
	{
		// 获取这个节点下的所有资源，跟给前端的API(/api/rdb/node/:id/resources会根据当前登陆用户获取有权限看到的资源列表)不同
		v1.GET("/node/:id/resources", v1ResourcesUnderNodeGet)
		// RDB作为一个类似CMDB的东西，接收各个子系统注册过来的资源，其他资源都是依托于项目创建的，RDB会根据nid自动挂载资源到相应节点
		v1.POST("/resources/register", v1ResourcesRegisterPost)
		// 资源销毁的时候，需要从库里清掉，同时需要把节点挂载关系也删除，一个资源可能挂载在多个节点，都要统统干掉
		v1.POST("/resources/unregister", v1ResourcesUnregisterPost)

		v1.POST("/containers/bind", v1ContainersBindPost)
		v1.POST("/container/sync", v1ContainerSyncPost)

		// 发送邮件、短信、语音、即时通讯消息，这些都依赖客户那边的通道
		v1.POST("/sender/mail", v1SendMail)
		v1.POST("/sender/sms", v1SendSms)
		v1.POST("/sender/voice", v1SendVoice)
		v1.POST("/sender/im", v1SendIm)

		v1.GET("/nodes", nodeGets)
		v1.GET("/node-include-trash/:id", nodeIncludeTrashGet)
		v1.GET("/node/:id", nodeGet)
		v1.GET("/node/:id/projs", v1treeUntilProjectGetsByNid)
		v1.GET("/tree/projs", v1TreeUntilProjectGets)
		v1.GET("/tree", v1TreeUntilTypGets)

		// 外部系统推送一些操作日志过来，RDB统一存储，实际用MQ会更好一些
		v1.POST("/resoplogs", v1OperationLogResPost)

		// 是否有权限做一些全局操作(即页面权限点)
		v1.GET("/can-do-global-op", v1CandoGlobalOp)
		// 是否在某个节点上有权限做某个操作(即资源权限点)
		v1.GET("/can-do-node-op", v1CandoNodeOp)
		// 同时校验多个操作权限点
		v1.GET("/can-do-node-ops", v1CandoNodeOps)

		// 获取用户、团队相关信息
		v1.GET("/get-user-by-uuid", v1UserGetByUUID)
		v1.GET("/get-users-by-uuids", v1UserGetByUUIDs)
		v1.GET("/get-users-by-ids", v1UserGetByIds)
		v1.GET("/get-users-by-names", v1UserGetByNames)
		v1.GET("/get-user-by-token", v1UserGetByToken)
		v1.GET("/get-users-by-query", userListGet)
		v1.GET("/get-teams-by-ids", v1TeamGetByIds)
		v1.GET("/get-user-ids-by-team-ids", v1UserIdsGetByTeamIds)

		v1.GET("/users", v1UserListGet)

		v1.POST("/login", v1Login)
		v1.POST("/send-login-code", sendLoginCode)

		// 第三方系统获取某个用户的所有权限点
		v1.GET("/perms/global", v1PermGlobalOps)

		// session
		v1.GET("/sessions/:sid", v1SessionGet)
		v1.GET("/sessions/:sid/user", v1SessionGetUser)
		v1.GET("/sessions", v1SessionListGet)
		v1.DELETE("/sessions/:sid", v1SessionDelete)

		// token
		v1.GET("/tokens/:token", v1TokenGet)
		v1.GET("/tokens/:token/user", v1TokenGetUser)
		v1.DELETE("/tokens/:token", v1TokenDelete)

		// 第三方系统同步权限表的数据
		v1.GET("/table/sync/role-operation", v1RoleOperationGets)
		v1.GET("/table/sync/role-global-user", v1RoleGlobalUserGets)
	}

	amsUserLogin := r.Group("/api/ams-ce").Use(shouldBeLogin())
	{
		amsUserLogin.GET("/hosts", hostGets)
		amsUserLogin.POST("/hosts", hostPost)
		amsUserLogin.GET("/host/:id", hostGet)
		amsUserLogin.PUT("/hosts/tenant", hostTenantPut)
		amsUserLogin.PUT("/hosts/node", hostNodePut)
		amsUserLogin.PUT("/hosts/back", hostBackPut)
		amsUserLogin.PUT("/hosts/note", hostNotePut)
		amsUserLogin.PUT("/hosts/cate", hostCatePut)
		amsUserLogin.DELETE("/hosts", hostDel)
		amsUserLogin.GET("/hosts/search", hostSearchGets)
		amsUserLogin.POST("/hosts/fields", hostFieldNew)
		amsUserLogin.GET("/hosts/fields", hostFieldsGets)
		amsUserLogin.GET("/hosts/field/:id", hostFieldGet)
		amsUserLogin.PUT("/hosts/field/:id", hostFieldPut)
		amsUserLogin.DELETE("/hosts/field/:id", hostFieldDel)
		amsUserLogin.GET("/host/:id/fields", hostFieldGets)
		amsUserLogin.PUT("/host/:id/fields", hostFieldPuts)
	}

	amsV1 := r.Group("/v1/ams-ce").Use(shouldBeService())
	{
		amsV1.POST("/hosts/register", v1HostRegister)
	}

}
