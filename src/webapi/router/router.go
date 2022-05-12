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

func New(version string) *gin.Engine {
	gin.SetMode(config.C.RunMode)

	if strings.ToLower(config.C.RunMode) == "release" {
		aop.DisableConsoleColor()
	}

	r := gin.New()

	r.Use(stat())
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
		} else {
			pages.Any("/prometheus/*url", jwtAuth(), prometheusProxy)
		}

		pages.GET("/version", func(c *gin.Context) {
			c.String(200, version)
		})

		pages.POST("/auth/login", loginPost)
		pages.POST("/auth/logout", logoutPost)
		pages.POST("/auth/refresh", refreshPost)

		pages.GET("/auth/redirect", loginRedirect)
		pages.GET("/auth/callback", loginCallback)

		pages.GET("/metrics/desc", metricsDescGetFile)
		pages.POST("/metrics/desc", metricsDescGetMap)

		pages.GET("/roles", rolesGets)
		pages.GET("/notify-channels", notifyChannelsGets)
		pages.GET("/contact-keys", contactKeysGets)
		pages.GET("/clusters", clustersGets)

		pages.GET("/self/perms", jwtAuth(), user(), permsGets)
		pages.GET("/self/profile", jwtAuth(), user(), selfProfileGet)
		pages.PUT("/self/profile", jwtAuth(), user(), selfProfilePut)
		pages.PUT("/self/password", jwtAuth(), user(), selfPasswordPut)

		pages.GET("/users", jwtAuth(), user(), perm("/users"), userGets)
		pages.POST("/users", jwtAuth(), admin(), userAddPost)
		pages.GET("/user/:id/profile", jwtAuth(), userProfileGet)
		pages.PUT("/user/:id/profile", jwtAuth(), admin(), userProfilePut)
		pages.PUT("/user/:id/password", jwtAuth(), admin(), userPasswordPut)
		pages.DELETE("/user/:id", jwtAuth(), admin(), userDel)

		pages.GET("/metric-views", jwtAuth(), metricViewGets)
		pages.DELETE("/metric-views", jwtAuth(), user(), metricViewDel)
		pages.POST("/metric-views", jwtAuth(), user(), metricViewAdd)
		pages.PUT("/metric-views", jwtAuth(), user(), metricViewPut)

		pages.GET("/user-groups", jwtAuth(), user(), userGroupGets)
		pages.POST("/user-groups", jwtAuth(), user(), perm("/user-groups/add"), userGroupAdd)
		pages.GET("/user-group/:id", jwtAuth(), user(), userGroupGet)
		pages.PUT("/user-group/:id", jwtAuth(), user(), perm("/user-groups/put"), userGroupWrite(), userGroupPut)
		pages.DELETE("/user-group/:id", jwtAuth(), user(), perm("/user-groups/del"), userGroupWrite(), userGroupDel)
		pages.POST("/user-group/:id/members", jwtAuth(), user(), perm("/user-groups/put"), userGroupWrite(), userGroupMemberAdd)
		pages.DELETE("/user-group/:id/members", jwtAuth(), user(), perm("/user-groups/put"), userGroupWrite(), userGroupMemberDel)

		pages.GET("/busi-groups", jwtAuth(), user(), busiGroupGets)
		pages.POST("/busi-groups", jwtAuth(), user(), perm("/busi-groups/add"), busiGroupAdd)
		pages.GET("/busi-groups/alertings", jwtAuth(), busiGroupAlertingsGets)
		pages.GET("/busi-group/:id", jwtAuth(), user(), bgro(), busiGroupGet)
		pages.PUT("/busi-group/:id", jwtAuth(), user(), perm("/busi-groups/put"), bgrw(), busiGroupPut)
		pages.POST("/busi-group/:id/members", jwtAuth(), user(), perm("/busi-groups/put"), bgrw(), busiGroupMemberAdd)
		pages.DELETE("/busi-group/:id/members", jwtAuth(), user(), perm("/busi-groups/put"), bgrw(), busiGroupMemberDel)
		pages.DELETE("/busi-group/:id", jwtAuth(), user(), perm("/busi-groups/del"), bgrw(), busiGroupDel)
		pages.GET("/busi-group/:id/perm/:perm", jwtAuth(), user(), checkBusiGroupPerm)

		pages.GET("/targets", jwtAuth(), user(), targetGets)
		pages.DELETE("/targets", jwtAuth(), user(), perm("/targets/del"), targetDel)
		pages.GET("/targets/tags", jwtAuth(), user(), targetGetTags)
		pages.POST("/targets/tags", jwtAuth(), user(), perm("/targets/put"), targetBindTags)
		pages.DELETE("/targets/tags", jwtAuth(), user(), perm("/targets/put"), targetUnbindTags)
		pages.PUT("/targets/note", jwtAuth(), user(), perm("/targets/put"), targetUpdateNote)
		pages.PUT("/targets/bgid", jwtAuth(), user(), perm("/targets/put"), targetUpdateBgid)

		pages.GET("/dashboards/builtin/list", dashboardBuiltinList)
		pages.POST("/busi-group/:id/dashboards/builtin", jwtAuth(), user(), perm("/dashboards/add"), bgrw(), dashboardBuiltinImport)
		pages.GET("/busi-group/:id/dashboards", jwtAuth(), user(), perm("/dashboards"), bgro(), dashboardGets)
		pages.POST("/busi-group/:id/dashboards", jwtAuth(), user(), perm("/dashboards/add"), bgrw(), dashboardAdd)
		pages.POST("/busi-group/:id/dashboards/export", jwtAuth(), user(), perm("/dashboards"), bgro(), dashboardExport)
		pages.POST("/busi-group/:id/dashboards/import", jwtAuth(), user(), perm("/dashboards/add"), bgrw(), dashboardImport)
		pages.POST("/busi-group/:id/dashboard/:did/clone", jwtAuth(), user(), perm("/dashboards/add"), bgrw(), dashboardClone)
		pages.GET("/busi-group/:id/dashboard/:did", jwtAuth(), user(), perm("/dashboards"), bgro(), dashboardGet)
		pages.PUT("/busi-group/:id/dashboard/:did", jwtAuth(), user(), perm("/dashboards/put"), bgrw(), dashboardPut)
		pages.DELETE("/busi-group/:id/dashboard/:did", jwtAuth(), user(), perm("/dashboards/del"), bgrw(), dashboardDel)

		pages.GET("/busi-group/:id/chart-groups", jwtAuth(), user(), bgro(), chartGroupGets)
		pages.POST("/busi-group/:id/chart-groups", jwtAuth(), user(), bgrw(), chartGroupAdd)
		pages.PUT("/busi-group/:id/chart-groups", jwtAuth(), user(), bgrw(), chartGroupPut)
		pages.DELETE("/busi-group/:id/chart-groups", jwtAuth(), user(), bgrw(), chartGroupDel)

		pages.GET("/busi-group/:id/charts", jwtAuth(), user(), bgro(), chartGets)
		pages.POST("/busi-group/:id/charts", jwtAuth(), user(), bgrw(), chartAdd)
		pages.PUT("/busi-group/:id/charts", jwtAuth(), user(), bgrw(), chartPut)
		pages.DELETE("/busi-group/:id/charts", jwtAuth(), user(), bgrw(), chartDel)

		pages.GET("/share-charts", chartShareGets)
		pages.POST("/share-charts", jwtAuth(), chartShareAdd)

		pages.GET("/alert-rules/builtin/list", alertRuleBuiltinList)
		pages.POST("/busi-group/:id/alert-rules/builtin", jwtAuth(), user(), perm("/alert-rules/add"), bgrw(), alertRuleBuiltinImport)
		pages.GET("/busi-group/:id/alert-rules", jwtAuth(), user(), perm("/alert-rules"), alertRuleGets)
		pages.POST("/busi-group/:id/alert-rules", jwtAuth(), user(), perm("/alert-rules/add"), bgrw(), alertRuleAdd)
		pages.DELETE("/busi-group/:id/alert-rules", jwtAuth(), user(), perm("/alert-rules/del"), bgrw(), alertRuleDel)
		pages.PUT("/busi-group/:id/alert-rules/fields", jwtAuth(), user(), perm("/alert-rules/put"), bgrw(), alertRulePutFields)
		pages.PUT("/busi-group/:id/alert-rule/:arid", jwtAuth(), user(), perm("/alert-rules/put"), alertRulePut)
		pages.GET("/alert-rule/:arid", jwtAuth(), user(), perm("/alert-rules"), alertRuleGet)

		pages.GET("/busi-group/:id/alert-mutes", jwtAuth(), user(), perm("/alert-mutes"), bgro(), alertMuteGets)
		pages.POST("/busi-group/:id/alert-mutes", jwtAuth(), user(), perm("/alert-mutes/add"), bgrw(), alertMuteAdd)
		pages.DELETE("/busi-group/:id/alert-mutes", jwtAuth(), user(), perm("/alert-mutes/del"), bgrw(), alertMuteDel)

		pages.GET("/busi-group/:id/alert-subscribes", jwtAuth(), user(), perm("/alert-subscribes"), bgro(), alertSubscribeGets)
		pages.GET("/alert-subscribe/:sid", jwtAuth(), user(), perm("/alert-subscribes"), alertSubscribeGet)
		pages.POST("/busi-group/:id/alert-subscribes", jwtAuth(), user(), perm("/alert-subscribes/add"), bgrw(), alertSubscribeAdd)
		pages.PUT("/busi-group/:id/alert-subscribes", jwtAuth(), user(), perm("/alert-subscribes/put"), bgrw(), alertSubscribePut)
		pages.DELETE("/busi-group/:id/alert-subscribes", jwtAuth(), user(), perm("/alert-subscribes/del"), bgrw(), alertSubscribeDel)

		if config.C.AnonymousAccess.AlertDetail {
			pages.GET("/alert-cur-event/:eid", alertCurEventGet)
			pages.GET("/alert-his-event/:eid", alertHisEventGet)
		} else {
			pages.GET("/alert-cur-event/:eid", jwtAuth(), alertCurEventGet)
			pages.GET("/alert-his-event/:eid", jwtAuth(), alertHisEventGet)
		}

		// card logic
		pages.GET("/alert-cur-events/list", jwtAuth(), alertCurEventsList)
		pages.GET("/alert-cur-events/card", jwtAuth(), alertCurEventsCard)
		pages.POST("/alert-cur-events/card/details", jwtAuth(), alertCurEventsCardDetails)
		pages.GET("/alert-his-events/list", jwtAuth(), alertHisEventsList)
		pages.DELETE("/alert-cur-events", jwtAuth(), user(), perm("/alert-cur-events/del"), alertCurEventDel)

		pages.GET("/alert-aggr-views", jwtAuth(), alertAggrViewGets)
		pages.DELETE("/alert-aggr-views", jwtAuth(), alertAggrViewDel)
		pages.POST("/alert-aggr-views", jwtAuth(), alertAggrViewAdd)
		pages.PUT("/alert-aggr-views", jwtAuth(), alertAggrViewPut)

		pages.GET("/busi-group/:id/task-tpls", jwtAuth(), user(), perm("/job-tpls"), bgro(), taskTplGets)
		pages.POST("/busi-group/:id/task-tpls", jwtAuth(), user(), perm("/job-tpls/add"), bgrw(), taskTplAdd)
		pages.DELETE("/busi-group/:id/task-tpl/:tid", jwtAuth(), user(), perm("/job-tpls/del"), bgrw(), taskTplDel)
		pages.POST("/busi-group/:id/task-tpls/tags", jwtAuth(), user(), perm("/job-tpls/put"), bgrw(), taskTplBindTags)
		pages.DELETE("/busi-group/:id/task-tpls/tags", jwtAuth(), user(), perm("/job-tpls/put"), bgrw(), taskTplUnbindTags)
		pages.GET("/busi-group/:id/task-tpl/:tid", jwtAuth(), user(), perm("/job-tpls"), bgro(), taskTplGet)
		pages.PUT("/busi-group/:id/task-tpl/:tid", jwtAuth(), user(), perm("/job-tpls/put"), bgrw(), taskTplPut)

		pages.GET("/busi-group/:id/tasks", jwtAuth(), user(), perm("/job-tasks"), bgro(), taskGets)
		pages.POST("/busi-group/:id/tasks", jwtAuth(), user(), perm("/job-tasks/add"), bgrw(), taskAdd)
		pages.GET("/busi-group/:id/task/*url", jwtAuth(), user(), perm("/job-tasks"), taskProxy)
		pages.PUT("/busi-group/:id/task/*url", jwtAuth(), user(), perm("/job-tasks/put"), bgrw(), taskProxy)
	}

	service := r.Group("/v1/n9e")
	if len(config.C.BasicAuth) > 0 {
		service.Use(gin.BasicAuth(config.C.BasicAuth))
	}
	{
		service.Any("/prometheus/*url", prometheusProxy)
		service.POST("/users", userAddPost)

		service.GET("/targets", targetGets)
		service.DELETE("/targets", targetDel)
		service.GET("/targets/tags", targetGetTags)
		service.POST("/targets/tags", targetBindTags)
		service.DELETE("/targets/tags", targetUnbindTags)
		service.PUT("/targets/note", targetUpdateNote)
		service.PUT("/targets/bgid", targetUpdateBgid)

		service.GET("/alert-rules", alertRuleGets)
	}
}
