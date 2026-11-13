package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertRuleVerify_RejectsInvalidBusinessGroup(t *testing.T) {
	rule := AlertRule{GroupId: -1, Name: "CPU", RuleConfig: `{"prom_ql":"up == 0","severity":2}`}

	err := rule.Verify()

	require.EqualError(t, err, "GroupId(-1) invalid")
}

func TestAlertRuleVerify_RejectsInvalidTimeZone(t *testing.T) {
	rule := AlertRule{Name: "CPU", TimeZone: "invalid/location", RuleConfig: `{"prom_ql":"up == 0","severity":2}`}

	err := rule.Verify()

	require.EqualError(t, err, "invalid timezone: invalid/location")
}

func TestAlertRuleVerify_RejectsUnsafeName(t *testing.T) {
	rule := AlertRule{Name: "<script>", RuleConfig: `{"prom_ql":"up == 0","severity":2}`}

	err := rule.Verify()

	require.EqualError(t, err, "Name has invalid characters")
}

func TestAlertRuleVerify_RejectsMalformedAppendTag(t *testing.T) {
	rule := AlertRule{Name: "CPU", AppendTags: "env", RuleConfig: `{"prom_ql":"up == 0","severity":2}`}

	err := rule.Verify()

	require.EqualError(t, err, "AppendTags(env) invalid")
}

func TestAlertRuleVerify_RejectsDuplicateAppendTagKey(t *testing.T) {
	rule := AlertRule{Name: "CPU", AppendTags: "env=prod env=dev", RuleConfig: `{"prom_ql":"up == 0","severity":2}`}

	err := rule.Verify()

	require.EqualError(t, err, "AppendTags has duplicate key: env")
}

func TestAlertRuleVerify_RejectsNonnumericNotifyGroup(t *testing.T) {
	rule := AlertRule{Name: "CPU", NotifyGroups: "7 invalid", RuleConfig: `{"prom_ql":"up == 0","severity":2}`}

	err := rule.Verify()

	require.EqualError(t, err, "NotifyGroups(7 invalid) invalid")
}

func TestRequestConfig_NativeNetworkSelectsProviderSettings(t *testing.T) {
	config := &RequestConfig{
		TwilioRequestConfig:       &TwilioRequestConfig{NativeNetworkConfig: NativeNetworkConfig{Timeout: 101}},
		PushoverRequestConfig:     &PushoverRequestConfig{NativeNetworkConfig: NativeNetworkConfig{Timeout: 102}},
		SplunkOnCallRequestConfig: &SplunkOnCallRequestConfig{NativeNetworkConfig: NativeNetworkConfig{Timeout: 103}},
		LineRequestConfig:         &LineRequestConfig{NativeNetworkConfig: NativeNetworkConfig{Timeout: 104}},
		JiraRequestConfig:         &JiraRequestConfig{NativeNetworkConfig: NativeNetworkConfig{Timeout: 105}},
	}
	for _, tc := range []struct {
		name string
		want *NativeNetworkConfig
	}{
		{name: RequestTypeTwilio, want: &config.TwilioRequestConfig.NativeNetworkConfig},
		{name: RequestTypePushover, want: &config.PushoverRequestConfig.NativeNetworkConfig},
		{name: RequestTypeSplunkOnCall, want: &config.SplunkOnCallRequestConfig.NativeNetworkConfig},
		{name: RequestTypeLine, want: &config.LineRequestConfig.NativeNetworkConfig},
		{name: RequestTypeJira, want: &config.JiraRequestConfig.NativeNetworkConfig},
	} {
		t.Run(tc.name, func(t *testing.T) {
			network := config.NativeNetwork(tc.name)

			require.Same(t, tc.want, network)
			require.Equal(t, tc.want.Timeout, network.Timeout)
		})
	}
}
