package router

import (
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/pkg/aop"
	"github.com/didi/nightingale/v5/src/webapi/config"
	promstat "github.com/didi/nightingale/v5/src/webapi/stat"
)

func stat() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		code := fmt.Sprintf("%d", c.Writer.Status())
		method := c.Request.Method
		labels := []string{promstat.Service, code, c.FullPath(), method}

		promstat.RequestCounter.WithLabelValues(labels...).Inc()
		promstat.RequestDuration.WithLabelValues(labels...).Observe(float64(time.Since(start).Seconds()))
	}
}

func languageDetector() gin.HandlerFunc {
	headerKey := config.C.I18NHeaderKey
	return func(c *gin.Context) {
		if headerKey != "" {
			lang := c.GetHeader(headerKey)
			if lang != "" {
				if strings.HasPrefix(lang, "*") || strings.HasPrefix(lang, "zh") {
					c.Request.Header.Set("X-Language", "zh")
				} else if strings.HasPrefix(lang, "en") {
					c.Request.Header.Set("X-Language", "en")
				} else {
					c.Request.Header.Set("X-Language", lang)
				}
			}
		}
		c.Next()
	}
}

func New(version string) *gin.Engine {
	gin.SetMode(config.C.RunMode)

	if strings.ToLower(config.C.RunMode) == "release" {
		aop.DisableConsoleColor()
	}

	r := gin.New()

	r.Use(stat())
	r.Use(languageDetector())
	r.Use(aop.Recovery())

	// whether print access log
	if config.C.HTTP.PrintAccessLog {
		r.Use(aop.Logger())
	}

	configRoute(r, version)
	configNoRoute(r)

	return r
}

func configNoRoute(r *gin.Engine) {
	r.NoRoute(func(c *gin.Context) {
		arr := strings.Split(c.Request.URL.Path, ".")
		suffix := arr[len(arr)-1]
		switch suffix {
		case "png", "jpeg", "jpg", "svg", "ico", "gif", "css", "js", "html", "htm", "gz", "zip", "map":
			c.File(path.Join(strings.Split("pub/"+c.Request.URL.Path, "/")...))
		default:
			c.File(path.Join("pub", "index.html"))
		}
	})
}

func configRoute(r *gin.Engine, version string) {
	if config.C.HTTP.PProf {
		pprof.Register(r, "/api/debug/pprof")
	}

	r.GET("/ping", func(c *gin.Context) {
		c.String(200, "pong")
	})

	r.GET("/pid", func(c *gin.Context) {
		c.String(200, fmt.Sprintf("%d", os.Getpid()))
	})

	r.GET("/addr", func(c *gin.Context) {
		c.String(200, c.Request.RemoteAddr)
	})

	r.GET("/version", func(c *gin.Context) {
		c.String(200, version)
	})

	r.GET("/i18n", func(c *gin.Context) {
		ginx.NewRender(c).Message("just a test: %s", "by ulric")
	})

	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	pagesPrefix := "/api/n9e"

	pages := r.Group(pagesPrefix)
	{
		if config.C.AnonymousAccess.PromQuerier {
			pages.Any("/prometheus/*url", prometheusProxy)
			pages.POST("/query-range-batch", promBatchQueryRange)
		} else {
			pages.Any("/prometheus/*url", auth(), prometheusProxy)
			pages.POST("/query-range-batch", auth(), promBatchQueryRange)
		}

		pages.GET("/version", func(c *gin.Context) {
			c.String(200, version)
		})

		pages.POST("/auth/login", jwtMock(), loginPost)
		pages.POST("/auth/logout", jwtMock(), logoutPost)
		pages.POST("/auth/refresh", jwtMock(), refreshPost)

		pages.GET("/auth/sso-config", ssoConfigGet)
		pages.GET("/auth/redirect", loginRedirect)
		pages.GET("/auth/redirect/cas", loginRedirectCas)
		pages.GET("/auth/redirect/oauth", loginRedirectOAuth)
		pages.GET("/auth/callback", loginCallback)
		pages.GET("/auth/callback/cas", loginCallbackCas)
		pages.GET("/auth/callback/oauth", loginCallbackOAuth)

		pages.GET("/metrics/desc", metricsDescGetFile)
		pages.POST("/metrics/desc", metricsDescGetMap)

		pages.GET("/roles", rolesGets)
		pages.GET("/notify-channels", notifyChannelsGets)
		pages.GET("/contact-keys", contactKeysGets)
		pages.GET("/clusters", clustersGets)

		pages.GET("/self/perms", auth(), user(), permsGets)
		pages.GET("/self/profile", auth(), user(), selfProfileGet)
		pages.PUT("/self/profile", auth(), user(), selfProfilePut)
		pages.PUT("/self/password", auth(), user(), selfPasswordPut)

		pages.GET("/users", auth(), user(), perm("/users"), userGets)
		pages.POST("/users", auth(), admin(), userAddPost)
		pages.GET("/user/:id/profile", auth(), userProfileGet)
		pages.PUT("/user/:id/profile", auth(), admin(), userProfilePut)
		pages.PUT("/user/:id/password", auth(), admin(), userPasswordPut)
		pages.DELETE("/user/:id", auth(), admin(), userDel)

		pages.GET("/metric-views", auth(), metricViewGets)
		pages.DELETE("/metric-views", auth(), user(), metricViewDel)
		pages.POST("/metric-views", auth(), user(), metricViewAdd)
		pages.PUT("/metric-views", auth(), user(), metricViewPut)

		pages.GET("/user-groups", auth(), user(), userGroupGets)
		pages.POST("/user-groups", auth(), user(), perm("/user-groups/add"), userGroupAdd)
		pages.GET("/user-group/:id", auth(), user(), userGroupGet)
		pages.PUT("/user-group/:id", auth(), user(), perm("/user-groups/put"), userGroupWrite(), userGroupPut)
		pages.DELETE("/user-group/:id", auth(), user(), perm("/user-groups/del"), userGroupWrite(), userGroupDel)
		pages.POST("/user-group/:id/members", auth(), user(), perm("/user-groups/put"), userGroupWrite(), userGroupMemberAdd)
		pages.DELETE("/user-group/:id/members", auth(), user(), perm("/user-groups/put"), userGroupWrite(), userGroupMemberDel)

		pages.GET("/busi-groups", auth(), user(), busiGroupGets)
		pages.POST("/busi-groups", auth(), user(), perm("/busi-groups/add"), busiGroupAdd)
		pages.GET("/busi-groups/alertings", auth(), busiGroupAlertingsGets)
		pages.GET("/busi-group/:id", auth(), user(), bgro(), busiGroupGet)
		pages.PUT("/busi-group/:id", auth(), user(), perm("/busi-groups/put"), bgrw(), busiGroupPut)
		pages.POST("/busi-group/:id/members", auth(), user(), perm("/busi-groups/put"), bgrw(), busiGroupMemberAdd)
		pages.DELETE("/busi-group/:id/members", auth(), user(), perm("/busi-groups/put"), bgrw(), busiGroupMemberDel)
		pages.DELETE("/busi-group/:id", auth(), user(), perm("/busi-groups/del"), bgrw(), busiGroupDel)
		pages.GET("/busi-group/:id/perm/:perm", auth(), user(), checkBusiGroupPerm)

		pages.GET("/targets", auth(), user(), targetGets)
		pages.DELETE("/targets", auth(), user(), perm("/targets/del"), targetDel)
		pages.GET("/targets/tags", auth(), user(), targetGetTags)
		pages.POST("/targets/tags", auth(), user(), perm("/targets/put"), targetBindTagsByFE)
		pages.DELETE("/targets/tags", auth(), user(), perm("/targets/put"), targetUnbindTagsByFE)
		pages.PUT("/targets/note", auth(), user(), perm("/targets/put"), targetUpdateNote)
		pages.PUT("/targets/bgid", auth(), user(), perm("/targets/put"), targetUpdateBgid)

		pages.GET("/builtin-boards", builtinBoardGets)
		pages.GET("/builtin-board/:name", builtinBoardGet)

		pages.GET("/busi-group/:id/boards", auth(), user(), perm("/dashboards"), bgro(), boardGets)
		pages.POST("/busi-group/:id/boards", auth(), user(), perm("/dashboards/add"), bgrw(), boardAdd)
		pages.POST("/busi-group/:id/board/:bid/clone", auth(), user(), perm("/dashboards/add"), bgrw(), boardClone)

		pages.GET("/board/:bid", boardGet)
		pages.GET("/board/:bid/pure", boardPureGet)
		pages.PUT("/board/:bid", auth(), user(), perm("/dashboards/put"), boardPut)
		pages.PUT("/board/:bid/configs", auth(), user(), perm("/dashboards/put"), boardPutConfigs)
		pages.PUT("/board/:bid/public", auth(), user(), perm("/dashboards/put"), boardPutPublic)
		pages.DELETE("/boards", auth(), user(), perm("/dashboards/del"), boardDel)

		// migrate v5.8.0
		pages.GET("/dashboards", auth(), admin(), migrateDashboards)
		pages.GET("/dashboard/:id", auth(), admin(), migrateDashboardGet)
		pages.PUT("/dashboard/:id/migrate", auth(), admin(), migrateDashboard)

		// deprecated ↓
		pages.GET("/dashboards/builtin/list", builtinBoardGets)
		pages.POST("/busi-group/:id/dashboards/builtin", auth(), user(), perm("/dashboards/add"), bgrw(), dashboardBuiltinImport)
		pages.GET("/busi-group/:id/dashboards", auth(), user(), perm("/dashboards"), bgro(), dashboardGets)
		pages.POST("/busi-group/:id/dashboards", auth(), user(), perm("/dashboards/add"), bgrw(), dashboardAdd)
		pages.POST("/busi-group/:id/dashboards/export", auth(), user(), perm("/dashboards"), bgro(), dashboardExport)
		pages.POST("/busi-group/:id/dashboards/import", auth(), user(), perm("/dashboards/add"), bgrw(), dashboardImport)
		pages.POST("/busi-group/:id/dashboard/:did/clone", auth(), user(), perm("/dashboards/add"), bgrw(), dashboardClone)
		pages.GET("/busi-group/:id/dashboard/:did", auth(), user(), perm("/dashboards"), bgro(), dashboardGet)
		pages.PUT("/busi-group/:id/dashboard/:did", auth(), user(), perm("/dashboards/put"), bgrw(), dashboardPut)
		pages.DELETE("/busi-group/:id/dashboard/:did", auth(), user(), perm("/dashboards/del"), bgrw(), dashboardDel)

		pages.GET("/busi-group/:id/chart-groups", auth(), user(), bgro(), chartGroupGets)
		pages.POST("/busi-group/:id/chart-groups", auth(), user(), bgrw(), chartGroupAdd)
		pages.PUT("/busi-group/:id/chart-groups", auth(), user(), bgrw(), chartGroupPut)
		pages.DELETE("/busi-group/:id/chart-groups", auth(), user(), bgrw(), chartGroupDel)

		pages.GET("/busi-group/:id/charts", auth(), user(), bgro(), chartGets)
		pages.POST("/busi-group/:id/charts", auth(), user(), bgrw(), chartAdd)
		pages.PUT("/busi-group/:id/charts", auth(), user(), bgrw(), chartPut)
		pages.DELETE("/busi-group/:id/charts", auth(), user(), bgrw(), chartDel)
		// deprecated ↑

		pages.GET("/share-charts", chartShareGets)
		pages.POST("/share-charts", auth(), chartShareAdd)

		pages.GET("/alert-rules/builtin/list", alertRuleBuiltinList)
		pages.POST("/busi-group/:id/alert-rules/builtin", auth(), user(), perm("/alert-rules/add"), bgrw(), alertRuleBuiltinImport)
		pages.GET("/busi-group/:id/alert-rules", auth(), user(), perm("/alert-rules"), alertRuleGets)
		pages.POST("/busi-group/:id/alert-rules", auth(), user(), perm("/alert-rules/add"), bgrw(), alertRuleAddByFE)
		pages.DELETE("/busi-group/:id/alert-rules", auth(), user(), perm("/alert-rules/del"), bgrw(), alertRuleDel)
		pages.PUT("/busi-group/:id/alert-rules/fields", auth(), user(), perm("/alert-rules/put"), bgrw(), alertRulePutFields)
		pages.PUT("/busi-group/:id/alert-rule/:arid", auth(), user(), perm("/alert-rules/put"), alertRulePutByFE)
		pages.GET("/alert-rule/:arid", auth(), user(), perm("/alert-rules"), alertRuleGet)

		pages.GET("/busi-group/:id/recording-rules", auth(), user(), perm("/recording-rules"), recordingRuleGets)
		pages.POST("/busi-group/:id/recording-rules", auth(), user(), perm("/recording-rules/add"), bgrw(), recordingRuleAddByFE)
		pages.DELETE("/busi-group/:id/recording-rules", auth(), user(), perm("/recording-rules/del"), bgrw(), recordingRuleDel)
		pages.PUT("/busi-group/:id/recording-rule/:rrid", auth(), user(), perm("/recording-rules/put"), bgrw(), recordingRulePutByFE)
		pages.GET("/recording-rule/:rrid", auth(), user(), perm("/recording-rules"), recordingRuleGet)
		pages.PUT("/busi-group/:id/recording-rules/fields", auth(), user(), perm("/recording-rules/put"), recordingRulePutFields)

		pages.GET("/busi-group/:id/alert-mutes", auth(), user(), perm("/alert-mutes"), bgro(), alertMuteGetsByBG)
		pages.POST("/busi-group/:id/alert-mutes", auth(), user(), perm("/alert-mutes/add"), bgrw(), alertMuteAdd)
		pages.DELETE("/busi-group/:id/alert-mutes", auth(), user(), perm("/alert-mutes/del"), bgrw(), alertMuteDel)
		pages.PUT("/busi-group/:id/alert-mute/:amid", auth(), user(), perm("/alert-mutes/put"), alertMutePutByFE)
		pages.PUT("/busi-group/:id/alert-mutes/fields", auth(), user(), perm("/alert-mutes/put"), bgrw(), alertMutePutFields)

		pages.GET("/busi-group/:id/alert-subscribes", auth(), user(), perm("/alert-subscribes"), bgro(), alertSubscribeGets)
		pages.GET("/alert-subscribe/:sid", auth(), user(), perm("/alert-subscribes"), alertSubscribeGet)
		pages.POST("/busi-group/:id/alert-subscribes", auth(), user(), perm("/alert-subscribes/add"), bgrw(), alertSubscribeAdd)
		pages.PUT("/busi-group/:id/alert-subscribes", auth(), user(), perm("/alert-subscribes/put"), bgrw(), alertSubscribePut)
		pages.DELETE("/busi-group/:id/alert-subscribes", auth(), user(), perm("/alert-subscribes/del"), bgrw(), alertSubscribeDel)

		if config.C.AnonymousAccess.AlertDetail {
			pages.GET("/alert-cur-event/:eid", alertCurEventGet)
			pages.GET("/alert-his-event/:eid", alertHisEventGet)
		} else {
			pages.GET("/alert-cur-event/:eid", auth(), alertCurEventGet)
			pages.GET("/alert-his-event/:eid", auth(), alertHisEventGet)
		}

		// card logic
		pages.GET("/alert-cur-events/list", auth(), alertCurEventsList)
		pages.GET("/alert-cur-events/card", auth(), alertCurEventsCard)
		pages.POST("/alert-cur-events/card/details", auth(), alertCurEventsCardDetails)
		pages.GET("/alert-his-events/list", auth(), alertHisEventsList)
		pages.DELETE("/alert-cur-events", auth(), user(), perm("/alert-cur-events/del"), alertCurEventDel)

		pages.GET("/alert-aggr-views", auth(), alertAggrViewGets)
		pages.DELETE("/alert-aggr-views", auth(), user(), alertAggrViewDel)
		pages.POST("/alert-aggr-views", auth(), user(), alertAggrViewAdd)
		pages.PUT("/alert-aggr-views", auth(), user(), alertAggrViewPut)

		pages.GET("/busi-group/:id/task-tpls", auth(), user(), perm("/job-tpls"), bgro(), taskTplGets)
		pages.POST("/busi-group/:id/task-tpls", auth(), user(), perm("/job-tpls/add"), bgrw(), taskTplAdd)
		pages.DELETE("/busi-group/:id/task-tpl/:tid", auth(), user(), perm("/job-tpls/del"), bgrw(), taskTplDel)
		pages.POST("/busi-group/:id/task-tpls/tags", auth(), user(), perm("/job-tpls/put"), bgrw(), taskTplBindTags)
		pages.DELETE("/busi-group/:id/task-tpls/tags", auth(), user(), perm("/job-tpls/put"), bgrw(), taskTplUnbindTags)
		pages.GET("/busi-group/:id/task-tpl/:tid", auth(), user(), perm("/job-tpls"), bgro(), taskTplGet)
		pages.PUT("/busi-group/:id/task-tpl/:tid", auth(), user(), perm("/job-tpls/put"), bgrw(), taskTplPut)

		pages.GET("/busi-group/:id/tasks", auth(), user(), perm("/job-tasks"), bgro(), taskGets)
		pages.POST("/busi-group/:id/tasks", auth(), user(), perm("/job-tasks/add"), bgrw(), taskAdd)
		pages.GET("/busi-group/:id/task/*url", auth(), user(), perm("/job-tasks"), taskProxy)
		pages.PUT("/busi-group/:id/task/*url", auth(), user(), perm("/job-tasks/put"), bgrw(), taskProxy)

		pages.GET("/servers", auth(), admin(), serversGet)
		pages.PUT("/server/:id", auth(), admin(), serverBindCluster)
		pages.POST("/servers", auth(), admin(), serverAddCluster)
		pages.DELETE("/servers", auth(), admin(), serverDelCluster)
	}

	service := r.Group("/v1/n9e")
	if len(config.C.BasicAuth) > 0 {
		service.Use(gin.BasicAuth(config.C.BasicAuth))
	}
	{
		service.Any("/prometheus/*url", prometheusProxy)
		service.POST("/users", userAddPost)
		service.GET("/users", userFindAll)

		service.GET("/targets", targetGets)
		service.GET("/targets/tags", targetGetTags)
		service.POST("/targets/tags", targetBindTagsByService)
		service.DELETE("/targets/tags", targetUnbindTagsByService)
		service.PUT("/targets/note", targetUpdateNoteByService)

		service.POST("/alert-rules", alertRuleAddByService)
		service.DELETE("/alert-rules", alertRuleDelByService)
		service.PUT("/alert-rule/:arid", alertRulePutByService)
		service.GET("/alert-rule/:arid", alertRuleGet)
		service.GET("/alert-rules", alertRulesGetByService)

		service.GET("/alert-mutes", alertMuteGets)
		service.POST("/alert-mutes", alertMuteAddByService)
		service.DELETE("/alert-mutes", alertMuteDel)

		service.GET("/alert-cur-events", alertCurEventsList)
		service.GET("/alert-his-events", alertHisEventsList)
		service.GET("/alert-his-event/:eid", alertHisEventGet)

		service.GET("/config/:id", configGet)
		service.GET("/configs", configsGet)
		service.PUT("/configs", configsPut)
		service.POST("/configs", configsPost)
		service.DELETE("/configs", configsDel)

		service.POST("/conf-prop/encrypt", confPropEncrypt)
		service.POST("/conf-prop/decrypt", confPropDecrypt)
	}
}
