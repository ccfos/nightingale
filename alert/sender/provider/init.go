package provider

func init() {
	// 独立媒介 Provider
	DefaultRegistry.Register(&DingtalkProvider{})
	DefaultRegistry.Register(&WecomProvider{})
	DefaultRegistry.Register(&FeishuProvider{})
	DefaultRegistry.Register(&FeishuCardProvider{})
	DefaultRegistry.Register(&TencentSmsProvider{})
	DefaultRegistry.Register(&TencentVoiceProvider{})

	// 通用 HTTP (高级用户)
	DefaultRegistry.Register(&GenericHTTPProvider{})
}
