package dispatch

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type round2Callback struct{ calls []sender.CallBackContext }

func (s *round2Callback) CallBack(c sender.CallBackContext) { s.calls = append(s.calls, c) }

func TestSendCallbacks_SelectsProviderAndPassesRecipients(t *testing.T) {
	for _, tc := range []struct {
		name   string
		url    string
		domain string
	}{
		{name: "custom domain", url: "https://example.test/hook", domain: "example.test"},
		{name: "fallback", url: "https://unknown.test/hook", domain: models.DefaultDomain},
		{name: "Feishu card", url: "https://" + models.FeishuDomain + "/hook?card=1", domain: models.FeishuCardDomain},
		{name: "Lark card", url: "https://" + models.LarkDomain + "/hook?card=1", domain: models.LarkCardDomain},
		{name: "Ibex", url: "${ibex}/hook", domain: models.IbexDomain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &round2Callback{}
			users, _ := recipientCaches()
			d := &Dispatch{userCache: users, CallBacks: map[string]sender.CallBacker{tc.domain: stub}}
			target := NewNotifyTarget()
			target.callbacks[tc.url] = struct{}{}
			target.userMap[1] = NewNotifyChannels([]string{"email"})
			rule := &models.AlertRule{Id: 5}
			event := &models.AlertCurEvent{Hash: "event"}

			d.SendCallbacks(rule, target, event)

			require.Len(t, stub.calls, 1)
			require.Same(t, rule, stub.calls[0].Rule)
			require.Equal(t, []*models.AlertCurEvent{event}, stub.calls[0].Events)
			require.Equal(t, []*models.User{users.GetByUserId(1)}, stub.calls[0].Users)
			require.Contains(t, stub.calls[0].CallBackURL, "/hook")
		})
	}
}

func TestSendCallbacks_SkipsEnabledGlobalWebhookDuplicate(t *testing.T) {
	stub := &round2Callback{}
	target := NewNotifyTarget()
	target.callbacks["https://example.test/hook"] = struct{}{}
	target.webhooks["https://example.test/hook"] = &models.Webhook{Enable: true}
	d := &Dispatch{userCache: &memsto.UserCacheType{}, CallBacks: map[string]sender.CallBacker{models.DefaultDomain: stub}}

	d.SendCallbacks(&models.AlertRule{}, target, &models.AlertCurEvent{})

	require.Empty(t, stub.calls)
}

type round2Provider struct {
	request *provider.NotifyRequest
	result  *provider.NotifyResult
}

func (*round2Provider) Ident() string                           { return "round2" }
func (*round2Provider) Check(*models.NotifyChannelConfig) error { return nil }
func (p *round2Provider) Notify(_ context.Context, request *provider.NotifyRequest) *provider.NotifyResult {
	p.request = request
	return p.result
}

func TestSendNotifyRuleMessage_BuildsRenderedProviderRequest(t *testing.T) {
	d := round2Dispatch(t)
	stub := &round2Provider{result: &provider.NotifyResult{Target: "recipient", Response: "accepted"}}
	old := provider.DefaultRegistry
	provider.DefaultRegistry = provider.NewRegistry()
	provider.DefaultRegistry.Register(stub)
	t.Cleanup(func() { provider.DefaultRegistry = old })
	channel := &models.NotifyChannelConfig{
		ID: 4, Name: "test channel", Ident: "round2", RequestType: "round2",
		ParamConfig: &models.NotifyParamConfig{UserInfo: &models.UserInfo{ContactKey: "email"}},
	}
	template := &models.MessageTemplate{Content: map[string]string{"body": "{{(index $events 0).RuleName}}"}}
	event := &models.AlertCurEvent{Id: 12, RuleName: "CPU high"}
	config := &models.NotifyConfig{Params: map[string]interface{}{"user_ids": []int64{1}, "team": "ops"}}

	SendNotifyRuleMessage(d.ctx, d.userCache, d.userGroupCache, d.notifyChannelCache, &memsto.CvalCache{},
		[]*models.AlertCurEvent{event}, 3, config, channel, template)

	require.NotNil(t, stub.request)
	require.Equal(t, int64(3), stub.request.NotifyRuleId)
	require.Same(t, channel, stub.request.Config)
	require.Equal(t, []string{"a@example.test"}, stub.request.Sendtos)
	require.Equal(t, "ops", stub.request.CustomParams["team"])
	require.Equal(t, "CPU high", fmt.Sprint(stub.request.TplContent["body"]))
	require.Equal(t, []*models.AlertCurEvent{event}, stub.request.Events)
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, models.NotiStatusSuccess, record.Status)
	require.Equal(t, "accepted", record.Details)
}

func TestSendNotifyRuleMessage_RecordsSMTPProviderFailure(t *testing.T) {
	d := round2Dispatch(t)
	stub := &round2Provider{result: &provider.NotifyResult{Err: errors.New("local delivery rejected")}}
	old := provider.DefaultRegistry
	provider.DefaultRegistry = provider.NewRegistry()
	provider.DefaultRegistry.Register(stub)
	t.Cleanup(func() { provider.DefaultRegistry = old })
	channel := &models.NotifyChannelConfig{ID: 4, Ident: "round2", RequestType: "smtp", Name: "mail"}
	config := &models.NotifyConfig{Params: map[string]interface{}{"bot_name": "ops"}}

	SendNotifyRuleMessage(d.ctx, d.userCache, d.userGroupCache, d.notifyChannelCache, &memsto.CvalCache{},
		[]*models.AlertCurEvent{{Id: 12}}, 3, config, channel, nil)

	require.NotNil(t, stub.request)
	require.Nil(t, stub.request.SmtpChan)
	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, "local delivery rejected", record.Details)
	require.Equal(t, models.NotiStatusFailure, record.Status)
	require.Equal(t, "mail", record.Channel)
	require.NotEmpty(t, record.Target)
}

type round2Sender struct{ calls []sender.MessageContext }

func (s *round2Sender) Send(c sender.MessageContext) { s.calls = append(s.calls, c) }

func TestReloadTpls_PreservesExtraSenderAndRendersNewTemplate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.NotifyTpl{}))
	require.NoError(t, db.Create(&models.NotifyTpl{Channel: "custom", Content: "{{.RuleName}}"}).Error)
	stub := &round2Sender{}
	d := &Dispatch{
		ctx: ctx.NewContext(context.Background(), db, true), notifyConfigCache: &memsto.NotifyConfigCacheType{},
		ExtraSenders: map[string]sender.Sender{"custom": stub},
	}

	err = d.reloadTpls()

	require.NoError(t, err)
	require.Same(t, stub, d.Senders["custom"])
	require.NotNil(t, d.Senders[models.Email])
	require.NotNil(t, d.CallBacks[models.DefaultDomain])
	require.NotNil(t, d.CallBacks[models.FeishuCardDomain])
	require.Contains(t, string(d.genNoticeBytes(&models.AlertCurEvent{RuleName: "CPU high"})), "CPU high")
}
