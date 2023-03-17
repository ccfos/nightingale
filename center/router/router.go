package router

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/center/cconf"
	"github.com/ccfos/nightingale/v6/center/cstats"
	"github.com/ccfos/nightingale/v6/center/metas"
	"github.com/ccfos/nightingale/v6/center/sso"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/toolkits/pkg/ginx"

	"github.com/gin-gonic/gin"
)

type Router struct {
	HTTP              httpx.Config
	Center            cconf.Center
	Operations        cconf.Operation
	DatasourceCache   *memsto.DatasourceCacheType
	NotifyConfigCache *memsto.NotifyConfigCacheType
	PromClients       *prom.PromClientMap
	Redis             storage.Redis
	MetaSet           *metas.Set
	Sso               *sso.SsoClient
	Ctx               *ctx.Context
}

func New(httpConfig httpx.Config, center cconf.Center, operations cconf.Operation, ds *memsto.DatasourceCacheType, ncc *memsto.NotifyConfigCacheType,
	pc *prom.PromClientMap, redis storage.Redis, sso *sso.SsoClient, ctx *ctx.Context, metaSet *metas.Set) *Router {
	return &Router{
		HTTP:              httpConfig,
		Center:            center,
		Operations:        operations,
		DatasourceCache:   ds,
		NotifyConfigCache: ncc,
		PromClients:       pc,
		Redis:             redis,
		Sso:               sso,
		Ctx:               ctx,
		MetaSet:           metaSet,
	}
}

func stat() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		code := fmt.Sprintf("%d", c.Writer.Status())
		method := c.Request.Method
		labels := []string{cstats.Service, code, c.FullPath(), method}

		cstats.RequestCounter.WithLabelValues(labels...).Inc()
		cstats.RequestDuration.WithLabelValues(labels...).Observe(float64(time.Since(start).Seconds()))
	}
}

func languageDetector(i18NHeaderKey string) gin.HandlerFunc {
	headerKey := i18NHeaderKey
	return func(c *gin.Context) {
		if headerKey != "" {
			lang := c.GetHeader(headerKey)
			if lang != "" {
				if strings.HasPrefix(lang, "zh") {
					c.Request.Header.Set("X-Language", "zh")
				} else if strings.HasPrefix(lang, "en") {
					c.Request.Header.Set("X-Language", "en")
				} else {
					c.Request.Header.Set("X-Language", lang)
				}
			} else {
				c.Request.Header.Set("X-Language", "en")
			}
		}
		c.Next()
	}
}

func (rt *Router) configNoRoute(r *gin.Engine) {
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

func (rt *Router) Config(r *gin.Engine) {
	r.Use(stat())
	r.Use(languageDetector(rt.Center.I18NHeaderKey))
	r.Use(aop.Recovery())

	pagesPrefix := "/api/n9e"
	pages := r.Group(pagesPrefix)
	{

		if rt.Center.AnonymousAccess.PromQuerier {
			pages.Any("/proxy/:id/*url", rt.dsProxy)
			pages.POST("/query-range-batch", rt.promBatchQueryRange)
		} else {
			pages.Any("/proxy/:id/*url", rt.auth(), rt.dsProxy)
			pages.POST("/query-range-batch", rt.auth(), rt.promBatchQueryRange)
		}

		pages.POST("/auth/login", rt.jwtMock(), rt.loginPost)
		pages.POST("/auth/logout", rt.jwtMock(), rt.logoutPost)
		pages.POST("/auth/refresh", rt.jwtMock(), rt.refreshPost)

		pages.GET("/auth/sso-config", rt.ssoConfigNameGet)
		pages.GET("/auth/redirect", rt.loginRedirect)
		pages.GET("/auth/redirect/cas", rt.loginRedirectCas)
		pages.GET("/auth/redirect/oauth", rt.loginRedirectOAuth)
		pages.GET("/auth/callback", rt.loginCallback)
		pages.GET("/auth/callback/cas", rt.loginCallbackCas)
		pages.GET("/auth/callback/oauth", rt.loginCallbackOAuth)

		pages.GET("/metrics/desc", rt.metricsDescGetFile)
		pages.POST("/metrics/desc", rt.metricsDescGetMap)

		pages.GET("/notify-channels", rt.notifyChannelsGets)
		pages.GET("/contact-keys", rt.contactKeysGets)

		pages.GET("/self/perms", rt.auth(), rt.user(), rt.permsGets)
		pages.GET("/self/profile", rt.auth(), rt.user(), rt.selfProfileGet)
		pages.PUT("/self/profile", rt.auth(), rt.user(), rt.selfProfilePut)
		pages.PUT("/self/password", rt.auth(), rt.user(), rt.selfPasswordPut)

		pages.GET("/users", rt.auth(), rt.user(), rt.perm("/users"), rt.userGets)
		pages.POST("/users", rt.auth(), rt.admin(), rt.userAddPost)
		pages.GET("/user/:id/profile", rt.auth(), rt.userProfileGet)
		pages.PUT("/user/:id/profile", rt.auth(), rt.admin(), rt.userProfilePut)
		pages.PUT("/user/:id/password", rt.auth(), rt.admin(), rt.userPasswordPut)
		pages.DELETE("/user/:id", rt.auth(), rt.admin(), rt.userDel)

		pages.GET("/metric-views", rt.auth(), rt.metricViewGets)
		pages.DELETE("/metric-views", rt.auth(), rt.user(), rt.metricViewDel)
		pages.POST("/metric-views", rt.auth(), rt.user(), rt.metricViewAdd)
		pages.PUT("/metric-views", rt.auth(), rt.user(), rt.metricViewPut)

		pages.GET("/user-groups", rt.auth(), rt.user(), rt.userGroupGets)
		pages.POST("/user-groups", rt.auth(), rt.user(), rt.perm("/user-groups/add"), rt.userGroupAdd)
		pages.GET("/user-group/:id", rt.auth(), rt.user(), rt.userGroupGet)
		pages.PUT("/user-group/:id", rt.auth(), rt.user(), rt.perm("/user-groups/put"), rt.userGroupWrite(), rt.userGroupPut)
		pages.DELETE("/user-group/:id", rt.auth(), rt.user(), rt.perm("/user-groups/del"), rt.userGroupWrite(), rt.userGroupDel)
		pages.POST("/user-group/:id/members", rt.auth(), rt.user(), rt.perm("/user-groups/put"), rt.userGroupWrite(), rt.userGroupMemberAdd)
		pages.DELETE("/user-group/:id/members", rt.auth(), rt.user(), rt.perm("/user-groups/put"), rt.userGroupWrite(), rt.userGroupMemberDel)

		pages.GET("/busi-groups", rt.auth(), rt.user(), rt.busiGroupGets)
		pages.POST("/busi-groups", rt.auth(), rt.user(), rt.perm("/busi-groups/add"), rt.busiGroupAdd)
		pages.GET("/busi-groups/alertings", rt.auth(), rt.busiGroupAlertingsGets)
		pages.GET("/busi-group/:id", rt.auth(), rt.user(), rt.bgro(), rt.busiGroupGet)
		pages.PUT("/busi-group/:id", rt.auth(), rt.user(), rt.perm("/busi-groups/put"), rt.bgrw(), rt.busiGroupPut)
		pages.POST("/busi-group/:id/members", rt.auth(), rt.user(), rt.perm("/busi-groups/put"), rt.bgrw(), rt.busiGroupMemberAdd)
		pages.DELETE("/busi-group/:id/members", rt.auth(), rt.user(), rt.perm("/busi-groups/put"), rt.bgrw(), rt.busiGroupMemberDel)
		pages.DELETE("/busi-group/:id", rt.auth(), rt.user(), rt.perm("/busi-groups/del"), rt.bgrw(), rt.busiGroupDel)
		pages.GET("/busi-group/:id/perm/:perm", rt.auth(), rt.user(), rt.checkBusiGroupPerm)

		pages.GET("/targets", rt.auth(), rt.user(), rt.targetGets)
		pages.POST("/target/list", rt.auth(), rt.user(), rt.targetGetsByHostFilter)
		pages.DELETE("/targets", rt.auth(), rt.user(), rt.perm("/targets/del"), rt.targetDel)
		pages.GET("/targets/tags", rt.auth(), rt.user(), rt.targetGetTags)
		pages.POST("/targets/tags", rt.auth(), rt.user(), rt.perm("/targets/put"), rt.targetBindTagsByFE)
		pages.DELETE("/targets/tags", rt.auth(), rt.user(), rt.perm("/targets/put"), rt.targetUnbindTagsByFE)
		pages.PUT("/targets/note", rt.auth(), rt.user(), rt.perm("/targets/put"), rt.targetUpdateNote)
		pages.PUT("/targets/bgid", rt.auth(), rt.user(), rt.perm("/targets/put"), rt.targetUpdateBgid)

		pages.POST("/builtin-cate-favorite", rt.auth(), rt.user(), rt.builtinCateFavoriteAdd)
		pages.DELETE("/builtin-cate-favorite/:name", rt.auth(), rt.user(), rt.builtinCateFavoriteDel)

		pages.GET("/builtin-boards", rt.builtinBoardGets)
		pages.GET("/builtin-board/:name", rt.builtinBoardGet)
		pages.GET("/dashboards/builtin/list", rt.builtinBoardGets)
		pages.GET("/builtin-boards-cates", rt.auth(), rt.user(), rt.builtinBoardCateGets)
		pages.POST("/builtin-boards-detail", rt.auth(), rt.user(), rt.builtinBoardDetailGets)
		pages.GET("/integrations/icon/:cate/:name", func(c *gin.Context) {
			cate := ginx.UrlParamStr(c, "cate")
			fp := "integrations/" + cate + "/icon/" + ginx.UrlParamStr(c, "name")
			c.File(path.Join(fp))
		})

		pages.GET("/busi-group/:id/boards", rt.auth(), rt.user(), rt.perm("/dashboards"), rt.bgro(), rt.boardGets)
		pages.POST("/busi-group/:id/boards", rt.auth(), rt.user(), rt.perm("/dashboards/add"), rt.bgrw(), rt.boardAdd)
		pages.POST("/busi-group/:id/board/:bid/clone", rt.auth(), rt.user(), rt.perm("/dashboards/add"), rt.bgrw(), rt.boardClone)

		pages.GET("/board/:bid", rt.boardGet)
		pages.GET("/board/:bid/pure", rt.boardPureGet)
		pages.PUT("/board/:bid", rt.auth(), rt.user(), rt.perm("/dashboards/put"), rt.boardPut)
		pages.PUT("/board/:bid/configs", rt.auth(), rt.user(), rt.perm("/dashboards/put"), rt.boardPutConfigs)
		pages.PUT("/board/:bid/public", rt.auth(), rt.user(), rt.perm("/dashboards/put"), rt.boardPutPublic)
		pages.DELETE("/boards", rt.auth(), rt.user(), rt.perm("/dashboards/del"), rt.boardDel)

		pages.GET("/share-charts", rt.chartShareGets)
		pages.POST("/share-charts", rt.auth(), rt.chartShareAdd)

		pages.GET("/alert-rules/builtin/list", rt.auth(), rt.user(), rt.alertRuleBuiltinList)
		pages.GET("/alert-rules/builtin/alerts-cates", rt.auth(), rt.user(), rt.builtinAlertCateGets)

		pages.GET("/busi-group/:id/alert-rules", rt.auth(), rt.user(), rt.perm("/alert-rules"), rt.alertRuleGets)
		pages.POST("/busi-group/:id/alert-rules", rt.auth(), rt.user(), rt.perm("/alert-rules/add"), rt.bgrw(), rt.alertRuleAddByFE)
		pages.POST("/busi-group/:id/alert-rules/import", rt.auth(), rt.user(), rt.perm("/alert-rules/add"), rt.bgrw(), rt.alertRuleAddByImport)
		pages.DELETE("/busi-group/:id/alert-rules", rt.auth(), rt.user(), rt.perm("/alert-rules/del"), rt.bgrw(), rt.alertRuleDel)
		pages.PUT("/busi-group/:id/alert-rules/fields", rt.auth(), rt.user(), rt.perm("/alert-rules/put"), rt.bgrw(), rt.alertRulePutFields)
		pages.PUT("/busi-group/:id/alert-rule/:arid", rt.auth(), rt.user(), rt.perm("/alert-rules/put"), rt.alertRulePutByFE)
		pages.GET("/alert-rule/:arid", rt.auth(), rt.user(), rt.perm("/alert-rules"), rt.alertRuleGet)

		pages.GET("/busi-group/:id/recording-rules", rt.auth(), rt.user(), rt.perm("/recording-rules"), rt.recordingRuleGets)
		pages.POST("/busi-group/:id/recording-rules", rt.auth(), rt.user(), rt.perm("/recording-rules/add"), rt.bgrw(), rt.recordingRuleAddByFE)
		pages.DELETE("/busi-group/:id/recording-rules", rt.auth(), rt.user(), rt.perm("/recording-rules/del"), rt.bgrw(), rt.recordingRuleDel)
		pages.PUT("/busi-group/:id/recording-rule/:rrid", rt.auth(), rt.user(), rt.perm("/recording-rules/put"), rt.bgrw(), rt.recordingRulePutByFE)
		pages.GET("/recording-rule/:rrid", rt.auth(), rt.user(), rt.perm("/recording-rules"), rt.recordingRuleGet)
		pages.PUT("/busi-group/:id/recording-rules/fields", rt.auth(), rt.user(), rt.perm("/recording-rules/put"), rt.recordingRulePutFields)

		pages.GET("/busi-group/:id/alert-mutes", rt.auth(), rt.user(), rt.perm("/alert-mutes"), rt.bgro(), rt.alertMuteGetsByBG)
		pages.POST("/busi-group/:id/alert-mutes", rt.auth(), rt.user(), rt.perm("/alert-mutes/add"), rt.bgrw(), rt.alertMuteAdd)
		pages.DELETE("/busi-group/:id/alert-mutes", rt.auth(), rt.user(), rt.perm("/alert-mutes/del"), rt.bgrw(), rt.alertMuteDel)
		pages.PUT("/busi-group/:id/alert-mute/:amid", rt.auth(), rt.user(), rt.perm("/alert-mutes/put"), rt.alertMutePutByFE)
		pages.PUT("/busi-group/:id/alert-mutes/fields", rt.auth(), rt.user(), rt.perm("/alert-mutes/put"), rt.bgrw(), rt.alertMutePutFields)

		pages.GET("/busi-group/:id/alert-subscribes", rt.auth(), rt.user(), rt.perm("/alert-subscribes"), rt.bgro(), rt.alertSubscribeGets)
		pages.GET("/alert-subscribe/:sid", rt.auth(), rt.user(), rt.perm("/alert-subscribes"), rt.alertSubscribeGet)
		pages.POST("/busi-group/:id/alert-subscribes", rt.auth(), rt.user(), rt.perm("/alert-subscribes/add"), rt.bgrw(), rt.alertSubscribeAdd)
		pages.PUT("/busi-group/:id/alert-subscribes", rt.auth(), rt.user(), rt.perm("/alert-subscribes/put"), rt.bgrw(), rt.alertSubscribePut)
		pages.DELETE("/busi-group/:id/alert-subscribes", rt.auth(), rt.user(), rt.perm("/alert-subscribes/del"), rt.bgrw(), rt.alertSubscribeDel)

		if rt.Center.AnonymousAccess.AlertDetail {
			pages.GET("/alert-cur-event/:eid", rt.alertCurEventGet)
			pages.GET("/alert-his-event/:eid", rt.alertHisEventGet)
		} else {
			pages.GET("/alert-cur-event/:eid", rt.auth(), rt.alertCurEventGet)
			pages.GET("/alert-his-event/:eid", rt.auth(), rt.alertHisEventGet)
		}

		// card logic
		pages.GET("/alert-cur-events/list", rt.auth(), rt.alertCurEventsList)
		pages.GET("/alert-cur-events/card", rt.auth(), rt.alertCurEventsCard)
		pages.POST("/alert-cur-events/card/details", rt.auth(), rt.alertCurEventsCardDetails)
		pages.GET("/alert-his-events/list", rt.auth(), rt.alertHisEventsList)
		pages.DELETE("/alert-cur-events", rt.auth(), rt.user(), rt.perm("/alert-cur-events/del"), rt.alertCurEventDel)

		pages.GET("/alert-aggr-views", rt.auth(), rt.alertAggrViewGets)
		pages.DELETE("/alert-aggr-views", rt.auth(), rt.user(), rt.alertAggrViewDel)
		pages.POST("/alert-aggr-views", rt.auth(), rt.user(), rt.alertAggrViewAdd)
		pages.PUT("/alert-aggr-views", rt.auth(), rt.user(), rt.alertAggrViewPut)

		pages.GET("/busi-group/:id/task-tpls", rt.auth(), rt.user(), rt.perm("/job-tpls"), rt.bgro(), rt.taskTplGets)
		pages.POST("/busi-group/:id/task-tpls", rt.auth(), rt.user(), rt.perm("/job-tpls/add"), rt.bgrw(), rt.taskTplAdd)
		pages.DELETE("/busi-group/:id/task-tpl/:tid", rt.auth(), rt.user(), rt.perm("/job-tpls/del"), rt.bgrw(), rt.taskTplDel)
		pages.POST("/busi-group/:id/task-tpls/tags", rt.auth(), rt.user(), rt.perm("/job-tpls/put"), rt.bgrw(), rt.taskTplBindTags)
		pages.DELETE("/busi-group/:id/task-tpls/tags", rt.auth(), rt.user(), rt.perm("/job-tpls/put"), rt.bgrw(), rt.taskTplUnbindTags)
		pages.GET("/busi-group/:id/task-tpl/:tid", rt.auth(), rt.user(), rt.perm("/job-tpls"), rt.bgro(), rt.taskTplGet)
		pages.PUT("/busi-group/:id/task-tpl/:tid", rt.auth(), rt.user(), rt.perm("/job-tpls/put"), rt.bgrw(), rt.taskTplPut)

		pages.GET("/busi-group/:id/tasks", rt.auth(), rt.user(), rt.perm("/job-tasks"), rt.bgro(), rt.taskGets)
		pages.POST("/busi-group/:id/tasks", rt.auth(), rt.user(), rt.perm("/job-tasks/add"), rt.bgrw(), rt.taskAdd)
		pages.GET("/busi-group/:id/task/*url", rt.auth(), rt.user(), rt.perm("/job-tasks"), rt.taskProxy)
		pages.PUT("/busi-group/:id/task/*url", rt.auth(), rt.user(), rt.perm("/job-tasks/put"), rt.bgrw(), rt.taskProxy)

		pages.GET("/servers", rt.auth(), rt.admin(), rt.serversGet)
		pages.GET("/server-clusters", rt.auth(), rt.admin(), rt.serverClustersGet)

		pages.POST("/datasource/list", rt.auth(), rt.datasourceList)
		pages.POST("/datasource/plugin/list", rt.auth(), rt.pluginList)
		pages.POST("/datasource/upsert", rt.auth(), rt.admin(), rt.datasourceUpsert)
		pages.POST("/datasource/desc", rt.auth(), rt.admin(), rt.datasourceGet)
		pages.POST("/datasource/status/update", rt.auth(), rt.admin(), rt.datasourceUpdataStatus)
		pages.DELETE("/datasource/", rt.auth(), rt.admin(), rt.datasourceDel)

		pages.GET("/roles", rt.auth(), rt.admin(), rt.roleGets)
		pages.POST("/roles", rt.auth(), rt.admin(), rt.roleAdd)
		pages.PUT("/roles", rt.auth(), rt.admin(), rt.rolePut)
		pages.DELETE("/role/:id", rt.auth(), rt.admin(), rt.roleDel)

		pages.GET("/role/:id/ops", rt.auth(), rt.admin(), rt.operationOfRole)
		pages.PUT("/role/:id/ops", rt.auth(), rt.admin(), rt.roleBindOperation)
		pages.GET("operation", rt.operations)

		pages.GET("/notify-tpls", rt.auth(), rt.admin(), rt.notifyTplGets)
		pages.PUT("/notify-tpl/content", rt.auth(), rt.admin(), rt.notifyTplUpdateContent)
		pages.PUT("/notify-tpl", rt.auth(), rt.admin(), rt.notifyTplUpdate)
		pages.POST("/notify-tpl/preview", rt.auth(), rt.admin(), rt.notifyTplPreview)

		pages.GET("/sso-configs", rt.auth(), rt.admin(), rt.ssoConfigGets)
		pages.PUT("/sso-config", rt.auth(), rt.admin(), rt.ssoConfigUpdate)

		pages.GET("/webhooks", rt.auth(), rt.admin(), rt.webhookGets)
		pages.PUT("/webhooks", rt.auth(), rt.admin(), rt.webhookPuts)

		pages.GET("/notify-script", rt.auth(), rt.admin(), rt.notifyScriptGet)
		pages.PUT("/notify-script", rt.auth(), rt.admin(), rt.notifyScriptPut)

		pages.GET("/notify-channel", rt.auth(), rt.admin(), rt.notifyChannelGets)
		pages.PUT("/notify-channel", rt.auth(), rt.admin(), rt.notifyChannelPuts)

		pages.GET("/notify-contact", rt.auth(), rt.admin(), rt.notifyContactGets)
		pages.PUT("/notify-contact", rt.auth(), rt.admin(), rt.notifyContactPuts)

		pages.GET("/notify-config", rt.auth(), rt.admin(), rt.notifyConfigGet)
		pages.PUT("/notify-config", rt.auth(), rt.admin(), rt.notifyConfigPut)
	}

	if rt.HTTP.Service.Enable {
		service := r.Group("/v1/n9e")
		if len(rt.HTTP.Service.BasicAuth) > 0 {
			service.Use(gin.BasicAuth(rt.HTTP.Service.BasicAuth))
		}
		{
			service.Any("/prometheus/*url", rt.dsProxy)
			service.POST("/users", rt.userAddPost)
			service.GET("/users", rt.userFindAll)

			service.GET("/targets", rt.targetGets)
			service.GET("/targets/tags", rt.targetGetTags)
			service.POST("/targets/tags", rt.targetBindTagsByService)
			service.DELETE("/targets/tags", rt.targetUnbindTagsByService)
			service.PUT("/targets/note", rt.targetUpdateNoteByService)

			service.POST("/alert-rules", rt.alertRuleAddByService)
			service.DELETE("/alert-rules", rt.alertRuleDelByService)
			service.PUT("/alert-rule/:arid", rt.alertRulePutByService)
			service.GET("/alert-rule/:arid", rt.alertRuleGet)
			service.GET("/alert-rules", rt.alertRulesGetByService)

			service.GET("/alert-mutes", rt.alertMuteGets)
			service.POST("/alert-mutes", rt.alertMuteAddByService)
			service.DELETE("/alert-mutes", rt.alertMuteDel)

			service.GET("/alert-cur-events", rt.alertCurEventsList)
			service.GET("/alert-his-events", rt.alertHisEventsList)
			service.GET("/alert-his-event/:eid", rt.alertHisEventGet)

			service.GET("/config/:id", rt.configGet)
			service.GET("/configs", rt.configsGet)
			service.PUT("/configs", rt.configsPut)
			service.POST("/configs", rt.configsPost)
			service.DELETE("/configs", rt.configsDel)

			service.POST("/conf-prop/encrypt", rt.confPropEncrypt)
			service.POST("/conf-prop/decrypt", rt.confPropDecrypt)
		}
	}

	if rt.HTTP.Heartbeat.Enable {
		heartbeat := r.Group("/v1/n9e")
		{
			if len(rt.HTTP.Heartbeat.BasicAuth) > 0 {
				heartbeat.Use(gin.BasicAuth(rt.HTTP.Heartbeat.BasicAuth))
			}
			heartbeat.POST("/heartbeat", rt.heartbeat)
		}
	}

	rt.configNoRoute(r)
}

func Render(c *gin.Context, data, msg interface{}) {
	if msg == nil {
		if data == nil {
			data = struct{}{}
		}
		c.JSON(http.StatusOK, gin.H{"data": data, "error": ""})
	} else {
		c.JSON(http.StatusOK, gin.H{"error": gin.H{"message": msg}})
	}
}

func Dangerous(c *gin.Context, v interface{}, code ...int) {
	if v == nil {
		return
	}

	switch t := v.(type) {
	case string:
		if t != "" {
			c.JSON(http.StatusOK, gin.H{"error": gin.H{"message": v}})
		}
	case error:
		c.JSON(http.StatusOK, gin.H{"error": gin.H{"message": t.Error()}})
	}
}
