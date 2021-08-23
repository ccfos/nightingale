package http

import (
	"fmt"
	"os"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"

	"github.com/didi/nightingale/v5/config"
)

func configRoutes(r *gin.Engine) {
	/*
		csrfMid := csrf.Middleware(csrf.Options{
			Secret: config.Config.HTTP.CsrfSecret,
			ErrorFunc: func(c *gin.Context) {
				c.JSON(452, gin.H{"err": "csrf token mismatch"})
				c.Abort()
			},
		})
	*/

	if config.Config.HTTP.Pprof {
		pprof.Register(r, "/api/debug/pprof")
	}

	guest := r.Group("/api/n9e")
	{
		guest.GET("/ping", func(c *gin.Context) {
			c.String(200, "pong")
		})
		guest.GET("/pid", func(c *gin.Context) {
			c.String(200, fmt.Sprintf("%d", os.Getpid()))
		})
		guest.GET("/addr", func(c *gin.Context) {
			c.String(200, c.Request.RemoteAddr)
		})

		guest.POST("/auth/login", loginPost)
		guest.GET("/auth/logout", logoutGet)

		// 开源版本，为了支持图表分享功能，允许匿名查询数据
		guest.POST("/query", GetData)
		guest.POST("/instant-query", GetDataInstant)
		guest.POST("/tag-pairs", GetTagPairs)
		guest.POST("/tag-keys", GetTagKeys)
		guest.POST("/tag-values", GetTagValues)
		guest.POST("/tag-metrics", GetMetrics)
		guest.GET("/check-promql", checkPromeQl)
	}

	// for brower, expose location in nginx.conf
	pages := r.Group("/api/n9e")

	{

		pages.GET("/csrf", func(c *gin.Context) {
			// renderData(c, csrf.GetToken(c), nil)
			renderData(c, "not supported", nil)
		})

		pages.GET("/roles", rolesGet)
		pages.GET("/self/profile", selfProfileGet)
		pages.PUT("/self/profile", selfProfilePut)
		pages.PUT("/self/password", selfPasswordPut)
		pages.GET("/self/token", selfTokenGets)
		pages.POST("/self/token", selfTokenPost)
		pages.PUT("/self/token", selfTokenPut)
		pages.GET("/users", login(), userGets)
		pages.POST("/users", admin(), userAddPost)
		pages.GET("/user/:id/profile", login(), userProfileGet)
		pages.PUT("/user/:id/profile", admin(), userProfilePut)
		pages.PUT("/user/:id/status", admin(), userStatusPut)
		pages.PUT("/user/:id/password", admin(), userPasswordPut)
		pages.DELETE("/user/:id", admin(), userDel)

		pages.GET("/user-groups", login(), userGroupListGet)
		pages.GET("/user-groups/mine", login(), userGroupMineGet)
		pages.POST("/user-groups", login(), userGroupAdd)
		pages.PUT("/user-group/:id", login(), userGroupPut)
		pages.GET("/user-group/:id", login(), userGroupGet)
		pages.POST("/user-group/:id/members", login(), userGroupMemberAdd)
		pages.DELETE("/user-group/:id/members", login(), userGroupMemberDel)
		pages.DELETE("/user-group/:id", login(), userGroupDel)

		pages.GET("/classpaths", login(), classpathListGets)
		pages.GET("/classpaths/tree", login(), classpathListNodeGets)
		pages.GET("/classpaths/tree-node/:id", login(), classpathListNodeGetsById)
		pages.POST("/classpaths", login(), classpathAdd)
		pages.PUT("/classpath/:id", login(), classpathPut)
		pages.DELETE("/classpath/:id", login(), classpathDel)
		pages.POST("/classpath/:id/resources", login(), classpathAddResources)
		pages.DELETE("/classpath/:id/resources", login(), classpathDelResources)
		pages.GET("/classpath/:id/resources", login(), classpathGetsResources)

		pages.GET("/classpaths/favorites", login(), classpathFavoriteGet)
		pages.POST("/classpath/:id/favorites", login(), classpathFavoriteAdd)
		pages.DELETE("/classpath/:id/favorites", login(), classpathFavoriteDel)

		pages.GET("/resources", login(), resourcesQuery)
		pages.PUT("/resources/note", resourceNotePut)
		pages.PUT("/resources/tags", resourceTagsPut)
		pages.PUT("/resources/classpaths", resourceClasspathsPut)
		pages.PUT("/resources/mute", resourceMutePut)
		pages.GET("/resource/:id", login(), resourceGet)
		pages.DELETE("/resource/:id", login(), resourceDel)

		pages.GET("/mutes", login(), muteGets)
		pages.POST("/mutes", login(), muteAdd)
		pages.GET("/mute/:id", login(), muteGet)
		pages.DELETE("/mute/:id", login(), muteDel)

		pages.GET("/dashboards", login(), dashboardGets)
		pages.POST("/dashboards", login(), dashboardAdd)
		pages.POST("/dashboards-clone", login(), dashboardClone)
		pages.POST("/dashboards/import", login(), dashboardImport)
		pages.POST("/dashboards/export", login(), dashboardExport)
		pages.GET("/dashboard/:id", login(), dashboardGet)
		pages.PUT("/dashboard/:id", login(), dashboardPut)
		pages.DELETE("/dashboard/:id", login(), dashboardDel)
		pages.POST("/dashboard/:id/favorites", login(), dashboardFavoriteAdd)
		pages.DELETE("/dashboard/:id/favorites", login(), dashboardFavoriteDel)
		pages.GET("/dashboard/:id/chart-groups", login(), chartGroupGets)
		pages.POST("/dashboard/:id/chart-groups", login(), chartGroupAdd)

		pages.PUT("/chart-groups", login(), chartGroupsPut)
		pages.DELETE("/chart-group/:id", login(), chartGroupDel)
		pages.GET("/chart-group/:id/charts", login(), chartGets)
		pages.POST("/chart-group/:id/charts", login(), chartAdd)
		pages.PUT("/chart/:id", login(), chartPut)
		pages.DELETE("/chart/:id", login(), chartDel)
		pages.PUT("/charts/configs", login(), chartConfigsPut)
		pages.GET("/charts/tmps", chartTmpGets)
		pages.POST("/charts/tmps", login(), chartTmpAdd)

		pages.GET("/alert-rule-groups", login(), alertRuleGroupGets)
		pages.GET("/alert-rule-groups/favorites", login(), alertRuleGroupFavoriteGet)
		pages.POST("/alert-rule-groups", login(), alertRuleGroupAdd)
		pages.GET("/alert-rule-group/:id", login(), alertRuleGroupGet)
		pages.GET("/alert-rule-group/:id/alert-rules", login(), alertRuleOfGroupGet)
		pages.DELETE("/alert-rule-group/:id/alert-rules", login(), alertRuleOfGroupDel)
		pages.PUT("/alert-rule-group/:id", login(), alertRuleGroupPut)
		pages.DELETE("/alert-rule-group/:id", login(), alertRuleGroupDel)
		pages.POST("/alert-rule-group/:id/favorites", login(), alertRuleGroupFavoriteAdd)
		pages.DELETE("/alert-rule-group/:id/favorites", login(), alertRuleGroupFavoriteDel)

		pages.POST("/alert-rules", login(), alertRuleAdd)
		pages.PUT("/alert-rules/status", login(), alertRuleStatusPut)
		pages.PUT("/alert-rules/notify-groups", login(), alertRuleNotifyGroupsPut)
		pages.PUT("/alert-rules/notify-channels", login(), alertRuleNotifyChannelsPut)
		pages.PUT("/alert-rules/append-tags", login(), alertRuleAppendTagsPut)
		pages.GET("/alert-rule/:id", login(), alertRuleGet)
		pages.PUT("/alert-rule/:id", login(), alertRulePut)
		pages.DELETE("/alert-rule/:id", login(), alertRuleDel)

		pages.GET("/alert-events", login(), alertEventGets)
		pages.DELETE("/alert-events", login(), alertEventsDel)
		pages.GET("/alert-event/:id", login(), alertEventGet)
		pages.DELETE("/alert-event/:id", login(), alertEventDel)
		pages.PUT("/alert-event/:id", login(), alertEventNotePut)

		pages.GET("/history-alert-events", login(), historyAlertEventGets)
		pages.GET("/history-alert-event/:id", login(), historyAlertEventGet)

		pages.GET("/classpath/:id/collect-rules", login(), collectRuleGets)
		pages.POST("/collect-rules", login(), collectRuleAdd)
		pages.DELETE("/collect-rules", login(), collectRuleDel)
		pages.PUT("/collect-rule/:id", login(), collectRulePut)
		pages.POST("/log/check", regExpCheck)

		pages.GET("/metric-descriptions", metricDescriptionGets)
		pages.POST("/metric-descriptions", login(), metricDescriptionAdd)
		pages.DELETE("/metric-descriptions", login(), metricDescriptionDel)
		pages.PUT("/metric-description/:id", login(), metricDescriptionPut)

		pages.GET("/contact-channels", contactChannelsGet)
		pages.GET("/notify-channels", notifyChannelsGet)

		pages.GET("/tpl/list", tplNameGets)
		pages.GET("/tpl/content", tplGet)

		pages.GET("/status", Status)

	}

	// for brower, expose location in nginx.conf
	pagesV2 := r.Group("/api/n9e/v2")
	{
		pagesV2.POST("/collect-rules", login(), collectRulesAdd)
	}

	// for thirdparty, do not expose location in nginx.conf
	v1 := r.Group("/v1/n9e")
	{
		v1.POST("/query", GetData)
		v1.POST("/instant-query", GetDataInstant)
		v1.POST("/tag-keys", GetTagKeys)
		v1.POST("/tag-values", GetTagValues)
		v1.POST("/tag-pairs", GetTagPairs)
		v1.POST("/tag-metrics", GetMetrics)
		v1.POST("/push", PushData)
		v1.GET("/collect-rules-belong-to-ident", collectRuleGetsByIdent)
		v1.GET("/collect-rules-summary", collectRuleSummaryGetByIdent)

		v1.GET("/can-do-op-by-name", login(), canDoOpByName)
		v1.GET("/can-do-op-by-token", login(), canDoOpByToken)
		v1.GET("/get-user-by-name", login(), getUserByName)
	}

	push := r.Group("/v1/n9e/series").Use(gzip.Gzip(gzip.DefaultCompression))
	{
		push.POST("", PushSeries)
	}

}
