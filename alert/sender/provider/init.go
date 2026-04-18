package provider

func init() {
	// 独立媒介 Provider
	DefaultRegistry.Register(&DingtalkProvider{})
	DefaultRegistry.Register(&DingtalkAppProvider{})
	DefaultRegistry.Register(&FeishuProvider{})
	DefaultRegistry.Register(&FeishuAppProvider{})
	DefaultRegistry.Register(&WecomProvider{})
	DefaultRegistry.Register(&WecomAppProvider{})
	DefaultRegistry.Register(&FeishuCardProvider{})
	DefaultRegistry.Register(&LarkProvider{})
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
}
