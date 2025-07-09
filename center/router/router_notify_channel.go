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

type flushDutyChannelsResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Data struct {
		Items []struct {
			ChannelID   int    `json:"channel_id"`
			ChannelName string `json:"channel_name"`
			Status      string `json:"status"`
		} `json:"items"`
		Total int `json:"total"`
	} `json:"data"`
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

	items, err := getFlashDutyChannels(nc.RequestConfig.FlashDutyRequestConfig.IntegrationUrl, jsonData)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(items, nil)
}

// getFlashDutyChannels 从FlashDuty API获取频道列表
func getFlashDutyChannels(integrationUrl string, jsonData []byte) ([]struct {
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Status      string `json:"status"`
}, error) {
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
	httpResp, err := (&http.Client{}).Do(req)
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
