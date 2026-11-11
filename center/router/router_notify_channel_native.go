package router

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/gin-gonic/gin"
)

// 原生对接媒介（Jira 等）的辅助接口：保存前「校验凭证」，以及通知规则里的下拉数据。

const nativeChannelAPITimeout = 60 * time.Second

// NotifyChannelCheckForm 校验一份「可能尚未保存」的媒介配置，形状与测试接口的 config 一致
type NotifyChannelCheckForm struct {
	Config models.NotifyChannelConfig `json:"config"`
}

// notifyChannelConfigCheck 对媒介配置逐项检查凭证与权限，返回 [{name, ok, required, skipped, message}]。
//
// 不走 nc.Verify()：用户可能还没填媒介名称就点了校验，这里只关心凭证本身；
// provider 侧的格式校验（Check）仍然要过，格式都不对就没必要发请求。
func (rt *Router) notifyChannelConfigCheck(c *gin.Context) {
	var f NotifyChannelCheckForm
	ginx.BindJSON(c, &f)
	nc := &f.Config

	bombErr(http.StatusBadRequest, provider.VerifyChannelConfig(nc))
	p, ok := provider.DefaultRegistry.Resolve(nc)
	if !ok {
		ginx.Bomb(http.StatusBadRequest, "unsupported channel")
	}
	checker, ok := p.(provider.CredentialChecker)
	if !ok {
		ginx.Bomb(http.StatusBadRequest, "this media type does not support credential check")
	}
	client, err := models.GetHTTPClient(nc)
	bombErr(http.StatusBadRequest, err)

	ctx, cancel := context.WithTimeout(c.Request.Context(), nativeChannelAPITimeout)
	defer cancel()
	items := checker.CheckCredential(ctx, nc, client)
	ginx.NewRender(c).Data(provider.LocalizeCheckItems(items, func(k string) string { return translate(c, k) }), nil)
}

// jiraChannelAPI 取已保存的 Jira 媒介并构造只读查询客户端
func (rt *Router) jiraChannelAPI(c *gin.Context, ctx context.Context) *provider.JiraAPI {
	id := ginx.UrlParamInt64(c, "id")
	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", id)
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}
	if nc.RequestType != models.RequestTypeJira {
		ginx.Bomb(http.StatusBadRequest, "notify channel is not a jira channel")
	}
	api, err := provider.NewJiraClientForChannel(ctx, nc)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, "%s", rt.localizeErr(c, err))
	}
	return api
}

func (rt *Router) localizeErr(c *gin.Context, err error) string {
	return provider.LocalizeError(err, func(k string) string { return translate(c, k) })
}

// renderNativeResult 统一出口：第三方报错按请求语言翻译提示后返回
func (rt *Router) renderNativeResult(c *gin.Context, data interface{}, err error) {
	if err != nil {
		ginx.NewRender(c).Data(nil, errors.New(rt.localizeErr(c, err)))
		return
	}
	ginx.NewRender(c).Data(data, nil)
}

func (rt *Router) jiraProjectList(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), nativeChannelAPITimeout)
	defer cancel()
	list, err := rt.jiraChannelAPI(c, ctx).Projects(ctx)
	if list == nil {
		list = []provider.JiraProject{}
	}
	rt.renderNativeResult(c, list, err)
}

func (rt *Router) jiraIssueTypeList(c *gin.Context) {
	project := strings.TrimSpace(ginx.QueryStr(c, "project"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), nativeChannelAPITimeout)
	defer cancel()
	list, err := rt.jiraChannelAPI(c, ctx).IssueTypes(ctx, project)
	if list == nil {
		list = []provider.JiraIssueType{}
	}
	rt.renderNativeResult(c, list, err)
}

func (rt *Router) jiraIssueTypeCheck(c *gin.Context) {
	project := strings.TrimSpace(ginx.QueryStr(c, "project"))
	issueType := strings.TrimSpace(ginx.QueryStr(c, "issue_type"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), nativeChannelAPITimeout)
	defer cancel()
	res, err := rt.jiraChannelAPI(c, ctx).IssueTypeCheck(ctx, project, issueType)
	rt.renderNativeResult(c, res, err)
}

func (rt *Router) jiraPriorityList(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), nativeChannelAPITimeout)
	defer cancel()
	list, err := rt.jiraChannelAPI(c, ctx).Priorities(ctx)
	if list == nil {
		list = []provider.JiraPriority{}
	}
	rt.renderNativeResult(c, list, err)
}
