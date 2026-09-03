package router

import (
	"bytes"
	"context"
	"encoding/json"

	// TODO(dingtalkapp): errors 仅在已注释的 dingtalkGroupsGetByNotifyChannel 中使用，钉钉应用上线时一并恢复。
	// "errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/i18n"
)

func (rt *Router) feishuVisibleChatsGet(c *gin.Context) {
	var req struct {
		Query      string `json:"query"`
		PageSize   int    `json:"page_size"`
		UserIDType string `json:"user_id_type"`
		PageToken  string `json:"page_token"`
	}
	ginx.BindJSON(c, &req)

	cid := ginx.UrlParamInt64(c, "id")
	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", cid)
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}
	if nc.RequestConfig == nil || nc.RequestConfig.FeishuAppRequestConfig == nil {
		ginx.Bomb(http.StatusBadRequest, "feishu app request config cannot be nil")
	}

	appCfg := nc.RequestConfig.FeishuAppRequestConfig
	query := req.Query
	if query == "" {
		ginx.Bomb(http.StatusBadRequest, "query is required")
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	userIDType := req.UserIDType
	if userIDType == "" {
		userIDType = "user_id"
	}
	pageToken := req.PageToken

	client, err := buildNotifyHTTPClientForFeishu(nc, appCfg)
	ginx.Dangerous(err)

	token, err := provider.GetFeishuTenantAccessToken(context.Background(), client, appCfg.AppID, appCfg.AppSecret)
	ginx.Dangerous(err)

	data, err := provider.SearchFeishuVisibleChats(context.Background(), client, token, query, pageSize, userIDType, pageToken)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(data, nil)
}

func buildNotifyHTTPClientForFeishu(nc *models.NotifyChannelConfig, appCfg *models.FeishuAppRequestConfig) (*http.Client, error) {
	if nc.RequestConfig != nil && nc.RequestConfig.HTTPRequestConfig != nil {
		return models.GetHTTPClient(nc)
	}

	timeout := appCfg.Timeout
	if timeout <= 0 {
		timeout = 10000
	}
	tmp := &models.NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Timeout: timeout,
				Headers: map[string]string{"Content-Type": "application/json"},
			},
			FeishuAppRequestConfig: appCfg,
		},
	}
	return models.GetHTTPClient(tmp)
}

func (rt *Router) notifyChannelsAdd(c *gin.Context) {
	me := c.MustGet("user").(*models.User)

	var lst []*models.NotifyChannelConfig
	ginx.BindJSON(c, &lst)
	if len(lst) == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	names := make([]string, 0, len(lst))
	for i := range lst {
		ginx.Dangerous(lst[i].Verify())
		names = append(names, lst[i].Name)

		lst[i].CreateBy = me.Username
		lst[i].CreateAt = time.Now().Unix()
		lst[i].UpdateBy = me.Username
		lst[i].UpdateAt = time.Now().Unix()
	}

	lstWithSameName, err := models.NotifyChannelsGet(rt.Ctx, "name IN ?", names)
	ginx.Dangerous(err)
	if len(lstWithSameName) > 0 {
		ginx.Bomb(http.StatusBadRequest, "name already exists")
	}

	ids := make([]int64, 0, len(lst))
	for _, item := range lst {
		err := models.Insert(rt.Ctx, item)
		ginx.Dangerous(err)

		ids = append(ids, item.ID)
	}
	ginx.NewRender(c).Data(ids, nil)
}

func (rt *Router) notifyChannelsDel(c *gin.Context) {
	var f idsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	lst, err := models.NotifyChannelsGet(rt.Ctx, "id in (?)", f.Ids)
	ginx.Dangerous(err)
	notifyRuleIds, err := models.UsedByNotifyRule(rt.Ctx, models.NotiChList(lst))
	ginx.Dangerous(err)
	if len(notifyRuleIds) > 0 {
		ginx.NewRender(c).Message(fmt.Errorf("used by notify rule: %v", notifyRuleIds))
		return
	}

	ginx.NewRender(c).Message(models.DB(rt.Ctx).
		Delete(&models.NotifyChannelConfig{}, "id in (?)", f.Ids).Error)
}

func (rt *Router) notifyChannelPut(c *gin.Context) {
	me := c.MustGet("user").(*models.User)

	var f models.NotifyChannelConfig
	ginx.BindJSON(c, &f)

	lstWithSameName, err := models.NotifyChannelsGet(rt.Ctx, "name = ? and id <> ?", f.Name, f.ID)
	ginx.Dangerous(err)
	if len(lstWithSameName) > 0 {
		ginx.Bomb(http.StatusBadRequest, "name already exists")
	}

	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", ginx.UrlParamInt64(c, "id"))
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}

	f.UpdateBy = me.Username
	ginx.NewRender(c).Message(nc.Update(rt.Ctx, f))
}

func (rt *Router) notifyChannelGet(c *gin.Context) {
	cid := ginx.UrlParamInt64(c, "id")
	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", cid)
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}

	ginx.NewRender(c).Data(nc, nil)
}

func (rt *Router) notifyChannelGetBy(c *gin.Context) {
	ident := ginx.QueryStr(c, "ident")
	nc, err := models.NotifyChannelGet(rt.Ctx, "ident = ?", ident)
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}

	nc.ParamConfig = &models.NotifyParamConfig{}
	nc.RequestConfig = &models.RequestConfig{}

	ginx.NewRender(c).Data(nc, nil)
}

func (rt *Router) notifyChannelsGet(c *gin.Context) {
	lst, err := models.NotifyChannelsGet(rt.Ctx, "", nil)
	if err == nil {
		models.FillUpdateByNicknames(rt.Ctx, lst)
	}
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) notifyChannelsGetForNormalUser(c *gin.Context) {
	lst, err := models.NotifyChannelsGet(rt.Ctx, "")
	ginx.Dangerous(err)

	newLst := make([]*models.NotifyChannelConfig, 0, len(lst))
	for _, c := range lst {
		newLst = append(newLst, &models.NotifyChannelConfig{
			ID:          c.ID,
			Name:        c.Name,
			Ident:       c.Ident,
			Enable:      c.Enable,
			RequestType: c.RequestType,
			ParamConfig: c.ParamConfig,
		})
	}
	ginx.NewRender(c).Data(newLst, nil)
}

func (rt *Router) notifyChannelIdentsGet(c *gin.Context) {
	channels, err := models.NotifyChannelsGet(rt.Ctx, "", nil)
	ginx.Dangerous(err)

	idents := make(map[string]struct{})
	for _, channel := range channels {
		if channel.Ident != "" {
			idents[channel.Ident] = struct{}{}
		}
	}

	lst := make([]string, 0, len(idents))
	for ident := range idents {
		lst = append(lst, ident)
	}

	sort.Strings(lst)
	ginx.NewRender(c).Data(lst, nil)
}

// TODO(dingtalkapp): 钉钉应用本次不上线，dingtalkGroupsGetByNotifyChannel handler 先整段注释；
// 对应路由 POST /dingtalk-group-list/:id 也在 router.go 一起注释。上线时去掉 /* 和 */。
/*
func (rt *Router) dingtalkGroupsGetByNotifyChannel(c *gin.Context) {
	type reqBody struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
	}

	cid := ginx.UrlParamInt64(c, "id")
	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", cid)
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}
	if nc.RequestType != "dingtalkapp" {
		ginx.Bomb(http.StatusBadRequest, "notify channel is not dingtalkapp")
	}
	if nc.RequestConfig == nil || nc.RequestConfig.DingtalkAppRequestConfig == nil {
		ginx.Bomb(http.StatusBadRequest, "dingtalk app request config cannot be nil")
	}
	clientID := nc.RequestConfig.DingtalkAppRequestConfig.AppKey
	if clientID == "" {
		ginx.Bomb(http.StatusBadRequest, "dingtalk app client_id(app_key) cannot be empty")
	}

	req := reqBody{
		Page:     1,
		PageSize: 50,
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		ginx.Bomb(http.StatusBadRequest, "json body invalid: %v", err)
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 50
	}

	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		ginx.Bomb(http.StatusBadRequest, "page must be >= 1")
	}
	if pageSize < 1 || pageSize > 500 {
		ginx.Bomb(http.StatusBadRequest, "page_size must be in [1, 500]")
	}
	offset := (page - 1) * pageSize

	list, total, err := models.DingtalkGroupsGetByClientIDPage(rt.Ctx, clientID, true, offset, pageSize)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, nil)
}
*/

func (rt *Router) flashDutyNotifyChannelsGet(c *gin.Context) {
	cid := ginx.UrlParamInt64(c, "id")
	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", cid)
	ginx.Dangerous(err)
	if nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}

	configs, err := models.ConfigsSelectByCkey(rt.Ctx, "flashduty_app_key")
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, "failed to get flashduty app key")
	}

	jsonData := []byte("{}")
	if len(configs) > 0 {
		me := c.MustGet("user").(*models.User)
		jsonData = []byte(fmt.Sprintf(`{"member_name":"%s","email":"%s","phone":"%s"}`, me.Username, me.Email, me.Phone))
	}

	items, err := getFlashDutyChannels(nc.RequestConfig.FlashDutyRequestConfig.IntegrationUrl, jsonData, time.Duration(nc.RequestConfig.FlashDutyRequestConfig.Timeout)*time.Millisecond)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(items, nil)
}

type flushDutyChannelsResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Data struct {
		Items []FlashDutyChannel `json:"items"`
		Total int                `json:"total"`
	} `json:"data"`
}

type FlashDutyChannel struct {
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Status      string `json:"status"`
}

// getFlashDutyChannels 从FlashDuty API获取频道列表
func getFlashDutyChannels(integrationUrl string, jsonData []byte, timeout time.Duration) ([]FlashDutyChannel, error) {
	// 解析URL，提取baseUrl和参数
	baseUrl, integrationKey, err := parseIntegrationUrl(integrationUrl)
	if err != nil {
		return nil, err
	}

	if integrationKey == "" {
		return nil, fmt.Errorf("integration_key not found in URL")
	}

	// 构建新的API URL，保持原始路径
	url := fmt.Sprintf("%s/channel/list-by-integration?integration_key=%s", baseUrl, integrationKey)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	httpResp, err := (&http.Client{
		Timeout: timeout,
	}).Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	var res flushDutyChannelsResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}

	if res.Error.Message != "" {
		return nil, newMessageError(res.Error.Message)
	}

	return res.Data.Items, nil
}

// parseIntegrationUrl 从URL中提取baseUrl和参数
func parseIntegrationUrl(urlStr string) (baseUrl string, integrationKey string, err error) {
	// 解析URL
	parsedUrl, err := url.Parse(urlStr)
	if err != nil {
		return "", "", err
	}

	host := fmt.Sprintf("%s://%s", parsedUrl.Scheme, parsedUrl.Host)

	// 提取查询参数
	queryParams := parsedUrl.Query()
	integrationKey = queryParams.Get("integration_key")

	return host, integrationKey, nil
}

func (rt *Router) pagerDutyNotifyServicesGet(c *gin.Context) {
	cid := ginx.UrlParamInt64(c, "id")
	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", cid)
	ginx.Dangerous(err)
	if err != nil || nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}

	items, err := getPagerDutyServices(nc.RequestConfig.PagerDutyRequestConfig.ApiKey, time.Duration(nc.RequestConfig.PagerDutyRequestConfig.Timeout)*time.Millisecond)
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, "failed to get pagerduty services: %v", err)
	}
	// 服务: []集成，扁平化为服务-集成
	var flattenedItems []map[string]string
	for _, svc := range items {
		for _, integ := range svc.Integrations {
			flattenedItems = append(flattenedItems, map[string]string{
				"service_id":          svc.ID,
				"service_name":        svc.Name,
				"integration_summary": integ.Summary,
				"integration_id":      integ.ID,
				"integration_url":     integ.Self,
			})
		}
	}

	ginx.NewRender(c).Data(flattenedItems, nil)
}

func (rt *Router) pagerDutyIntegrationKeyGet(c *gin.Context) {
	serviceId := ginx.UrlParamStr(c, "service_id")
	integrationId := ginx.UrlParamStr(c, "integration_id")
	cid := ginx.UrlParamInt64(c, "id")
	nc, err := models.NotifyChannelGet(rt.Ctx, "id = ?", cid)
	ginx.Dangerous(err)
	if err != nil || nc == nil {
		ginx.Bomb(http.StatusNotFound, "notify channel not found")
	}

	integrationUrl := fmt.Sprintf("https://api.pagerduty.com/services/%s/integrations/%s", serviceId, integrationId)
	integrationKey, err := getPagerDutyIntegrationKey(integrationUrl, nc.RequestConfig.PagerDutyRequestConfig.ApiKey, time.Duration(nc.RequestConfig.PagerDutyRequestConfig.Timeout)*time.Millisecond)
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, "failed to get pagerduty integration key: %v", err)
	}

	ginx.NewRender(c).Data(map[string]string{
		"integration_key": integrationKey,
	}, nil)
}

type PagerDutyIntegration struct {
	ID             string `json:"id"`
	IntegrationKey string `json:"integration_key"`
	Self           string `json:"self"` // integration 的 API URL
	Summary        string `json:"summary"`
}

type PagerDutyService struct {
	Name         string                 `json:"name"`
	ID           string                 `json:"id"`
	Integrations []PagerDutyIntegration `json:"integrations"`
}

// getPagerDutyServices 从 PagerDuty API 分页获取所有服务及其集成信息
func getPagerDutyServices(apiKey string, timeout time.Duration) ([]PagerDutyService, error) {
	const limit = 100 // 每页最大数量
	var offset uint   // 分页偏移量
	var allServices []PagerDutyService

	for {
		// 构建带分页参数的 URL
		url := fmt.Sprintf("https://api.pagerduty.com/services?limit=%d&offset=%d", limit, offset)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Token token=%s", apiKey))
		req.Header.Set("Accept", "application/vnd.pagerduty+json;version=2")

		httpResp, err := (&http.Client{Timeout: timeout}).Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		if err != nil {
			return nil, err
		}

		// 定义包含分页信息的响应结构
		var serviceRes struct {
			Services []PagerDutyService `json:"services"`
			More     bool               `json:"more"` // 是否还有更多数据
			Limit    uint               `json:"limit"`
			Offset   uint               `json:"offset"`
		}

		if err := json.Unmarshal(body, &serviceRes); err != nil {
			return nil, err
		}
		allServices = append(allServices, serviceRes.Services...)
		// 判断是否还有更多数据
		if !serviceRes.More || len(serviceRes.Services) < int(limit) {
			break
		}
		offset += limit // 准备请求下一页
	}

	return allServices, nil
}

// getPagerDutyIntegrationKey 通过 integration 的 API URL 获取 integration key
func getPagerDutyIntegrationKey(integrationUrl, apiKey string, timeout time.Duration) (string, error) {
	req, err := http.NewRequest("GET", integrationUrl, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Token token=%s", apiKey))

	httpResp, err := (&http.Client{
		Timeout: timeout,
	}).Do(req)
	if err != nil {
		return "", err
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", err
	}

	var integRes struct {
		Integration struct {
			IntegrationKey string `json:"integration_key"`
		} `json:"integration"`
	}

	if err := json.Unmarshal(body, &integRes); err != nil {
		return "", err
	}

	return integRes.Integration.IntegrationKey, nil
}

// NotifyChannelTestForm 测试一份「可能尚未保存」的媒介配置。
//
// Config 来自请求体而不是库，因此本接口不查 notify_channel 表、也不做 Enable 闸门校验
// （草稿态的 enable 常为 false）。TplContent 是模板**源码**，由本接口负责渲染成正文。
type NotifyChannelTestForm struct {
	Config       models.NotifyChannelConfig `json:"config"`
	NotifyConfig models.NotifyConfig        `json:"notify_config"` // 只用其中的 params/severities，channel_id 与 template_id 被忽略
	TplContent   map[string]string          `json:"tpl_content"`
	EventIDs     []int64                    `json:"event_ids"`
	MockEventForm
}

// notifyChannelTestResult 刻意只回「成不成 + 为什么不成」。
//
// 不回显任何配置字段，是因为 Grafana 的同类接口（CVE-2025-12141）正是靠回显脱敏配置
// 被用来提取第三方凭据。同理不回 sendtos/target：那是 user_ids/user_group_ids 经
// GetNotifyConfigParams 解析出的真实邮箱与手机号，回传等于提供「传 ID 换联系方式」的读放大。
type notifyChannelTestResult struct {
	Success      bool   `json:"success"`
	ErrorMessage string `json:"error_message"`
}

// buildChannelTestMockEvent 构造用于媒介连通性测试的内置模拟事件，不落库。
// 与通知规则测试、工作流试跑共用 newMockEvent 骨架，只在标签上做来源区分。
func buildChannelTestMockEvent(lang string, f MockEventForm) *models.AlertCurEvent {
	return newMockEvent(mockEventSpec{
		RuleName:     i18n.Sprintf(lang, "Media type test mock event"),
		RuleNote:     i18n.Sprintf(lang, "This is a mock event sent by media type test, just to verify that the media type config works"),
		Hash:         "notify-channel-test-mock-event",
		Severity:     f.MockSeverity,
		IsRecovered:  f.MockIsRecovered,
		PromQL:       "cpu_usage_active > 80",
		TriggerValue: "81.5",
		ExtraTags:    []string{"ident=mock-host-01", "source=notify-channel-test"},
	})
}

// buildTestTplContent 把请求体里的模板**源码**渲染成 sendToNotifyChannel 期望的正文
// （直接塞源码进去会把 {{$event.RuleName}} 原样发出去）。
//
// 两处细节决定了这个功能有没有意义，都不能省：
//
//  1. NotifyChannelIdent 必须带上。RenderEvent 按它选渲染分支——email 走 text/template
//     且不转义，slack 有专门的 &lt; 还原，其余走 html/template + JSON 转义。留空会一律落到
//     默认分支：测试一个 smtp 媒介时正文里的换行变成字面量 \n、引号变成 \"，而保存之后走
//     生产链路（模板行带 notify_channel_ident）却是正常的——同一份模板两种结果，正好违背
//     「保存前看到真实投递效果」这个目的。
//
//  2. 走 RenderEventStrict 而不是 RenderEvent。后者把 Parse/Execute 错误当正文返回，
//     只要第三方端点回 2xx 接口就报 success=true——用户看到「测试成功」，群里收到的却是
//     一段 "failed to parse template: ..."。测试接口的全部价值就是如实回答通不通。
//
// 空模板是合法形态、不是参数错误：body 直接透传 $events 的媒介（内置 callback 的默认 body
// 是 {{ jsonMarshal $events }}）根本不引用 $tpl，前端也就推导不出字段名。生产链路上它走的是
// 种子模板 {"content": ""}（models/message_tpl.go 的 MsgTplMap），渲染结果同样是空——这里
// 返回空 map 与之等价，{{$tpl.x}} 会渲染成空串且不报错。
func buildTestTplContent(nc *models.NotifyChannelConfig, tplSrc map[string]string,
	events []*models.AlertCurEvent, siteUrl string) (map[string]interface{}, error) {
	if !NeedMessageTemplate(nc.RequestType) || len(tplSrc) == 0 {
		return make(map[string]interface{}), nil
	}
	tpl := &models.MessageTemplate{Content: tplSrc, NotifyChannelIdent: nc.Ident}
	return tpl.RenderEventStrict(events, siteUrl)
}

func (rt *Router) notifyChannelConfigTest(c *gin.Context) {
	var f NotifyChannelTestForm
	ginx.BindJSON(c, &f)

	nc := &f.Config

	// 内联 script 配置 = 从请求体直取任意代码执行：SendScript 会把 script 写盘、chmod 0777
	// 再 exec，且 path 非空时直接执行服务端任意路径。未保存的配置没有任何审计留痕，
	// 因此这条路径一律拒绝，引导用户先保存（保存动作有 create_by，且同样受权限约束）。
	if nc.RequestType == "script" {
		ginx.Bomb(http.StatusBadRequest, "script media type must be saved before testing")
	}

	// 不走 ncc.Verify() 里的 VerifyByProvider 钩子：那个函数指针只在初始化了告警引擎的
	// 进程里被赋值（alert/alert.go），center 单独部署时为 nil，按类型的必填校验会被静默跳过。
	// 这里直接调 provider 侧的校验，保证任何部署形态下口径一致。
	bombErr(http.StatusBadRequest, nc.Verify())
	bombErr(http.StatusBadRequest, provider.VerifyChannelConfig(nc))

	events := []*models.AlertCurEvent{}
	if f.UseMockEvent {
		events = append(events, buildChannelTestMockEvent(c.GetHeader("X-Language"), f.MockEventForm))
	} else {
		if len(f.EventIDs) == 0 {
			ginx.Bomb(http.StatusBadRequest, "event_ids or use_mock_event required")
		}
		hisEvents, err := models.AlertHisEventGetByIds(rt.Ctx, f.EventIDs)
		ginx.Dangerous(err)
		if len(hisEvents) == 0 {
			ginx.Bomb(http.StatusBadRequest, "event not found")
		}
		for _, he := range hisEvents {
			event := he.ToCur()
			event.SetTagsMap()
			events = append(events, event)
		}
	}

	siteUrl := resolveSiteUrl(rt.Ctx)

	tplContent, rerr := buildTestTplContent(nc, f.TplContent, events, siteUrl)
	if rerr != nil {
		// 模板写错是业务结果，与投递失败同一个出口：200 + success=false，
		// 前端在结果页展示报错原文
		ginx.NewRender(c).Data(notifyChannelTestResult{Success: false, ErrorMessage: rerr.Error()}, nil)
		return
	}

	_, err := sendToNotifyChannel(rt.Ctx, rt.UserCache, rt.UserGroupCache, f.NotifyConfig, nc, events, tplContent, siteUrl)

	res := notifyChannelTestResult{Success: err == nil}
	if err != nil {
		res.ErrorMessage = err.Error()
	}
	// 第二个参数恒为 nil：测试失败是业务结果而非接口错误，让前端拿 200 + success=false，
	// 避免 ginx 的错误通道把第三方报错原文当成系统异常渲染。
	ginx.NewRender(c).Data(res, nil)
}
