package provider

import "github.com/ccfos/nightingale/v6/models"

func init() {
	// 自定义逻辑 Provider：各自文件实现 Ident/Check/Notify
	DefaultRegistry.Register(&DingtalkProvider{})
	// TODO(dingtalkapp): 钉钉应用本次不上线，先不注册 Provider；未注册时 VerifyChannelConfig/Resolve 都会直接报错，避免误用。待上线时恢复本行。
	// DefaultRegistry.Register(&DingtalkAppProvider{})
	DefaultRegistry.Register(&FeishuAppProvider{})
	DefaultRegistry.Register(&WecomProvider{})
	DefaultRegistry.Register(&WecomAppProvider{})
	DefaultRegistry.Register(&FeishuCardProvider{})
	DefaultRegistry.Register(&LarkCardProvider{})
	DefaultRegistry.Register(&TencentSmsProvider{})
	DefaultRegistry.Register(&TencentVoiceProvider{})
	DefaultRegistry.Register(&AliyunSmsProvider{})
	DefaultRegistry.Register(&AliyunVoiceProvider{})
	DefaultRegistry.Register(&PagerDutyProvider{})
	DefaultRegistry.Register(&ScriptProvider{})
	DefaultRegistry.Register(&EmailProvider{})
	DefaultRegistry.Register(&FlashDutyProvider{})
	DefaultRegistry.Register(&CallbackProvider{})

	// 原生对接的海外媒介：request_type 与 ident 同名；旧版同名 ident 的 request_type=http 记录
	// 在 Check 阶段被拒，按 request_type 兜底到 callback，发送行为不变
	DefaultRegistry.Register(&JiraProvider{})
	DefaultRegistry.Register(&DiscordProvider{})
	DefaultRegistry.Register(&JSMAlertProvider{})
	DefaultRegistry.Register(&SlackWebhookProvider{})
	DefaultRegistry.Register(&MattermostWebhookProvider{})

	// 纯 HTTP webhook 模板驱动 Provider：只差 ident，统一走 simpleHTTPProvider
	for _, ident := range []string{
		models.Feishu,
		models.Lark,
		models.Telegram,
		models.SlackBot,
		models.MattermostBot,
	} {
		DefaultRegistry.Register(&simpleHTTPProvider{ident: ident})
	}
}
