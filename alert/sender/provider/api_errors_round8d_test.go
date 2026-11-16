package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

// A response body can fail after headers have arrived (for example a truncated connection).
type r8dBrokenBody struct{}

func (r8dBrokenBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (r8dBrokenBody) Close() error             { return nil }

func TestR8dWecomLookupFailures(t *testing.T) {
	for _, email := range []bool{false, true} {
		for _, body := range []string{`not json`, `{"errcode":400,"errmsg":"denied"}`, `{"errcode":0}`} {
			t.Run(body+map[bool]string{true: "email", false: "mobile"}[email], func(t *testing.T) {
				srv, got := newCaptureServer(t, always(200, body))
				p := &WecomAppProvider{}
				client := wecomLocalClient(t, srv)
				var err error
				if email {
					_, err = p.getUserIDByEmail(context.Background(), client, "token &", " user@test.invalid ")
				} else {
					_, err = p.getUserIDByMobile(context.Background(), client, "token &", "123")
				}
				require.Error(t, err)
				calls := got()
				require.Len(t, calls, 1)
				require.Equal(t, "POST", calls[0].Method)
				require.Equal(t, "token &", calls[0].Query.Get("access_token"))
				require.Equal(t, "application/json", calls[0].Header.Get("Content-Type"))
				if email {
					require.Equal(t, "/cgi-bin/user/get_userid_by_email", calls[0].Path)
					require.JSONEq(t, `{"email":"user@test.invalid","email_type":1}`, string(calls[0].Body))
				} else {
					require.Equal(t, "/cgi-bin/user/getuserid", calls[0].Path)
					require.JSONEq(t, `{"mobile":"123"}`, string(calls[0].Body))
				}
			})
		}
	}
}

func TestR8dAppResponseReadFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*http.Client) error
	}{
		{"wecom token", func(c *http.Client) error {
			_, e := (&WecomAppProvider{}).getAccessToken(context.Background(), c, &models.WecomAppRequestConfig{RetryTimes: 1})
			return e
		}},
		{"wecom mobile", func(c *http.Client) error {
			_, e := (&WecomAppProvider{}).getUserIDByMobile(context.Background(), c, "token", "123")
			return e
		}},
		{"wecom email", func(c *http.Client) error {
			_, e := (&WecomAppProvider{}).getUserIDByEmail(context.Background(), c, "token", "user@test.invalid")
			return e
		}},
		{"wecom post", func(c *http.Client) error {
			_, e := (&WecomAppProvider{}).postWecomAPI(context.Background(), c, &models.WecomAppRequestConfig{RetryTimes: 1}, "token", "http://local.invalid/send", map[string]string{"text": "message"})
			return e
		}},
		{"ding token", func(c *http.Client) error { _, e := GetAccessToken(context.Background(), c, "app", "secret"); return e }},
		{"ding upload", func(c *http.Client) error {
			_, e := UploadMedia(context.Background(), c, "token", "image", "aW1hZ2U=")
			return e
		}},
	} {
		for _, transportError := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "body", true: "transport"}[transportError], func(t *testing.T) {
				count := 0
				client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) {
					count++
					if transportError {
						return nil, io.ErrUnexpectedEOF
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: r8dBrokenBody{}, Request: r}, nil
				})}
				require.ErrorIs(t, tc.call(client), io.ErrUnexpectedEOF)
				require.Equal(t, 1, count)
			})
		}
	}
}

func TestR8dWecomPostResponseValidation(t *testing.T) {
	for _, tc := range []struct{ body, want string }{{`bad json`, "parse wecom response"}, {`{"errcode":400,"errmsg":"bad token"}`, "bad token"}, {`{"errcode":0,"invaliduser":"bob"}`, ""}} {
		t.Run(tc.body, func(t *testing.T) {
			srv, got := newCaptureServer(t, always(200, tc.body))
			response, err := (&WecomAppProvider{}).postWecomAPI(context.Background(), srv.Client(), &models.WecomAppRequestConfig{RetryTimes: 1}, "tok", srv.URL+"/send", map[string]string{"text": "message"})
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
			} else {
				require.NoError(t, err)
				require.Contains(t, response, "invaliduser=bob")
			}
			require.Len(t, got(), 1)
			require.JSONEq(t, `{"text":"message"}`, string(got()[0].Body))
			require.Equal(t, "tok", got()[0].Query.Get("access_token"))
		})
	}
}

func TestR8dNativeErrorHints(t *testing.T) {
	for _, tc := range []struct {
		name     string
		p        NotifyChannelProvider
		rc       *models.RequestConfig
		params   map[string]string
		path     string
		statuses []int
	}{
		{"ntfy", &NtfyProvider{}, &models.RequestConfig{NtfyRequestConfig: &models.NtfyRequestConfig{Token: "secret"}}, map[string]string{"topic": "test-topic"}, "/", []int{401, 403, 404, 413, 429}},
		{"line", &LineProvider{}, &models.RequestConfig{LineRequestConfig: &models.LineRequestConfig{ChannelAccessToken: "secret"}}, map[string]string{"to": "U" + strings.Repeat("a", 32)}, linePushPath, []int{400, 401, 403, 429}},
		{"pushover", &PushoverProvider{}, &models.RequestConfig{PushoverRequestConfig: &models.PushoverRequestConfig{Token: strings.Repeat("t", 30)}}, map[string]string{"user_key": strings.Repeat("u", 30)}, "/1/messages.json", []int{400, 429}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, status := range tc.statuses {
				t.Run(http.StatusText(status), func(t *testing.T) {
					body := `{"message":"monthly limit failed to send 'to'","error":"denied","code":42908,"status":0,"errors":["token invalid"]}`
					srv, got := newCaptureServer(t, always(status, body))
					switch tc.name {
					case "ntfy":
						tc.rc.NtfyRequestConfig.ServerURL = srv.URL
					case "line":
						tc.rc.LineRequestConfig.APIURL = srv.URL
					case "pushover":
						tc.rc.PushoverRequestConfig.APIURL = srv.URL
					}
					req := webhookReq(tc.p.Ident(), tc.rc, tc.params)
					req.HttpClient = srv.Client()
					res := tc.p.Notify(context.Background(), req)
					require.Error(t, res.Err)
					var hint *HintError
					require.True(t, errors.As(res.Err, &hint))
					require.NotEmpty(t, hint.Hint)
					calls := got()
					require.Len(t, calls, 1)
					require.Equal(t, "POST", calls[0].Method)
					require.Equal(t, tc.path, calls[0].Path)
					if tc.name == "pushover" {
						require.Equal(t, strings.Repeat("t", 30), calls[0].form(t).Get("token"))
					} else {
						require.Equal(t, "Bearer secret", calls[0].Header.Get("Authorization"))
						require.Equal(t, "application/json", calls[0].Header.Get("Content-Type"))
					}
				})
			}
		})
	}
}
