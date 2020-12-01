package http

import (
	"github.com/gin-gonic/gin"
)

func Config(r *gin.Engine) {

	notLogin := r.Group("/api/rdb")
	{
		notLogin.GET("/ping", ping)
		notLogin.GET("/ldap/used", ldapUsed)
		notLogin.POST("/auth/login", login)
		notLogin.GET("/auth/logout", logout)
		notLogin.GET("/ops/global", globalOpsGet)
		notLogin.GET("/ops/local", localOpsGet)
		notLogin.GET("/roles/global", globalRoleGet)
		notLogin.GET("/roles/local", localRoleGet)
		notLogin.POST("/users/invite", userInvitePost)

		notLogin.GET("/auth/v2/authorize", authAuthorizeV2)
		notLogin.GET("/auth/v2/callback", authCallbackV2)
		notLogin.GET("/auth/v2/logout", logoutV2)

		notLogin.POST("/auth/send-login-code-by-sms", v1SendLoginCodeBySms)
		notLogin.POST("/auth/send-login-code-by-email", v1SendLoginCodeByEmail)
		notLogin.POST("/auth/send-rst-code-by-sms", sendRstCodeBySms)
		notLogin.POST("/auth/rst-password", rstPassword)
		notLogin.GET("/auth/captcha", captchaGet)

		notLogin.GET("/v2/nodes", nodeGets)
	}

	hbs := r.Group("/api/hbs")
	{
		hbs.POST("/heartbeat", heartBeat)
		hbs.GET("/instances", instanceGets)
	}

	rootLogin := r.Group("/api/rdb").Use(shouldBeRoot())
	{
		rootLogin.GET("/configs/smtp", smtpConfigsGet)
		rootLogin.POST("/configs/smtp/test", smtpTest)
		rootLogin.PUT("/configs/smtp", smtpConfigsPut)

		rootLogin.GET("/log/login", loginLogGets)
		rootLogin.GET("/log/operation", operationLogGets)

		rootLogin.POST("/roles", roleAddPost)
		rootLogin.PUT("/role/:id", rolePut)
		rootLogin.DELETE("/role/:id", roleDel)
		rootLogin.GET("/role/:id", roleDetail)
		rootLogin.GET("/role/:id/users", roleGlobalUsersGet)
		rootLogin.PUT("/role/:id/users/bind", roleGlobalUsersBind)
		rootLogin.PUT("/role/:id/users/unbind", roleGlobalUsersUnbind)

		rootLogin.POST("/users", userAddPost)
		rootLogin.GET("/user/:id/profile", userProfileGet)
		rootLogin.PUT("/user/:id/profile", userProfilePut)
		rootLogin.PUT("/user/:id/password", userPasswordPut)
		rootLogin.DELETE("/user/:id", userDel)

		rootLogin.POST("/node-cates", nodeCatePost)
		rootLogin.PUT("/node-cate/:id", nodeCatePut)
		rootLogin.DELETE("/node-cate/:id", nodeCateDel)
		rootLogin.POST("/node-cates/fields", nodeCateFieldNew)
		rootLogin.PUT("/node-cates/field/:id", nodeCateFieldPut)
		rootLogin.DELETE("/node-cates/field/:id", nodeCateFieldDel)

		rootLogin.GET("/nodes/trash", nodeTrashGets)
		rootLogin.PUT("/nodes/trash/recycle", nodeTrashRecycle)

		rootLogin.POST("/sso/clients", ssoClientsPost)
		rootLogin.GET("/sso/clients", ssoClientsGet)
		rootLogin.GET("/sso/clients/:clientId", ssoClientGet)
		rootLogin.PUT("/sso/clients/:clientId", ssoClientPut)
		rootLogin.DELETE("/sso/clients/:clientId", ssoClientDel)
	}

	userLogin := r.Group("/api/rdb").Use(shouldBeLogin())
	{
		userLogin.GET("/resoplogs", operationLogResGets)

		userLogin.GET("/self/profile", selfProfileGet)
		userLogin.PUT("/self/profile", selfProfilePut)
		userLogin.PUT("/self/password", selfPasswordPut)
		userLogin.GET("/self/token", selfTokenGets)
		userLogin.POST("/self/token", selfTokenPost)
		userLogin.PUT("/self/token", selfTokenPut)
		userLogin.GET("/self/perms/global", permGlobalOps)

		userLogin.GET("/users", userListGet)
		userLogin.GET("/users/invite", userInviteGet)

		userLogin.GET("/teams/all", teamAllGet)
		userLogin.GET("/teams/mine", teamMineGet)
		userLogin.POST("/teams", teamAddPost)
		userLogin.PUT("/team/:id", teamPut)
		userLogin.GET("/team/:id", teamDetail)
		userLogin.PUT("/team/:id/users/bind", teamUserBind)
		userLogin.PUT("/team/:id/users/unbind", teamUserUnbind)
		userLogin.DELETE("/team/:id", teamDel)

		userLogin.GET("/node-cates", nodeCateGets)
		userLogin.GET("/node-cates/fields", nodeCateFieldGets)
		userLogin.GET("/node-cates/field/:id", nodeCateFieldGet)

		userLogin.POST("/nodes", nodePost)
		userLogin.GET("/nodes", nodeGets)
		userLogin.GET("/node/:id", nodeGet)
		userLogin.PUT("/node/:id", nodePut)
		userLogin.DELETE("/node/:id", nodeDel)
		userLogin.GET("/node/:id/fields", nodeFieldGets)
		userLogin.PUT("/node/:id/fields", nodeFieldPuts)
		userLogin.GET("/node/:id/roles", rolesUnderNodeGets)
		userLogin.POST("/node/:id/roles", rolesUnderNodePost)
		userLogin.DELETE("/node/:id/roles", rolesUnderNodeDel)
		userLogin.GET("/node/:id/resources", resourceUnderNodeGet)
		userLogin.GET("/node/:id/resources/cate-count", renderNodeResourcesCountByCate)
		userLogin.POST("/node/:id/resources/bind", resourceBindNode)
		userLogin.POST("/node/:id/resources/unbind", resourceUnbindNode)
		userLogin.PUT("/node/:id/resources/note", resourceUnderNodeNotePut)
		userLogin.PUT("/node/:id/resources/labels", resourceUnderNodeLabelsPut)

		userLogin.GET("/tree", treeUntilLeafGets)
		userLogin.GET("/tree/projs", treeUntilProjectGets)

		userLogin.GET("/resources/search", resourceSearchGet)
		userLogin.PUT("/resources/note", resourceNotePut)
		userLogin.GET("/resources/bindings", resourceBindingsGet)
		userLogin.GET("/resources/orphan", resourceOrphanGet)
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
		v1.GET("/node/:id", nodeGet)
		v1.GET("/node/:id/projs", v1treeUntilProjectGetsByNid)
		v1.GET("/tree/projs", v1TreeUntilProjectGets)

		// 外部系统推送一些操作日志过来，RDB统一存储，实际用MQ会更好一些
		v1.POST("/resoplogs", v1OperationLogResPost)

		// 是否有权限做一些全局操作(即页面权限点)
		v1.GET("/can-do-global-op", v1CandoGlobalOp)
		// 是否在某个节点上有权限做某个操作(即资源权限点)
		v1.GET("/can-do-node-op", v1CandoNodeOp)
		// 同时校验多个操作权限点
		v1.GET("/can-do-node-ops", v1CandoNodeOps)

		// 获取用户、团队相关信息
		v1.GET("/get-username-by-uuid", v1UsernameGetByUUID)
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
		v1.POST("/send-login-code-by-sms", v1SendLoginCodeBySms)
		v1.POST("/send-login-code-by-email", v1SendLoginCodeByEmail)

		// 第三方系统获取某个用户的所有权限点
		v1.GET("/perms/global", v1PermGlobalOps)

		// 第三方系统同步权限表的数据
		v1.GET("/table/sync/role-operation", v1RoleOperationGets)
		v1.GET("/table/sync/role-global-user", v1RoleGlobalUserGets)
	}
}
