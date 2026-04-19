package provider

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
)

// validateSimpleHTTPConfig 校验 HTTP webhook 类媒介的基础配置：
// POST / application/json / 非空 URL / 非空 Body，以及可选的必填 Parameters。
// 供 simpleHTTPProvider 以及有额外 Notify 逻辑的 Provider (dingtalk/wecom/feishucard/larkcard) 复用。
func validateSimpleHTTPConfig(ident string, requireParams []string, c *models.NotifyChannelConfig) error {
	if err := c.ValidateHTTPRequestConfig(); err != nil {
		return err
	}
	h := c.RequestConfig.HTTPRequestConfig
	if h.Method != "POST" {
		return fmt.Errorf("%s provider requires POST method", ident)
	}
	if h.Headers == nil || h.Headers["Content-Type"] != "application/json" {
		return fmt.Errorf("%s provider requires Content-Type: application/json header", ident)
	}
	if h.URL == "" {
		return fmt.Errorf("%s provider requires URL", ident)
	}
	if h.Request.Body == "" {
		return fmt.Errorf("%s provider requires request body", ident)
	}
	for _, k := range requireParams {
		if h.Request.Parameters == nil || h.Request.Parameters[k] == "" {
			return fmt.Errorf("%s provider requires %s parameter", ident, k)
		}
	}
	return nil
}

// simpleHTTPProvider 覆盖纯模板驱动的 HTTP webhook 媒介 (telegram/feishu/lark/discord/
// slackbot/slackwebhook/mattermostbot/mattermostwebhook/jira/jsm_alert)：
// Check 走统一四段校验，Notify 直接透传到 SendHTTPRequest。
// 需要额外必填参数校验的媒介 (dingtalk/wecom) 不走这个类型，直接调用 validateSimpleHTTPConfig。
type simpleHTTPProvider struct {
	ident string
}

func (p *simpleHTTPProvider) Ident() string { return p.ident }

func (p *simpleHTTPProvider) Check(c *models.NotifyChannelConfig) error {
	return validateSimpleHTTPConfig(p.ident, nil, c)
}

func (p *simpleHTTPProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	h := req.Config.RequestConfig.HTTPRequestConfig
	resp, err := SendHTTPRequest(h, req.Events, req.TplContent,
		req.CustomParams, req.Sendtos, req.HttpClient)
	return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: resp, Err: err}
}
