package models

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPRequestConfig_VerifyRequiresURLAndSupportedMethod(t *testing.T) {
	cases := []struct {
		name   string
		config HTTPRequestConfig
		want   string
	}{
		{name: "missing url", config: HTTPRequestConfig{Method: "POST"}, want: "URL cannot be empty"},
		{name: "missing method", config: HTTPRequestConfig{URL: "/notify"}, want: "method cannot be empty"},
		{name: "unsupported method", config: HTTPRequestConfig{URL: "/notify", Method: "DELETE"}, want: "GET, POST or PUT"},
		{name: "get", config: HTTPRequestConfig{URL: "/notify", Method: "GET"}},
		{name: "post", config: HTTPRequestConfig{URL: "/notify", Method: "POST"}},
		{name: "put", config: HTTPRequestConfig{URL: "/notify", Method: "PUT"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := NotifyChannelConfig{RequestConfig: &RequestConfig{HTTPRequestConfig: &tc.config}}

			err := channel.ValidateHTTPRequestConfig()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestSMTPRequestConfig_VerifyRequiresConnectionAndSenderFields(t *testing.T) {
	cases := []struct {
		name   string
		change func(*SMTPRequestConfig)
		want   string
	}{
		{name: "missing host", change: func(c *SMTPRequestConfig) { c.Host = "" }, want: "host cannot be empty"},
		{name: "missing port", change: func(c *SMTPRequestConfig) { c.Port = 0 }, want: "port must be greater than 0"},
		{name: "missing username", change: func(c *SMTPRequestConfig) { c.Username = "" }, want: "username cannot be empty"},
		{name: "missing password", change: func(c *SMTPRequestConfig) { c.Password = "" }, want: "password cannot be empty"},
		{name: "missing sender", change: func(c *SMTPRequestConfig) { c.From = "" }, want: "from address cannot be empty"},
		{name: "valid", change: func(c *SMTPRequestConfig) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := SMTPRequestConfig{
				Host: "127.0.0.1", Port: 25, Username: "user", Password: "secret", From: "n9e@example.test",
			}
			tc.change(&config)
			channel := NotifyChannelConfig{RequestConfig: &RequestConfig{SMTPRequestConfig: &config}}

			err := channel.ValidateSMTPRequestConfig()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestNotifyChannel_ValidatorsRejectMissingRequiredConfig(t *testing.T) {
	cases := []struct {
		name     string
		validate func(*NotifyChannelConfig) error
	}{
		{name: "http", validate: (*NotifyChannelConfig).ValidateHTTPRequestConfig},
		{name: "smtp", validate: (*NotifyChannelConfig).ValidateSMTPRequestConfig},
		{name: "script", validate: (*NotifyChannelConfig).ValidateScriptRequestConfig},
		{name: "flashduty", validate: (*NotifyChannelConfig).ValidateFlashDutyRequestConfig},
		{name: "pagerduty", validate: (*NotifyChannelConfig).ValidatePagerDutyRequestConfig},
		{name: "jira", validate: (*NotifyChannelConfig).ValidateJiraRequestConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := NotifyChannelConfig{RequestConfig: &RequestConfig{}}

			err := tc.validate(&channel)

			require.ErrorContains(t, err, "config cannot be nil")
		})
	}
}

func TestNotifyChannel_ValidateScriptRequiresSource(t *testing.T) {
	cases := []struct {
		name   string
		config ScriptRequestConfig
		want   string
	}{
		{name: "invalid type", config: ScriptRequestConfig{ScriptType: "python"}, want: "type must be"},
		{name: "missing source", config: ScriptRequestConfig{ScriptType: "script"}, want: "must be provided"},
		{name: "inline", config: ScriptRequestConfig{ScriptType: "script", Script: "echo hello"}},
		{name: "path", config: ScriptRequestConfig{ScriptType: "path", Path: "/tmp/notify.sh"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := NotifyChannelConfig{RequestConfig: &RequestConfig{ScriptRequestConfig: &tc.config}}

			err := channel.ValidateScriptRequestConfig()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestNotifyChannel_VerifyValidatesIdentityAndCustomParameters(t *testing.T) {
	oldVerifier := VerifyByProvider
	VerifyByProvider = nil
	t.Cleanup(func() { VerifyByProvider = oldVerifier })
	cases := []struct {
		name   string
		change func(*NotifyChannelConfig)
		want   string
	}{
		{name: "missing name", change: func(c *NotifyChannelConfig) { c.Name = "" }, want: "name cannot be empty"},
		{name: "missing ident", change: func(c *NotifyChannelConfig) { c.Ident = "" }, want: "identifier cannot be empty"},
		{name: "invalid ident", change: func(c *NotifyChannelConfig) { c.Ident = "bad/ident" }, want: "identifier must be"},
		{
			name:   "invalid type",
			change: func(c *NotifyChannelConfig) { c.RequestType = "unknown" },
			want:   "invalid request type",
		},
		{
			name: "missing parameter name",
			change: func(c *NotifyChannelConfig) {
				c.ParamConfig = &NotifyParamConfig{Custom: Params{Params: []ParamItem{{Key: "token"}}}}
			},
			want: "valid cname",
		},
		{name: "valid", change: func(c *NotifyChannelConfig) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := NotifyChannelConfig{Name: "test", Ident: "my-channel_1", RequestType: "http"}
			tc.change(&channel)

			err := channel.Verify()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestNotifyChannel_VerifyPropagatesProviderValidation(t *testing.T) {
	oldVerifier := VerifyByProvider
	t.Cleanup(func() { VerifyByProvider = oldVerifier })
	providerError := errors.New("provider rejected configuration")
	var verified *NotifyChannelConfig
	VerifyByProvider = func(c *NotifyChannelConfig) error { verified = c; return providerError }
	channel := &NotifyChannelConfig{Name: "test", Ident: "test", RequestType: "http"}

	err := channel.Verify()

	require.ErrorIs(t, err, providerError)
	require.Same(t, channel, verified)
}

func TestJiraRequestConfig_VerifyValidatesCloudCredentials(t *testing.T) {
	cases := []struct {
		name   string
		change func(*JiraRequestConfig)
		want   string
	}{
		{name: "missing site", change: func(c *JiraRequestConfig) { c.SiteURL = " " }, want: "site url cannot be empty"},
		{name: "bad scheme", change: func(c *JiraRequestConfig) { c.SiteURL = "ftp://jira.test" }, want: "site url must be"},
		{name: "missing host", change: func(c *JiraRequestConfig) { c.SiteURL = "https:///" }, want: "site url must be"},
		{name: "bad deployment", change: func(c *JiraRequestConfig) { c.DeploymentType = "server" }, want: "deployment type"},
		{name: "bad token type", change: func(c *JiraRequestConfig) { c.TokenType = "unknown" }, want: "token type"},
		{name: "missing email", change: func(c *JiraRequestConfig) { c.Email = " " }, want: "email cannot be empty"},
		{name: "missing token", change: func(c *JiraRequestConfig) { c.APIToken = " " }, want: "api token cannot be empty"},
		{name: "variable site", change: func(c *JiraRequestConfig) { c.SiteURL = "{{ $site }}" }},
		{name: "classic token", change: func(c *JiraRequestConfig) { c.TokenType = JiraTokenClassic }},
		{name: "scoped token", change: func(c *JiraRequestConfig) { c.TokenType = JiraTokenScoped }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := JiraRequestConfig{SiteURL: "https://jira.example.test", Email: "a@example.test", APIToken: "secret"}
			tc.change(&config)
			channel := NotifyChannelConfig{RequestConfig: &RequestConfig{JiraRequestConfig: &config}}

			err := channel.ValidateJiraRequestConfig()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestNotifyChannel_ValidateDiscordAvatar(t *testing.T) {
	cases := []struct {
		name      string
		avatar    string
		wantError bool
	}{
		{name: "default"},
		{name: "https", avatar: "https://image.example.test/avatar.png"},
		{name: "variable", avatar: "{{ $avatar }}"},
		{name: "invalid", avatar: "file:///tmp/avatar.png", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := NotifyChannelConfig{RequestConfig: &RequestConfig{
				DiscordRequestConfig: &DiscordRequestConfig{AvatarURL: tc.avatar},
			}}

			err := channel.ValidateDiscordRequestConfig()

			if tc.wantError {
				require.ErrorContains(t, err, "avatar url")
			} else {
				require.NoError(t, err)
			}
		})
	}
	require.NoError(t, (&NotifyChannelConfig{}).ValidateDiscordRequestConfig())
}

func TestNotifyChannel_ValidateMattermostIcon(t *testing.T) {
	cases := []struct {
		name      string
		icon      string
		wantError bool
	}{
		{name: "default"},
		{name: "url", icon: "https://image.example.test/icon.png"},
		{name: "emoji", icon: ":bell:"},
		{name: "variable", icon: "{{ $icon }}"},
		{name: "invalid", icon: "bell", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := &MattermostWebhookRequestConfig{Icon: tc.icon}
			channel := NotifyChannelConfig{RequestConfig: &RequestConfig{MattermostWebhookRequestConfig: config}}

			err := channel.ValidateMattermostWebhookRequestConfig()

			if tc.wantError {
				require.ErrorContains(t, err, "mattermost icon")
			} else {
				require.NoError(t, err)
			}
		})
	}
	require.NoError(t, (&NotifyChannelConfig{}).ValidateMattermostWebhookRequestConfig())
}

func TestNotifyChannel_ValidateJSMPriorityMapping(t *testing.T) {
	cases := []struct {
		name   string
		config JSMAlertRequestConfig
		want   string
	}{
		{name: "default"},
		{
			name:   "custom",
			config: JSMAlertRequestConfig{APIURL: "https://jsm.example.test", PriorityMap: map[string]string{"1": " p4 "}},
		},
		{name: "variable", config: JSMAlertRequestConfig{APIURL: "{{ $api }}"}},
		{name: "invalid url", config: JSMAlertRequestConfig{APIURL: "file:///tmp/jsm"}, want: "api url"},
		{
			name:   "invalid severity",
			config: JSMAlertRequestConfig{PriorityMap: map[string]string{"4": "P1"}},
			want:   "invalid severity",
		},
		{
			name:   "invalid priority",
			config: JSMAlertRequestConfig{PriorityMap: map[string]string{"1": "P6"}},
			want:   "invalid priority",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := NotifyChannelConfig{RequestConfig: &RequestConfig{JSMAlertRequestConfig: &tc.config}}

			err := channel.ValidateJSMAlertRequestConfig()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
	require.NoError(t, (&NotifyChannelConfig{}).ValidateJSMAlertRequestConfig())
}

func TestJSMAlertRequestConfig_PriorityNormalizesOverridesAndUsesDefaults(t *testing.T) {
	config := &JSMAlertRequestConfig{PriorityMap: map[string]string{"1": " p4 ", "2": " "}}

	require.Equal(t, "P4", config.Priority(1))
	require.Equal(t, "P2", config.Priority(2))
	require.Equal(t, "P3", config.Priority(3))
	require.Equal(t, "P1", (*JSMAlertRequestConfig)(nil).Priority(1))
}

func TestGetHTTPClient_AppliesProviderNetworkOverrides(t *testing.T) {
	cases := []struct {
		name        string
		requestType string
		config      *RequestConfig
		want        time.Duration
		insecure    bool
	}{
		{name: "native defaults", requestType: RequestTypeTeams, want: 10 * time.Second},
		{
			name: "http defaults", requestType: "http", config: &RequestConfig{HTTPRequestConfig: &HTTPRequestConfig{}},
			want: 10 * time.Second,
		},
		{
			name: "flashduty", requestType: "flashduty",
			config: &RequestConfig{FlashDutyRequestConfig: &FlashDutyRequestConfig{Timeout: 125}}, want: 125 * time.Millisecond,
		},
		{
			name: "feishu", requestType: "feishuapp",
			config: &RequestConfig{FeishuAppRequestConfig: &FeishuAppRequestConfig{Timeout: 150}}, want: 150 * time.Millisecond,
		},
		{
			name: "wecom", requestType: "wecomapp",
			config: &RequestConfig{WecomAppRequestConfig: &WecomAppRequestConfig{Timeout: 175}}, want: 175 * time.Millisecond,
		},
		{
			name: "native override", requestType: RequestTypeTeams,
			config: &RequestConfig{TeamsRequestConfig: &TeamsRequestConfig{
				NativeNetworkConfig: NativeNetworkConfig{Timeout: 250, Proxy: "http://127.0.0.1:9999", InsecureSkipVerify: true},
			}},
			want: 250 * time.Millisecond, insecure: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := &NotifyChannelConfig{RequestType: tc.requestType, RequestConfig: tc.config}

			client, err := GetHTTPClient(channel)

			require.NoError(t, err)
			require.Equal(t, tc.want, client.Timeout)
			transport := client.Transport.(*http.Transport)
			t.Cleanup(transport.CloseIdleConnections)
			require.Equal(t, tc.insecure, transport.TLSClientConfig.InsecureSkipVerify)
			if tc.name == "native override" {
				proxy, err := transport.Proxy(httptest.NewRequest("POST", "http://127.0.0.1/notify", nil))
				require.NoError(t, err)
				require.Equal(t, "http://127.0.0.1:9999", proxy.String())
			}
		})
	}
}

func TestGetHTTPClient_RejectsMissingConfigAndMalformedProxy(t *testing.T) {
	cases := []struct {
		name    string
		channel NotifyChannelConfig
		want    string
	}{
		{name: "missing config", channel: NotifyChannelConfig{RequestType: "http"}, want: "request config not found"},
		{
			name: "invalid proxy",
			channel: NotifyChannelConfig{RequestType: "http", RequestConfig: &RequestConfig{
				HTTPRequestConfig: &HTTPRequestConfig{Proxy: "://invalid"},
			}},
			want: "invalid proxy URL",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := GetHTTPClient(&tc.channel)

			require.ErrorContains(t, err, tc.want)
			require.Nil(t, client)
		})
	}
}

func TestMessageTemplate_VerifyRequiresValidIdentityAndPrivateGroups(t *testing.T) {
	cases := []struct {
		name   string
		change func(*MessageTemplate)
		want   string
	}{
		{name: "missing name", change: func(c *MessageTemplate) { c.Name = "" }, want: "name cannot be empty"},
		{name: "missing ident", change: func(c *MessageTemplate) { c.Ident = "" }, want: "identifier cannot be empty"},
		{name: "bad ident", change: func(c *MessageTemplate) { c.Ident = "bad/ident" }, want: "identifier must be"},
		{
			name:   "empty content key",
			change: func(c *MessageTemplate) { c.Content = map[string]string{"": "text"} },
			want:   "empty keys",
		},
		{name: "private without groups", change: func(c *MessageTemplate) { c.Private = 1 }, want: "user group IDs"},
		{name: "invalid private flag", change: func(c *MessageTemplate) { c.Private = 2 }, want: "private flag"},
		{name: "private with group", change: func(c *MessageTemplate) { c.Private = 1; c.UserGroupIds = []int64{7} }},
		{name: "public", change: func(c *MessageTemplate) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			template := MessageTemplate{Name: "test", Ident: "my-template_1", Content: map[string]string{"text": "alert"}}
			tc.change(&template)

			err := template.Verify()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestNotifyRule_VerifyRequiresNameAndValidNotificationConfigs(t *testing.T) {
	rule := NotifyRule{}

	require.EqualError(t, rule.Verify(), "name cannot be empty")
	rule.Name = "on-call"
	rule.NotifyConfigs = []NotifyConfig{{ChannelID: 0}}
	require.EqualError(t, rule.Verify(), "invalid channel id")
	rule.NotifyConfigs = []NotifyConfig{{ChannelID: 3, Severities: []int{1, 2, 3}}}
	require.NoError(t, rule.Verify())
}

func TestNotifyConfig_VerifyChecksSeverityTimeRangesAndLabelFilters(t *testing.T) {
	cases := []struct {
		name   string
		config NotifyConfig
		want   string
	}{
		{name: "invalid channel", config: NotifyConfig{ChannelID: -1}, want: "invalid channel id"},
		{
			name:   "severity below range",
			config: NotifyConfig{ChannelID: 3, Severities: []int{0}},
			want:   "invalid severity level",
		},
		{
			name:   "severity above range",
			config: NotifyConfig{ChannelID: 3, Severities: []int{4}},
			want:   "invalid severity level",
		},
		{
			name: "missing start", config: NotifyConfig{ChannelID: 3, TimeRanges: []TimeRanges{{End: "17:00"}}},
			want: "start time cannot be empty",
		},
		{
			name: "missing end", config: NotifyConfig{ChannelID: 3, TimeRanges: []TimeRanges{{Start: "09:00"}}},
			want: "end time cannot be empty",
		},
		{
			name: "invalid label", config: NotifyConfig{ChannelID: 3, LabelKeys: []TagFilter{{Key: "env", Func: "unknown"}}},
			want: "invalid operation",
		},
		{
			name: "valid",
			config: NotifyConfig{
				ChannelID: 3, Severities: []int{1, 2, 3}, TimeRanges: []TimeRanges{{Start: "09:00", End: "17:00"}},
				LabelKeys: []TagFilter{{Key: "env", Func: "==", Value: "prod"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Verify()

			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.want)
			}
		})
	}
}

func TestNotifyRule_CleanFEFieldsPreservesDeliveryConfiguration(t *testing.T) {
	rule := NotifyRule{NotifyConfigs: []NotifyConfig{{
		ChannelID: 3, TemplateID: 4, Params: map[string]interface{}{"user_ids": []int64{7}},
		ChannelIdent: "email", UserNames: []string{"Alice"}, UserGroupNames: []string{"Ops"},
	}}}

	rule.CleanFEFields()

	config := rule.NotifyConfigs[0]
	require.Empty(t, config.ChannelIdent)
	require.Nil(t, config.UserNames)
	require.Nil(t, config.UserGroupNames)
	require.Equal(t, int64(3), config.ChannelID)
	require.Equal(t, int64(4), config.TemplateID)
	require.Equal(t, []int64{7}, config.ParseUserIDs())
}

func TestNotifyConfig_ParseRecipientIDsAcceptsTypedAndDecodedJSONArrays(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		want  []int64
	}{
		{name: "typed", value: []int64{7, 8}, want: []int64{7, 8}},
		{name: "decoded json", value: []interface{}{float64(7), float64(8)}, want: []int64{7, 8}},
		{name: "absent"},
		{name: "invalid type", value: "7 8"},
		{name: "fractional id", value: []float64{7.5}},
		{name: "unencodable", value: make(chan int)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := NotifyConfig{Params: map[string]interface{}{"user_ids": tc.value, "user_group_ids": tc.value}}

			users := config.ParseUserIDs()
			groups := config.ParseUserGroupIDs()

			require.Equal(t, tc.want, users)
			require.Equal(t, tc.want, groups)
		})
	}
}
