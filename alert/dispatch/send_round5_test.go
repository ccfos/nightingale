package dispatch

import (
	"testing"

	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestDispatchSend_PassesEventAndRecipientsToSelectedSender(t *testing.T) {
	d := round2Dispatch(t)
	stub := &round2Sender{}
	d.Senders = map[string]sender.Sender{"email": stub}
	d.notifyConfigCache = &memsto.NotifyConfigCacheType{}
	var inspected *models.AlertCurEvent
	d.BeforeSenderHook = func(event *models.AlertCurEvent) bool {
		inspected = event
		return true
	}
	target := NewNotifyTarget()
	target.userMap[1] = NewNotifyChannels([]string{"email", "unknown"})
	event := &models.AlertCurEvent{Id: 12, RuleName: "CPU high", Hash: "cpu-1", RuleConfig: "{}"}
	rule := &models.AlertRule{Id: 7}

	d.Send(rule, event, target, false)

	require.Same(t, event, inspected)
	require.Len(t, stub.calls, 1)
	require.Same(t, rule, stub.calls[0].Rule)
	require.Equal(t, []*models.AlertCurEvent{event}, stub.calls[0].Events)
	require.Equal(t, []*models.User{d.userCache.GetByUserId(1)}, stub.calls[0].Users)
	require.Same(t, d.ctx, stub.calls[0].Ctx)
}

func TestDispatchSend_SenderHookDoesNotSuppressCallbacks(t *testing.T) {
	d := round2Dispatch(t)
	stub, callback := &round2Sender{}, &round2Callback{}
	d.Senders = map[string]sender.Sender{"email": stub}
	d.CallBacks = map[string]sender.CallBacker{models.DefaultDomain: callback}
	d.notifyConfigCache = &memsto.NotifyConfigCacheType{}
	d.BeforeSenderHook = func(*models.AlertCurEvent) bool { return false }
	target := NewNotifyTarget()
	target.userMap[1] = NewNotifyChannels([]string{"email"})
	target.callbacks["http://local.test/hook"] = struct{}{}
	event := &models.AlertCurEvent{Id: 12, RuleConfig: "{}"}

	d.Send(&models.AlertRule{}, event, target, true)

	require.Empty(t, stub.calls)
	require.Len(t, callback.calls, 1)
	require.Equal(t, "http://local.test/hook", callback.calls[0].CallBackURL)
	require.Equal(t, []*models.AlertCurEvent{event}, callback.calls[0].Events)
}

func TestNeedBatchContacts_DetectsRecipientListTemplates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config *models.HTTPRequestConfig
		want   bool
	}{
		{name: "nil"},
		{name: "singular", config: &models.HTTPRequestConfig{URL: "http://local.test/{{$sendto}}"}},
		{
			name: "body list", want: true,
			config: &models.HTTPRequestConfig{Request: models.RequestDetail{Body: `{"users":{{$sendtos}}}`}},
		},
		{
			name: "header list", want: true,
			config: &models.HTTPRequestConfig{Headers: map[string]string{"X-Recipients": "{{$sendtos}}"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := NeedBatchContacts(tc.config)

			require.Equal(t, tc.want, got)
		})
	}
}
