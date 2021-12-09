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

var InternalServerError = "InternalServerError"

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

		pages.GET("/users", jwtAuth(), user(), userGets)
		pages.POST("/users", jwtAuth(), admin(), userAddPost)
		pages.GET("/user/:id/profile", jwtAuth(), userProfileGet)
		pages.PUT("/user/:id/profile", jwtAuth(), admin(), userProfilePut)
		pages.PUT("/user/:id/password", jwtAuth(), admin(), userPasswordPut)
		pages.DELETE("/user/:id", jwtAuth(), admin(), userDel)

		pages.GET("/user-groups", jwtAuth(), user(), userGroupGets)
		pages.POST("/user-groups", jwtAuth(), user(), userGroupAdd)
		pages.GET("/user-group/:id", jwtAuth(), user(), userGroupGet)
		pages.PUT("/user-group/:id", jwtAuth(), user(), userGroupWrite(), userGroupPut)
		pages.DELETE("/user-group/:id", jwtAuth(), user(), userGroupWrite(), userGroupDel)
		pages.POST("/user-group/:id/members", jwtAuth(), user(), userGroupWrite(), userGroupMemberAdd)
		pages.DELETE("/user-group/:id/members", jwtAuth(), user(), userGroupWrite(), userGroupMemberDel)
		pages.GET("/user-group/:id/perm/:perm", jwtAuth(), user(), checkBusiGroupPerm)

		pages.POST("/busi-groups", jwtAuth(), user(), busiGroupAdd)
		pages.GET("/busi-groups", jwtAuth(), user(), busiGroupGets)
		pages.GET("/busi-groups/alertings", jwtAuth(), busiGroupAlertingsGets)
		pages.GET("/busi-group/:id", jwtAuth(), user(), bgro(), busiGroupGet)
		pages.PUT("/busi-group/:id", jwtAuth(), user(), bgrw(), busiGroupPut)
		pages.POST("/busi-group/:id/members", jwtAuth(), user(), bgrw(), busiGroupMemberAdd)
		pages.DELETE("/busi-group/:id/members", jwtAuth(), user(), bgrw(), busiGroupMemberDel)
		pages.DELETE("/busi-group/:id", jwtAuth(), user(), bgrw(), busiGroupDel)

		pages.GET("/targets", jwtAuth(), user(), targetGets)
		pages.DELETE("/targets", jwtAuth(), user(), targetDel)
		pages.GET("/targets/tags", jwtAuth(), user(), targetGetTags)
		pages.POST("/targets/tags", jwtAuth(), user(), targetBindTags)
		pages.DELETE("/targets/tags", jwtAuth(), user(), targetUnbindTags)
		pages.PUT("/targets/note", jwtAuth(), user(), targetUpdateNote)
		pages.PUT("/targets/bgid", jwtAuth(), user(), targetUpdateBgid)

		pages.GET("/busi-group/:id/dashboards", jwtAuth(), user(), bgro(), dashboardGets)
		pages.POST("/busi-group/:id/dashboards", jwtAuth(), user(), bgrw(), dashboardAdd)
		pages.POST("/busi-group/:id/dashboards/export", jwtAuth(), user(), bgro(), dashboardExport)
		pages.POST("/busi-group/:id/dashboards/import", jwtAuth(), user(), bgrw(), dashboardImport)
		pages.POST("/busi-group/:id/dashboard/:did/clone", jwtAuth(), user(), bgrw(), dashboardClone)
		pages.GET("/busi-group/:id/dashboard/:did", jwtAuth(), user(), bgro(), dashboardGet)
		pages.PUT("/busi-group/:id/dashboard/:did", jwtAuth(), user(), bgrw(), dashboardPut)
		pages.DELETE("/busi-group/:id/dashboard/:did", jwtAuth(), user(), bgrw(), dashboardDel)

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

		pages.GET("/busi-group/:id/alert-rules", jwtAuth(), user(), alertRuleGets)
		pages.POST("/busi-group/:id/alert-rules", jwtAuth(), user(), bgrw(), alertRuleAdd)
		pages.DELETE("/busi-group/:id/alert-rules", jwtAuth(), user(), bgrw(), alertRuleDel)
		pages.PUT("/busi-group/:id/alert-rules/fields", jwtAuth(), user(), bgrw(), alertRulePutFields)
		pages.PUT("/busi-group/:id/alert-rule/:arid", jwtAuth(), user(), bgrw(), alertRulePut)
		pages.GET("/alert-rule/:arid", jwtAuth(), user(), alertRuleGet)

		pages.GET("/busi-group/:id/alert-mutes", jwtAuth(), user(), bgro(), alertMuteGets)
		pages.POST("/busi-group/:id/alert-mutes", jwtAuth(), user(), bgrw(), alertMuteAdd)
		pages.DELETE("/busi-group/:id/alert-mutes", jwtAuth(), user(), bgrw(), alertMuteDel)

		pages.GET("/busi-group/:id/alert-subscribes", jwtAuth(), user(), bgro(), alertSubscribeGets)
		pages.GET("/alert-subscribe/:sid", jwtAuth(), user(), alertSubscribeGet)
		pages.POST("/busi-group/:id/alert-subscribes", jwtAuth(), user(), bgrw(), alertSubscribeAdd)
		pages.PUT("/busi-group/:id/alert-subscribes", jwtAuth(), user(), bgrw(), alertSubscribePut)
		pages.DELETE("/busi-group/:id/alert-subscribes", jwtAuth(), user(), bgrw(), alertSubscribeDel)

		// pages.GET("/busi-group/:id/collect-rules", jwtAuth(), user(), bgro(), collectRuleGets)
		// pages.POST("/busi-group/:id/collect-rules", jwtAuth(), user(), bgrw(), collectRuleAdd)
		// pages.DELETE("/busi-group/:id/collect-rules", jwtAuth(), user(), bgrw(), collectRuleDel)
		// pages.GET("/busi-group/:id/collect-rule/:crid", jwtAuth(), user(), bgro(), collectRuleGet)
		// pages.PUT("/busi-group/:id/collect-rule/:crid", jwtAuth(), user(), bgrw(), collectRulePut)

		pages.GET("/busi-group/:id/alert-his-events", jwtAuth(), user(), bgro(), alertHisEventGets)
		pages.GET("/busi-group/:id/alert-cur-events", jwtAuth(), user(), bgro(), alertCurEventGets)
		pages.DELETE("/busi-group/:id/alert-cur-events", jwtAuth(), user(), bgrw(), alertCurEventDel)

		if config.C.AnonymousAccess.AlertDetail {
			pages.GET("/alert-cur-event/:eid", alertCurEventGet)
			pages.GET("/alert-his-event/:eid", alertHisEventGet)
		} else {
			pages.GET("/alert-cur-event/:eid", jwtAuth(), alertCurEventGet)
			pages.GET("/alert-his-event/:eid", jwtAuth(), alertHisEventGet)
		}

		pages.GET("/busi-group/:id/task-tpls", jwtAuth(), user(), bgro(), taskTplGets)
		pages.POST("/busi-group/:id/task-tpls", jwtAuth(), user(), bgrw(), taskTplAdd)
		pages.DELETE("/busi-group/:id/task-tpl/:tid", jwtAuth(), user(), bgrw(), taskTplDel)
		pages.POST("/busi-group/:id/task-tpls/tags", jwtAuth(), user(), bgrw(), taskTplBindTags)
		pages.DELETE("/busi-group/:id/task-tpls/tags", jwtAuth(), user(), bgrw(), taskTplUnbindTags)
		pages.GET("/busi-group/:id/task-tpl/:tid", jwtAuth(), user(), bgro(), taskTplGet)
		pages.PUT("/busi-group/:id/task-tpl/:tid", jwtAuth(), user(), bgrw(), taskTplPut)

		pages.GET("/busi-group/:id/tasks", jwtAuth(), user(), bgro(), taskGets)
		pages.POST("/busi-group/:id/tasks", jwtAuth(), user(), bgrw(), taskAdd)
		pages.GET("/busi-group/:id/task/*url", jwtAuth(), user(), bgro(), taskProxy)
		pages.PUT("/busi-group/:id/task/*url", jwtAuth(), user(), bgrw(), taskProxy)
	}

	service := r.Group("/v1/n9e", gin.BasicAuth(config.C.BasicAuth))
	{
		service.Any("/prometheus/*url", prometheusProxy)
		service.POST("/users", userAddPost)
	}
}
