package tools

import (
	"context"
	"fmt"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// 通知媒介(NotifyChannelConfig)与消息模板(MessageTemplate)的只读发现工具。
// 创建通知规则时，notify_configs[].channel_id / template_id 必须是真实 ID——这两个
// 工具让模型先列出可选项，而不是凭空猜 ID 或回退到 http_fetch 打自家 API。
//
// 不做 checkPerm：二者对标 FE 填下拉用的列表端点(GET /notify-channels、/message-templates)，
// 那两个端点本身就无 perm 守卫；返回值也不含 request_config 等密钥。若在此要求渠道/模板的
// 管理权限，有 /notification-rules/add 但无管理权限的用户会在建规则中途被挡住、闭环失效。

type notifyChannelResult struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Ident       string `json:"ident"`
	Enable      bool   `json:"enable"`
	RequestType string `json:"request_type,omitempty"`
}

type messageTemplateResult struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Ident              string `json:"ident,omitempty"`
	NotifyChannelIdent string `json:"notify_channel_ident,omitempty"`
}

func init() {
	register(defs.ListNotifyChannels, listNotifyChannels)
	register(defs.ListMessageTemplates, listMessageTemplates)
}

func listNotifyChannels(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	// 默认只列启用的渠道：通知规则引用一个禁用渠道不会发出通知，列出来反而误导。
	// include_disabled=true 时把禁用渠道也列出来（排查"渠道被禁用"时用）。
	enabled := 1
	if getArgBool(args, "include_disabled") {
		enabled = -1
	}

	channels, err := models.NotifyChannelGets(deps.DBCtx, 0, "", "", enabled)
	if err != nil {
		return "", fmt.Errorf("failed to query notify channels: %v", err)
	}

	query := getArgString(args, "query")
	results := make([]notifyChannelResult, 0)
	for _, ch := range channels {
		if query != "" && !containsIgnoreCase(ch.Name, query) && !containsIgnoreCase(ch.Ident, query) {
			continue
		}
		results = append(results, notifyChannelResult{
			ID:          ch.ID,
			Name:        ch.Name,
			Ident:       ch.Ident,
			Enable:      ch.Enable,
			RequestType: ch.RequestType,
		})
	}

	logger.Debugf("list_notify_channels: user_id=%d, found %d channels", user.Id, len(results))
	return marshalList(len(results), results), nil
}

func listMessageTemplates(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}

	// 模板按通知媒介的 ident 归属（MessageTemplate.NotifyChannelIdent，不是模板自身的 ident）。
	// 传 notify_channel_ident 就只列该媒介下的模板——配 notify_config 时先选 channel 再选其模板。
	// 必须走 MessageTemplatesGetBy（按 notify_channel_ident 过滤），FE 列表端点也用它；
	// MessageTemplateGets 过滤的是模板自身 ident 列，对自定义模板会漏掉。
	channelIdent := getArgString(args, "notify_channel_ident")

	var idents []string
	if channelIdent != "" {
		idents = []string{channelIdent}
	}
	tpls, err := models.MessageTemplatesGetBy(deps.DBCtx, idents)
	if err != nil {
		return "", fmt.Errorf("failed to query message templates: %v", err)
	}

	query := getArgString(args, "query")
	results := make([]messageTemplateResult, 0)
	for _, t := range tpls {
		if query != "" && !containsIgnoreCase(t.Name, query) {
			continue
		}
		results = append(results, messageTemplateResult{
			ID:                 t.ID,
			Name:               t.Name,
			Ident:              t.Ident,
			NotifyChannelIdent: t.NotifyChannelIdent,
		})
	}

	logger.Debugf("list_message_templates: user_id=%d, channel_ident=%s, found %d templates", user.Id, channelIdent, len(results))
	return marshalList(len(results), results), nil
}
