package provider

import (
	"github.com/ccfos/nightingale/v6/models"
)

func init() {
	// 独立媒介 Provider
	DefaultRegistry.Register(&DingtalkProvider{})
	DefaultRegistry.Register(&WecomProvider{})
	DefaultRegistry.Register(&FeishuCardProvider{})
	DefaultRegistry.Register(&LarkCardProvider{})
	DefaultRegistry.Register(&TencentSmsProvider{})
	DefaultRegistry.Register(&TencentVoiceProvider{})
	DefaultRegistry.Register(&AliyunSmsProvider{})
	DefaultRegistry.Register(&AliyunVoiceProvider{})
	DefaultRegistry.Register(&TelegramProvider{})
	DefaultRegistry.Register(&SlackBotProvider{})
	DefaultRegistry.Register(&SlackWebhookProvider{})
	DefaultRegistry.Register(&MattermostWebhookProvider{})
	DefaultRegistry.Register(&MattermostBotProvider{})
	DefaultRegistry.Register(&DiscordProvider{})
	DefaultRegistry.Register(&JsmProvider{})
	DefaultRegistry.Register(&JiraProvider{})
	DefaultRegistry.Register(&PagerDutyProvider{})
	DefaultRegistry.Register(&ScriptProvider{})
	DefaultRegistry.Register(&EmailProvider{})
	DefaultRegistry.Register(&FlashDutyProvider{})

	// 通用 HTTP (高级用户)
	DefaultRegistry.Register(&GenericHTTPProvider{})

	// 供 models.NotifyChannelsGet 做 ident 映射时使用：未知 ident 按 requestType 映射为 http/script
	models.KnownProviderIdents = knownProviderIdents
}

func knownProviderIdents() map[string]struct{} {
	m := make(map[string]struct{})
	for _, p := range DefaultRegistry.All() {
		m[p.Ident()] = struct{}{}
	}
	m["http"] = struct{}{} // 映射目标，视为已知避免重复映射
	return m
}
