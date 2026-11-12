package sender

import (
	"html/template"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestNewCallBacker_AssignsChannelSpecificTemplate(t *testing.T) {
	tpl := messageTemplate(t)
	for _, tc := range []struct {
		name    string
		domain  string
		channel string
		want    CallBacker
	}{
		{name: "dingtalk", domain: models.DingtalkDomain, channel: models.Dingtalk, want: &DingtalkSender{tpl: tpl}},
		{name: "wecom", domain: models.WecomDomain, channel: models.Wecom, want: &WecomSender{tpl: tpl}},
		{name: "feishu card", domain: models.FeishuCardDomain, channel: models.FeishuCard, want: &FeishuCardSender{tpl: tpl}},
		{name: "lark", domain: models.LarkDomain, channel: models.Lark, want: &LarkSender{tpl: tpl}},
		{name: "lark card", domain: models.LarkCardDomain, channel: models.LarkCard, want: &LarkCardSender{tpl: tpl}},
		{name: "telegram", domain: models.TelegramDomain, channel: models.Telegram, want: &TelegramSender{tpl: tpl}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NewCallBacker(tc.domain, nil, nil, nil, map[string]*template.Template{tc.channel: tpl})

			require.Equal(t, tc.want, got)
		})
	}
}

func TestExtractAtsParams_RejectsMalformedURL(t *testing.T) {
	got := ExtractAtsParams("http://local.test/%zz?ats=alice")

	require.Empty(t, got)
}
