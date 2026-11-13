package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

// Rewrite fixed provider API URLs to a local server; no request can reach the original host.
func wecomLocalClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	destination, err := url.Parse(server.URL)
	require.NoError(t, err)
	transport := server.Client().Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &http.Client{Transport: flashDutyTestTransport(func(req *http.Request) (*http.Response, error) {
		local := req.Clone(req.Context())
		local.URL.Scheme = destination.Scheme
		local.URL.Host = destination.Host
		local.Host = destination.Host
		return transport.RoundTrip(local)
	})}
}

func TestWecomAppNotify_ResolvesRecipientsAndRendersMarkdown(t *testing.T) {
	cases := []struct {
		name       string
		contactKey string
		recipient  string
		lookup     string
		lookupBody string
	}{
		{name: "direct user", contactKey: "userid", recipient: "alice"},
		{
			name: "mobile", contactKey: "phone", recipient: "13800000000", lookup: "/cgi-bin/user/getuserid",
			lookupBody: `{"mobile":"13800000000"}`,
		},
		{
			name: "email", contactKey: "email", recipient: "alice@example.invalid", lookup: "/cgi-bin/user/get_userid_by_email",
			lookupBody: `{"email":"alice@example.invalid","email_type":1}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
				switch r.Path {
				case "/cgi-bin/gettoken":
					return 200, `{"errcode":0,"access_token":"local-token"}`
				case "/cgi-bin/user/getuserid", "/cgi-bin/user/get_userid_by_email":
					return 200, `{"errcode":0,"userid":"alice"}`
				case "/cgi-bin/message/send":
					return 200, `{"errcode":0,"errmsg":"ok","msgid":"42"}`
				default:
					return 404, `{}`
				}
			})
			req := webhookReq("wecomapp", &models.RequestConfig{
				WecomAppRequestConfig: &models.WecomAppRequestConfig{CorpID: "corp", CorpSecret: "local-secret", AgentID: 42, RetryTimes: 1},
			}, nil)
			req.Config.ParamConfig = &models.NotifyParamConfig{UserInfo: &models.UserInfo{ContactKey: tc.contactKey}}
			req.HttpClient = wecomLocalClient(t, srv)
			req.Sendtos = []string{" ", " " + tc.recipient + " ", "bob"}
			// Lookup-based cases use one recipient so each lookup has a unique observable input.
			if tc.lookup != "" {
				req.Sendtos = []string{tc.recipient}
			}
			req.TplContent = map[string]interface{}{"title": "{{.event.RuleName}}", "content": "Recipient {{index .userIDs 0}}"}

			result := (&WecomAppProvider{}).Notify(context.Background(), req)

			require.NoError(t, result.Err)
			require.Contains(t, result.Response, `"msgid":"42"`)
			calls := got()
			require.Equal(t, http.MethodGet, calls[0].Method)
			require.Equal(t, "/cgi-bin/gettoken", calls[0].Path)
			require.Equal(t, "corp", calls[0].Query.Get("corpid"))
			require.Equal(t, "local-secret", calls[0].Query.Get("corpsecret"))
			expectedTarget := "alice,bob"
			expectedToUser := "alice|bob"
			if tc.lookup != "" {
				require.Len(t, calls, 3)
				require.Equal(t, tc.lookup, calls[1].Path)
				require.Equal(t, "local-token", calls[1].Query.Get("access_token"))
				require.Equal(t, http.MethodPost, calls[1].Method)
				require.JSONEq(t, tc.lookupBody, string(calls[1].Body))
				expectedTarget, expectedToUser = "alice", "alice"
			} else {
				require.Len(t, calls, 2)
			}
			require.Equal(t, expectedTarget, result.Target)
			sent := calls[len(calls)-1]
			require.Equal(t, "/cgi-bin/message/send", sent.Path)
			require.Equal(t, http.MethodPost, sent.Method)
			require.Equal(t, "local-token", sent.Query.Get("access_token"))
			require.Equal(t, "application/json", sent.Header.Get("Content-Type"))
			require.JSONEq(t, fmt.Sprintf(`{
   "touser":%q,"msgtype":"markdown","agentid":42,
   "markdown":{"content":"## cpu <high> & hot\n\nRecipient alice"}
  }`, expectedToUser), string(sent.Body))
		})
	}
}

func TestWecomAppNotify_PropagatesTokenFailureWithoutSending(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"errcode":40013,"errmsg":"invalid corp"}`))
	req := webhookReq("wecomapp", &models.RequestConfig{
		WecomAppRequestConfig: &models.WecomAppRequestConfig{CorpID: "bad", CorpSecret: "secret", AgentID: 1, RetryTimes: 1},
	}, nil)
	req.HttpClient = wecomLocalClient(t, srv)
	req.Sendtos = []string{"alice"}

	result := (&WecomAppProvider{}).Notify(context.Background(), req)

	require.ErrorContains(t, result.Err, "wecom get access_token")
	require.ErrorContains(t, result.Err, "40013")
	require.Len(t, got(), 1)
	require.Equal(t, "/cgi-bin/gettoken", got()[0].Path)
}

func TestWecomAppNotify_RejectsUnresolvableRecipients(t *testing.T) {
	cases := []struct {
		name       string
		contactKey string
		response   string
		want       string
	}{
		{
			name: "mobile rejected", contactKey: "phone",
			response: `{"errcode":60111,"errmsg":"user not found"}`, want: "get userid by mobile",
		},
		{name: "email absent", contactKey: "email", response: `{"errcode":0,"userid":""}`, want: "get userid by email"},
		{
			name: "email malformed response", contactKey: "email",
			response: `{`, want: "parse wecom get_userid_by_email response",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
				if r.Path == "/cgi-bin/gettoken" {
					return 200, `{"errcode":0,"access_token":"local-token"}`
				}
				return 200, tc.response
			})
			req := webhookReq("wecomapp", &models.RequestConfig{
				WecomAppRequestConfig: &models.WecomAppRequestConfig{CorpID: "corp", CorpSecret: "secret", AgentID: 1, RetryTimes: 1},
			}, nil)
			req.HttpClient = wecomLocalClient(t, srv)
			req.Config.ParamConfig = &models.NotifyParamConfig{UserInfo: &models.UserInfo{ContactKey: tc.contactKey}}
			req.Sendtos = []string{"unknown"}

			result := (&WecomAppProvider{}).Notify(context.Background(), req)

			require.ErrorContains(t, result.Err, tc.want)
			require.Len(t, got(), 2)
			require.NotEqual(t, "/cgi-bin/message/send", got()[1].Path)
		})
	}
}

func TestWecomAppNotify_RejectsEmptyTargets(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"errcode":0,"access_token":"local-token"}`))
	req := webhookReq("wecomapp", &models.RequestConfig{
		WecomAppRequestConfig: &models.WecomAppRequestConfig{CorpID: "corp", CorpSecret: "secret", AgentID: 1, RetryTimes: 1},
	}, nil)
	req.HttpClient = wecomLocalClient(t, srv)
	req.Sendtos = []string{" ", ""}

	result := (&WecomAppProvider{}).Notify(context.Background(), req)

	require.ErrorContains(t, result.Err, "no valid wecom target")
	require.Len(t, got(), 1)
}

func TestWecomAppNotify_PropagatesMessageRejection(t *testing.T) {
	srv, got := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
		if r.Path == "/cgi-bin/gettoken" {
			return 200, `{"errcode":0,"access_token":"local-token"}`
		}
		return 200, `{"errcode":81013,"errmsg":"all recipients invalid"}`
	})
	req := webhookReq("wecomapp", &models.RequestConfig{
		WecomAppRequestConfig: &models.WecomAppRequestConfig{CorpID: "corp", CorpSecret: "secret", AgentID: 1, RetryTimes: 1},
	}, nil)
	req.HttpClient = wecomLocalClient(t, srv)
	req.Sendtos = []string{"alice"}
	req.TplContent = map[string]interface{}{"content": "CPU high"}

	result := (&WecomAppProvider{}).Notify(context.Background(), req)

	require.ErrorContains(t, result.Err, "81013")
	require.Contains(t, result.Response, "all recipients invalid")
	require.Len(t, got(), 2)
	require.JSONEq(t, `{
  "touser":"alice","msgtype":"markdown","agentid":1,"markdown":{"content":"## Alert\n\nCPU high"}
 }`, string(got()[1].Body))
}
