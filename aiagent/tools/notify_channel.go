package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
	// 以下两个字段决定通知规则 notify_configs[].params 的形状，即"选了这个媒介还要让用户补什么信息"：
	// contact_key 非空 → 接收人按 user_ids/user_group_ids 选择，取用户 contact_info 的这个字段；
	// custom_params 非空 → params 里要逐项填这些 key（如钉钉群机器人的 access_token），值由用户提供。
	ContactKey   string                     `json:"contact_key,omitempty"`
	CustomParams []notifyChannelCustomParam `json:"custom_params,omitempty"`
}

type notifyChannelCustomParam struct {
	Key   string `json:"key"`
	CName string `json:"cname,omitempty"`
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
	register(defs.ListNotifyRuleCustomParams, listNotifyRuleCustomParams)
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
		r := notifyChannelResult{
			ID:          ch.ID,
			Name:        ch.Name,
			Ident:       ch.Ident,
			Enable:      ch.Enable,
			RequestType: ch.RequestType,
		}
		if ch.ParamConfig != nil {
			if ch.ParamConfig.UserInfo != nil {
				r.ContactKey = ch.ParamConfig.UserInfo.ContactKey
			}
			for _, p := range ch.ParamConfig.Custom.Params {
				r.CustomParams = append(r.CustomParams, notifyChannelCustomParam{Key: p.Key, CName: p.CName})
			}
		}
		results = append(results, r)
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

// notifyRuleCustomParamsResult 一组已被某些通知规则使用的自定义参数取值。
// used_by_rules 让模型能按规则名匹配"发到和规则 X 同一个群"这类指代。
type notifyRuleCustomParamsResult struct {
	Params      map[string]interface{} `json:"params"`
	UsedByRules []string               `json:"used_by_rules"`
}

// listNotifyRuleCustomParams 是 GET /api/n9e/notify-rule/custom-params 的工具版：
// 收集当前用户可见的通知规则里、指定媒介下已填过的自定义参数值（access_token/key 等），
// 按取值去重分组。用途是参数复用——"发到和规则 X 同一个钉钉群"时不必再让用户翻 token。
// 注意这里会把 token 暴露给模型，与 FE 同名端点的暴露面一致：perm 对齐 /notification-rules，
// 数据级只回用户所属团队能看到的规则（admin 不受限，对齐 list_notify_rules 而非路由的全员交集）。
func listNotifyRuleCustomParams(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermNotificationRules); err != nil {
		return "", err
	}

	channelID := int64(getArgInt(args, "notify_channel_id", 0))
	if channelID <= 0 {
		return "", fmt.Errorf("notify_channel_id is required: get the real channel id via list_notify_channels")
	}

	ch, err := models.NotifyChannelGet(deps.DBCtx, "id=?", channelID)
	if err != nil {
		return "", fmt.Errorf("failed to query notify channel: %v", err)
	}
	if ch == nil {
		return "", fmt.Errorf("notify channel %d not found, use list_notify_channels to get a real id", channelID)
	}

	// 只认媒介 ParamConfig 声明过的 key：params 里可能混着 user_ids 等接收人字段，复用无意义
	keyMap := make(map[string]string)
	if ch.ParamConfig != nil {
		for _, p := range ch.ParamConfig.Custom.Params {
			keyMap[p.Key] = p.CName
		}
	}
	if len(keyMap) == 0 {
		return "", fmt.Errorf("channel %q (id=%d) has no custom params; its notify-rule params are recipients (user_ids/user_group_ids), nothing to reuse here", ch.Name, channelID)
	}

	rules, err := models.NotifyRulesGet(deps.DBCtx, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to query notify rules: %v", err)
	}

	var myGroupIds []int64
	if !user.IsAdmin() {
		myGroupIds, err = getUserGroupIds(deps, user.Id)
		if err != nil {
			return "", err
		}
	}

	order := make([]string, 0)
	grouped := make(map[string]*notifyRuleCustomParamsResult)
	for _, nr := range rules {
		if !user.IsAdmin() && !int64SlicesOverlap(myGroupIds, nr.UserGroupIds) {
			continue
		}
		for _, nc := range nr.NotifyConfigs {
			if nc.ChannelID != channelID {
				continue
			}
			subset := make(map[string]interface{})
			for k, v := range nc.Params {
				if _, ok := keyMap[k]; ok {
					subset[k] = v
				}
			}
			if len(subset) == 0 {
				continue
			}

			fp := paramsFingerprint(subset)
			g, ok := grouped[fp]
			if !ok {
				g = &notifyRuleCustomParamsResult{Params: subset, UsedByRules: []string{}}
				grouped[fp] = g
				order = append(order, fp)
			}
			if len(g.UsedByRules) == 0 || g.UsedByRules[len(g.UsedByRules)-1] != nr.Name {
				g.UsedByRules = append(g.UsedByRules, nr.Name)
			}
		}
	}

	results := make([]*notifyRuleCustomParamsResult, 0, len(order))
	for _, fp := range order {
		results = append(results, grouped[fp])
	}

	logger.Debugf("list_notify_rule_custom_params: user_id=%d, channel_id=%d, found %d param sets", user.Id, channelID, len(results))
	return marshalList(len(results), results), nil
}

// paramsFingerprint 生成参数集合的去重指纹：key 排序后拼接，与 map 遍历顺序解耦；
// value 用 %q 引号化，避免值里含 ";"/"=" 时与相邻键值对产生拼接歧义。
func paramsFingerprint(m map[string]interface{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%q=%q;", k, fmt.Sprint(m[k]))
	}
	return sb.String()
}
