package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

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
	// 获取所有通知渠道
	channels, err := models.NotifyChannelsGet(rt.Ctx, "", nil)
	ginx.Dangerous(err)

	// ident 去重
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
		return nil, fmt.Errorf(res.Error.Message)
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
		ginx.Bomb(http.StatusInternalServerError, fmt.Sprintf("failed to get pagerduty services: %v", err))
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
		ginx.Bomb(http.StatusInternalServerError, fmt.Sprintf("failed to get pagerduty integration key: %v", err))
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
