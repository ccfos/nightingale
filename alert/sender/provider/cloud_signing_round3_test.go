package provider

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func aliyunRound3Request(t *testing.T, endpoint string, voice bool) *NotifyRequest {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	require.NoError(t, err)
	parameters := map[string]string{
		"AccessKeyId": "local-ak", "AccessKeySecret": "local-secret", "PhoneNumbers": "{{$sendto}}",
		"SignName": "Ops Team", "TemplateCode": "SMS_1", "TemplateParam": "{{$tpl.content}}",
	}
	if voice {
		parameters = map[string]string{
			"AccessKeyId": "local-ak", "AccessKeySecret": "local-secret", "CalledNumber": "{{$sendto}}",
			"CalledShowNumber": "01012345678", "TtsCode": "TTS_1", "TtsParam": "{{$tpl.content}}",
		}
	}
	req := webhookReq("http", &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
		URL: endpoint, Method: http.MethodPost, Headers: map[string]string{"Content-Type": "application/json", "Host": parsed.Host},
		RetryTimes: 1, Request: models.RequestDetail{Parameters: parameters},
	}}, nil)
	req.Sendtos = []string{"13800000000"}
	req.TplContent = map[string]interface{}{"content": "CPU high"}
	return req
}

func TestAliyunSMSNotify_SignsRenderedQueryAndRemovesCredentials(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"Code":"OK","RequestId":"request-1"}`))
	req := aliyunRound3Request(t, srv.URL, false)

	result := (&AliyunSmsProvider{}).Notify(context.Background(), req)

	require.NoError(t, result.Err)
	require.Equal(t, "13800000000", result.Target)
	require.Contains(t, result.Response, `"Code":"OK"`)
	require.Len(t, got(), 1)
	sent := got()[0]
	require.Equal(t, http.MethodPost, sent.Method)
	require.Equal(t, "/", sent.Path)
	require.Equal(t, url.Values{
		"PhoneNumbers": []string{"13800000000"}, "SignName": []string{"Ops Team"},
		"TemplateCode": []string{"SMS_1"}, "TemplateParam": []string{"CPU high"},
	}, sent.Query)
	require.Equal(t, "SendSms", sent.Header.Get("X-Acs-Action"))
	require.Equal(t, "2017-05-25", sent.Header.Get("X-Acs-Version"))
	require.NotEmpty(t, sent.Header.Get("X-Acs-Date"))
	require.NotEmpty(t, sent.Header.Get("X-Acs-Signature-Nonce"))
	require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", sent.Header.Get("X-Acs-Content-Sha256"))
	require.Regexp(t, `^ACS3-HMAC-SHA256 Credential=local-ak,SignedHeaders=host;.*Signature=[0-9a-f]{64}$`,
		sent.Header.Get("Authorization"))
	require.Empty(t, sent.Body)
	require.NotContains(t, string(sent.Body), "local-secret")
}

func TestAliyunVoiceNotify_SignsTTSParameters(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"Code":"OK"}`))
	req := aliyunRound3Request(t, srv.URL, true)

	result := (&AliyunVoiceProvider{}).Notify(context.Background(), req)

	require.NoError(t, result.Err)
	require.Len(t, got(), 1)
	sent := got()[0]
	require.Equal(t, http.MethodPost, sent.Method)
	require.Equal(t, url.Values{
		"CalledNumber": []string{"13800000000"}, "CalledShowNumber": []string{"01012345678"},
		"TtsCode": []string{"TTS_1"}, "TtsParam": []string{"CPU high"},
	}, sent.Query)
	require.Equal(t, "SingleCallByTts", sent.Header.Get("X-Acs-Action"))
	require.Contains(t, sent.Header.Get("Authorization"), "Credential=local-ak,")
	require.Empty(t, sent.Query.Get("AccessKeySecret"))
	require.Empty(t, sent.Query.Get("AccessKeyId"))
}

func TestAliyunNotify_ReportsHTTPRejection(t *testing.T) {
	cases := []struct {
		name     string
		provider NotifyChannelProvider
		voice    bool
	}{
		{name: "SMS", provider: &AliyunSmsProvider{}},
		{name: "voice", provider: &AliyunVoiceProvider{}, voice: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, always(403, `{"message":"access denied"}`))
			req := aliyunRound3Request(t, srv.URL, tc.voice)

			result := tc.provider.Notify(context.Background(), req)

			require.ErrorContains(t, result.Err, "status code: 403")
			require.Contains(t, result.Response, "access denied")
			require.Len(t, got(), 1)
		})
	}
}

func TestAliyunNotify_RejectsMissingClient(t *testing.T) {
	cases := []struct {
		name     string
		provider NotifyChannelProvider
		voice    bool
	}{
		{name: "SMS", provider: &AliyunSmsProvider{}},
		{name: "voice", provider: &AliyunVoiceProvider{}, voice: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := aliyunRound3Request(t, "http://127.0.0.1", tc.voice)
			req.HttpClient = nil

			result := tc.provider.Notify(context.Background(), req)

			require.ErrorContains(t, result.Err, "http client not found")
		})
	}
}

func TestAliyunNotify_RejectsEmptyEvents(t *testing.T) {
	cases := []struct {
		name     string
		provider NotifyChannelProvider
		voice    bool
	}{
		{name: "SMS", provider: &AliyunSmsProvider{}},
		{name: "voice", provider: &AliyunVoiceProvider{}, voice: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := aliyunRound3Request(t, "http://127.0.0.1", tc.voice)
			req.Events = nil

			result := tc.provider.Notify(context.Background(), req)

			require.ErrorContains(t, result.Err, "events is empty")
		})
	}
}

func TestAliyunNotify_RejectsInvalidBodyTemplateBeforeSending(t *testing.T) {
	cases := []struct {
		name     string
		provider NotifyChannelProvider
		voice    bool
	}{
		{name: "SMS", provider: &AliyunSmsProvider{}},
		{name: "voice", provider: &AliyunVoiceProvider{}, voice: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, always(200, `{"Code":"OK"}`))
			req := aliyunRound3Request(t, srv.URL, tc.voice)
			req.Config.RequestConfig.HTTPRequestConfig.Request.Body = "{{"

			result := tc.provider.Notify(context.Background(), req)

			require.ErrorContains(t, result.Err, "unclosed action")
			require.Empty(t, got())
		})
	}
}

func TestCloudProviderCheck_ValidatesHTTPContract(t *testing.T) {
	providers := []struct {
		name     string
		provider NotifyChannelProvider
	}{
		{name: "Aliyun SMS", provider: &AliyunSmsProvider{}},
		{name: "Aliyun voice", provider: &AliyunVoiceProvider{}},
		{name: "Tencent SMS", provider: &TencentSmsProvider{}},
		{name: "Tencent voice", provider: &TencentVoiceProvider{}},
	}
	cases := []struct {
		name   string
		mutate func(*models.HTTPRequestConfig)
		want   string
	}{
		{name: "valid", mutate: func(*models.HTTPRequestConfig) {}},
		{name: "wrong method", mutate: func(c *models.HTTPRequestConfig) { c.Method = "GET" }, want: "requires POST"},
		{name: "missing URL", mutate: func(c *models.HTTPRequestConfig) { c.URL = "" }, want: "URL cannot be empty"},
		{name: "missing headers", mutate: func(c *models.HTTPRequestConfig) { c.Headers = nil }, want: "Content-Type"},
		{
			name:   "wrong content type",
			mutate: func(c *models.HTTPRequestConfig) { c.Headers["Content-Type"] = "text/plain" }, want: "Content-Type",
		},
		{
			name:   "missing payload",
			mutate: func(c *models.HTTPRequestConfig) { c.Request = models.RequestDetail{} }, want: "requires request body",
		},
	}
	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					cfg := &models.NotifyChannelConfig{RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
						URL: "http://127.0.0.1", Method: "POST", Headers: map[string]string{"Content-Type": "application/json"},
						Request: models.RequestDetail{Body: `{"message":"alert"}`},
					}}}
					tc.mutate(cfg.RequestConfig.HTTPRequestConfig)

					err := p.provider.Check(cfg)

					if tc.want == "" {
						require.NoError(t, err)
					} else {
						require.ErrorContains(t, err, tc.want)
					}
				})
			}
		})
	}
}

func TestAliyunSignature_MatchesFixedCanonicalRequest(t *testing.T) {
	headers := map[string]string{
		"host": "sms.example.invalid", "x-acs-action": "SendSms", "x-acs-version": "2017-05-25",
		"x-acs-date": "2026-01-02T03:04:05Z", "x-acs-signature-nonce": "fixed-nonce", "Unrelated": "not signed",
	}
	params := map[string]string{
		"PhoneNumbers": "+12025550123", "SignName": "Ops Team", "TemplateCode": "SMS_1", "TemplateParam": `{"value":"a+b"}`,
	}

	signature, signedHeaders := getSignature("secret", "POST", "/", headers, params, "")

	// Fixed HMAC vector includes reserved query characters and excludes unrelated headers.
	require.Equal(t, "40dff8528741cd809ae514a54d590d88a3f97ceb51f930fba3ea75a478963026", signature)
	require.Equal(t, "host;x-acs-action;x-acs-date;x-acs-signature-nonce;x-acs-version", signedHeaders)
}

func TestTencentSignature_MatchesFixedCanonicalRequest(t *testing.T) {
	cfg := &models.HTTPRequestConfig{Method: "POST", Headers: map[string]string{
		"Host": "sms.example.invalid", "X-TC-Action": "SendSms", "Service": "sms", "Secret_ID": "local-ak", "Secret_Key": "secret",
	}}

	authorization := getTxSignature(cfg, `{"PhoneNumberSet":["+12025550123"]}`, 1551113065)

	require.Equal(t, "TC3-HMAC-SHA256 Credential=local-ak/2019-02-25/sms/tc3_request, "+
		"SignedHeaders=content-type;host;x-tc-action, Signature=1495990363eb80215a5368b5e4b04ce028cf53dfa4e528c0408d69e793140c78",
		authorization)
}

func TestSetTxHeader_DoesNotSendSecretKey(t *testing.T) {
	cfg := &models.HTTPRequestConfig{Method: "POST", Headers: map[string]string{
		"Host": "sms.example.invalid", "X-TC-Action": "SendSms", "Service": "sms", "Secret_ID": "local-ak", "Secret_Key": "secret",
	}}
	headers := map[string]string{"Host": "sms.example.invalid", "Secret_ID": "local-ak", "Secret_Key": "secret"}

	got := setTxHeader(cfg, headers, []byte(`{}`))

	require.NotContains(t, got, "Secret_Key")
	require.Contains(t, got["Authorization"], "Credential=local-ak/")
	// The signing input still comes from the channel config.
	require.Equal(t, "secret", cfg.Headers["Secret_Key"])
}
