package http

import (
	"fmt"
	"os"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/csrf"

	"github.com/didi/nightingale/v5/config"
)

func configRoutes(r *gin.Engine) {
	csrfMid := csrf.Middleware(csrf.Options{
		Secret: config.Config.HTTP.CsrfSecret,
		ErrorFunc: func(c *gin.Context) {
			c.JSON(452, gin.H{"err": "csrf token mismatch"})
			c.Abort()
		},
	})

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
	pages := r.Group("/api/n9e", csrfMid)

	{
		pages.GET("/csrf", func(c *gin.Context) {
			renderData(c, csrf.GetToken(c), nil)
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
		pages.GET("/classpaths/tree", login(), classpathTreeGets)
		pages.GET("/classpaths/tree-node/:id", login(), classpathTreeNodeGetsById)
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
	pagesV2 := r.Group("/api/n9e/v2", csrfMid)
	{
		pagesV2.POST("/collect-rules", login(), collectRulesAdd)
	}

	// for thirdparty, do not expose location in nginx.conf
	v1 := r.Group("/v1/n9e")
	{
		v1.GET("/roles", rolesGet)
		v1.GET("/self/profile", selfProfileGet)
		v1.PUT("/self/profile", selfProfilePut)
		v1.PUT("/self/password", selfPasswordPut)
		v1.GET("/self/token", selfTokenGets)
		v1.POST("/self/token", selfTokenPost)
		v1.PUT("/self/token", selfTokenPut)
		v1.GET("/users", login(), userGets)
		v1.POST("/users", admin(), userAddPost)
		v1.GET("/user/:id/profile", login(), userProfileGet)
		v1.PUT("/user/:id/profile", admin(), userProfilePut)
		v1.PUT("/user/:id/status", admin(), userStatusPut)
		v1.PUT("/user/:id/password", admin(), userPasswordPut)
		v1.DELETE("/user/:id", admin(), userDel)

		v1.GET("/user-groups", login(), userGroupListGet)
		v1.GET("/user-groups/mine", login(), userGroupMineGet)
		v1.POST("/user-groups", login(), userGroupAdd)
		v1.PUT("/user-group/:id", login(), userGroupPut)
		v1.GET("/user-group/:id", login(), userGroupGet)
		v1.POST("/user-group/:id/members", login(), userGroupMemberAdd)
		v1.DELETE("/user-group/:id/members", login(), userGroupMemberDel)
		v1.DELETE("/user-group/:id", login(), userGroupDel)

		v1.GET("/classpaths", login(), classpathListGets)
		v1.GET("/classpaths/tree", login(), classpathTreeGets)
		v1.GET("/classpaths/tree-node/:id", login(), classpathTreeNodeGetsById)
		v1.POST("/classpaths", login(), classpathAdd)
		v1.PUT("/classpath/:id", login(), classpathPut)
		v1.DELETE("/classpath/:id", login(), classpathDel)
		v1.POST("/classpath/:id/resources", login(), classpathAddResources)
		v1.DELETE("/classpath/:id/resources", login(), classpathDelResources)
		v1.GET("/classpath/:id/resources", login(), classpathGetsResources)

		v1.GET("/classpaths/favorites", login(), classpathFavoriteGet)
		v1.POST("/classpath/:id/favorites", login(), classpathFavoriteAdd)
		v1.DELETE("/classpath/:id/favorites", login(), classpathFavoriteDel)

		v1.GET("/resources", login(), resourcesQuery)
		v1.PUT("/resources/note", resourceNotePut)
		v1.PUT("/resources/tags", resourceTagsPut)
		v1.PUT("/resources/classpaths", resourceClasspathsPut)
		v1.PUT("/resources/mute", resourceMutePut)
		v1.GET("/resource/:id", login(), resourceGet)
		v1.DELETE("/resource/:id", login(), resourceDel)

		v1.GET("/classpath/:id/collect-rules", login(), collectRuleGets)

		v1.GET("/mutes", login(), muteGets)
		v1.POST("/mutes", login(), muteAdd)
		v1.GET("/mute/:id", login(), muteGet)
		v1.DELETE("/mute/:id", login(), muteDel)

		v1.GET("/dashboards", login(), dashboardGets)
		v1.POST("/dashboards", login(), dashboardAdd)
		v1.GET("/dashboard/:id", login(), dashboardGet)
		v1.PUT("/dashboard/:id", login(), dashboardPut)
		v1.DELETE("/dashboard/:id", login(), dashboardDel)
		v1.POST("/dashboard/:id/favorites", login(), dashboardFavoriteAdd)
		v1.DELETE("/dashboard/:id/favorites", login(), dashboardFavoriteDel)
		v1.GET("/dashboard/:id/chart-groups", login(), chartGroupGets)
		v1.POST("/dashboard/:id/chart-groups", login(), chartGroupAdd)

		v1.PUT("/chart-groups", login(), chartGroupsPut)
		v1.DELETE("/chart-group/:id", login(), chartGroupDel)
		v1.GET("/chart-group/:id/charts", login(), chartGets)
		v1.POST("/chart-group/:id/charts", login(), chartAdd)
		v1.PUT("/chart/:id", login(), chartPut)
		v1.DELETE("/chart/:id", login(), chartDel)
		v1.PUT("/charts/configs", login(), chartConfigsPut)
		v1.GET("/charts/tmps", login(), chartTmpGets)
		v1.POST("/charts/tmps", login(), chartTmpAdd)

		v1.GET("/alert-rule-groups", login(), alertRuleGroupGets)
		v1.POST("/alert-rule-groups", login(), alertRuleGroupAdd)
		v1.GET("/alert-rule-group/:id", login(), alertRuleGroupGet)
		v1.PUT("/alert-rule-group/:id", login(), alertRuleGroupPut)
		v1.DELETE("/alert-rule-group/:id", login(), alertRuleGroupDel)

		v1.GET("/alert-rule-groups/favorites", login(), alertRuleGroupFavoriteGet)
		v1.DELETE("/alert-rule-group/:id/favorites", login(), alertRuleGroupFavoriteDel)
		v1.POST("/alert-rule-group/:id/favorites", login(), alertRuleGroupFavoriteAdd)

		v1.GET("/alert-rule-group/:id/alert-rules", login(), alertRuleOfGroupGet)
		v1.DELETE("/alert-rule-group/:id/alert-rules", login(), alertRuleOfGroupDel)

		v1.POST("/alert-rules", login(), alertRuleAdd)
		v1.PUT("/alert-rules/status", login(), alertRuleStatusPut)
		v1.PUT("/alert-rules/notify-groups", login(), alertRuleNotifyGroupsPut)
		v1.PUT("/alert-rules/notify-channels", login(), alertRuleNotifyChannelsPut)
		v1.PUT("/alert-rules/append-tags", login(), alertRuleAppendTagsPut)
		v1.GET("/alert-rule/:id", login(), alertRuleGet)
		v1.PUT("/alert-rule/:id", login(), alertRulePut)
		v1.DELETE("/alert-rule/:id", login(), alertRuleDel)

		v1.GET("/alert-events", login(), alertEventGets)
		v1.DELETE("/alert-events", login(), alertEventsDel)
		v1.GET("/alert-event/:id", login(), alertEventGet)
		v1.DELETE("/alert-event/:id", login(), alertEventDel)

		v1.GET("/history-alert-events", login(), historyAlertEventGets)
		v1.GET("/history-alert-event/:id", login(), historyAlertEventGet)

		v1.POST("/collect-rules", login(), collectRuleAdd)
		v1.DELETE("/collect-rules", login(), collectRuleDel)
		v1.PUT("/collect-rule/:id", login(), collectRulePut)
		v1.GET("/collect-rules-belong-to-ident", collectRuleGetsByIdent)
		v1.GET("/collect-rules-summary", collectRuleSummaryGetByIdent)

		v1.GET("/metric-descriptions", metricDescriptionGets)
		v1.POST("/metric-descriptions", login(), metricDescriptionAdd)
		v1.DELETE("/metric-descriptions", login(), metricDescriptionDel)
		v1.PUT("/metric-description/:id", login(), metricDescriptionPut)

		v1.GET("/contact-channels", contactChannelsGet)
		v1.GET("/notify-channels", notifyChannelsGet)

		v1.POST("/push", PushData)

		v1.GET("/status", Status)

		v1.POST("/query", GetData)
		v1.POST("/instant-query", GetDataInstant)
		v1.POST("/tag-keys", GetTagKeys)
		v1.POST("/tag-values", GetTagValues)
		v1.POST("/tag-metrics", GetMetrics)
		v1.POST("/tag-pairs", GetTagPairs)
		v1.GET("/check-promql", checkPromeQl)

		v1.GET("/can-do-op-by-name", login(), canDoOpByName)
		v1.GET("/can-do-op-by-token", login(), canDoOpByToken)
	}

	push := r.Group("/v1/n9e/series").Use(gzip.Gzip(gzip.DefaultCompression))
	{
		push.POST("", PushSeries)
	}

}
