package provider

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

func TestTeamsAdaptiveCard(t *testing.T) {
	var payload struct {
		Type        string `json:"type"`
		Attachments []struct {
			ContentType string            `json:"contentType"`
			Content     teamsAdaptiveCard `json:"content"`
		} `json:"attachments"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" || r.URL.Query().Get("sig") != "signed-secret" {
			t.Error("invalid webhook request")
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	req := webhookReq(models.RequestTypeTeams, nil, map[string]string{"webhook_url": srv.URL + "/workflows/123/triggers/manual/paths/invoke?sig=signed-secret"})
	req.TplContent["content"] = "CPU > 90% & \"hot\"\n第二行"
	for _, recovered := range []bool{false, true} {
		req.Events[0].IsRecovered = recovered
		res := (&TeamsProvider{}).Notify(context.Background(), req)
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		if res.Target != srv.URL+"/***" || res.Response != "accepted by Teams webhook" {
			t.Fatalf("unexpected result: %+v", res)
		}
		if payload.Type != "message" || len(payload.Attachments) != 1 {
			t.Fatalf("invalid envelope: %+v", payload)
		}
		a := payload.Attachments[0]
		if a.ContentType != "application/vnd.microsoft.card.adaptive" || a.Content.Type != "AdaptiveCard" || len(a.Content.Body) < 2 {
			t.Fatalf("invalid card: %+v", a)
		}
		if a.Content.Body[1].Text != req.TplContent["content"] || a.Content.Actions[0].URL != "http://n9e.example.com/share/alert-his-events/42" {
			t.Fatalf("bad content: %+v", a)
		}
		if a.Content.Body[0].Items[1].Text != req.Events[0].RuleName {
			t.Fatalf("rule name must appear once in the separate title: %+v", a.Content.Body[0])
		}
		wantColor, wantStatus := "Attention", "Triggered"
		if recovered {
			wantColor, wantStatus = "Good", "Recovered"
		}
		if status := a.Content.Body[0].Items[0].Columns[1].Items[0]; status.Color != wantColor || !strings.Contains(status.Text, wantStatus) {
			t.Fatalf("bad status: %+v", a)
		}
	}
}

func TestTeamsErrorsAndSecretMasking(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.Write([]byte("Microsoft Teams endpoint returned HTTP error 429"))
			return
		}
		w.Write([]byte("1"))
	}))
	defer srv.Close()
	req := webhookReq(models.Teams, nil, map[string]string{"webhook_url": srv.URL + "/path?sig=secret"})
	if r := (&TeamsProvider{}).Notify(context.Background(), req); r.Err != nil || calls != 2 {
		t.Fatalf("throttle retry failed: %+v calls=%d", r, calls)
	}
	for _, body := range []string{"some error", `{"error":{"message":"disabled workflow"}}`} {
		_, err := checkTeamsResponse(&nativeResponse{StatusCode: 200, Body: []byte(body)})
		if err == nil {
			t.Fatalf("accepted error: %s", body)
		}
	}
	raw := "https://example.com/webhookb2/path-secret/IncomingWebhook/another-secret?sig=signed%2Bsecret"
	masked := maskTeamsError(raw+" path-secret another-secret signed+secret signed%2Bsecret", raw)
	for _, secret := range []string{"path-secret", "another-secret", "signed+secret", "signed%2Bsecret"} {
		if strings.Contains(masked, secret) {
			t.Fatalf("leaked %s: %s", secret, masked)
		}
	}
	raw = "https://x.environment.api.powerplatform.com:443/powerautomate/automations/direct/workflows/abcdef0123456789/triggers/manual/paths/invoke?api-version=1&sp=%2Ftriggers%2Fmanual%2Frun&sv=1.0&sig=abc"
	plain := "unexpected status code 401: workflows /triggers/manual/run not authorized at 2026-10-13T11:01:10.0Z"
	if masked := maskTeamsError(plain+" abcdef0123456789 sig=abc", raw); masked != plain+" *** sig=***" {
		t.Fatalf("only credentials should be redacted: %s", masked)
	}
	for _, raw := range []string{"", "invalid", "ftp://example.com/path", "https://user:pass@example.com/path"} {
		req.CustomParams["webhook_url"] = raw
		if r := (&TeamsProvider{}).Notify(context.Background(), req); r.Err == nil {
			t.Fatalf("accepted invalid webhook %s", raw)
		}
	}
}

type twilioTestTransport func(*http.Request) (*http.Response, error)

func (f twilioTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func twilioTestReply(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"0"}}, Body: io.NopCloser(strings.NewReader(body))}
}
func twilioReq(mode string) *NotifyRequest {
	return &NotifyRequest{
		Config: &models.NotifyChannelConfig{Name: "Twilio", Ident: models.Twilio, RequestType: models.RequestTypeTwilio,
			RequestConfig: &models.RequestConfig{TwilioRequestConfig: &models.TwilioRequestConfig{
				AccountSID: "AC" + strings.Repeat("a", 32), AuthToken: "secret-auth-token", Mode: mode, From: "+14155552671",
				NativeNetworkConfig: models.NativeNetworkConfig{RetryTimes: 2, RetrySleep: 1},
			}}, ParamConfig: &models.NotifyParamConfig{UserInfo: &models.UserInfo{ContactKey: "phone"}},
		}, Sendtos: []string{"+8613800138000", "+14155552672", "+8613800138000"},
		TplContent: map[string]interface{}{"content": "CPU > 90% & \"hot\". <Hangup/> 中文"},
	}
}

func TestTwilioSMSAndVoiceRequests(t *testing.T) {
	for _, mode := range []string{"sms", "voice"} {
		t.Run(mode, func(t *testing.T) {
			req := twilioReq(mode)
			cfg := req.Config.RequestConfig.TwilioRequestConfig
			if mode == "sms" {
				cfg.From = ""
				cfg.MessagingServiceSID = "MG" + strings.Repeat("b", 32)
			} else {
				cfg.Language = "fr-FR"
			}
			calls := 0
			req.HttpClient = &http.Client{Transport: twilioTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				u, p, ok := r.BasicAuth()
				if !ok || u != cfg.AccountSID || p != cfg.AuthToken || r.Method != "POST" || r.URL.Host != "api.twilio.com" || r.URL.Scheme != "https" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Error("invalid authenticated request")
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if r.Form.Get("To") != req.Sendtos[calls-1] {
					t.Error("recipient not preserved")
				}
				sid := "SM" + strings.Repeat("c", 32)
				if mode == "sms" {
					if !strings.HasSuffix(r.URL.Path, "/Messages.json") || r.Form.Get("Body") != req.TplContent["content"] || r.Form.Get("MessagingServiceSid") != cfg.MessagingServiceSID || r.Form.Get("From") != "" {
						t.Errorf("invalid SMS form: %v", r.Form)
					}
				} else {
					sid = "CA" + strings.Repeat("c", 32)
					if !strings.HasSuffix(r.URL.Path, "/Calls.json") || r.Form.Get("From") != cfg.From || r.Form.Get("Body") != "" || r.Form.Get("MessagingServiceSid") != "" {
						t.Errorf("invalid call form: %v", r.Form)
					}
					var twiml twilioTwiML
					if err := xml.Unmarshal([]byte(r.Form.Get("Twiml")), &twiml); err != nil {
						t.Fatal(err)
					}
					if twiml.Say.Text != req.TplContent["content"] || twiml.Say.Voice != "woman" || twiml.Say.Language != "fr-FR" || strings.Contains(r.Form.Get("Twiml"), "<Hangup/>") {
						t.Errorf("unsafe or invalid TwiML: %s", r.Form.Get("Twiml"))
					}
				}
				return twilioTestReply(201, `{"sid":"`+sid+`","status":"queued","error_code":null}`), nil
			})}
			res := (&TwilioProvider{}).Notify(context.Background(), req)
			if res.Err != nil || calls != 2 || res.Target != "+8613800138000,+14155552672" || strings.Count(res.Response, "accepted") != 2 {
				t.Fatalf("invalid result: %+v calls=%d", res, calls)
			}
		})
	}
}

func TestTwilioRetryOnlyRateLimit(t *testing.T) {
	for _, status := range []int{429, 400, 401, 500, 0} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			req := twilioReq("sms")
			req.Sendtos = req.Sendtos[:1]
			calls := 0
			req.HttpClient = &http.Client{Transport: twilioTestTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					if status == 0 {
						return nil, errors.New("network error secret-auth-token")
					}
					return twilioTestReply(status, `{"code":20003,"message":"secret-auth-token"}`), nil
				}
				return twilioTestReply(201, `{"sid":"SM123","status":"queued"}`), nil
			})}
			res := (&TwilioProvider{}).Notify(context.Background(), req)
			if status == 429 {
				if res.Err != nil || calls != 2 {
					t.Fatalf("429 retry failed: %+v %d", res, calls)
				}
			} else {
				if res.Err == nil || calls != 1 || strings.Contains(res.Err.Error(), "secret-auth-token") {
					t.Fatalf("unsafe retry or leaked token: %+v %d", res, calls)
				}
			}
		})
	}
}

func TestTwilioPreflightAndPartialFailures(t *testing.T) {
	req := twilioReq("sms")
	calls := 0
	req.HttpClient = &http.Client{Transport: twilioTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return twilioTestReply(400, `{"code":21211,"message":"invalid recipient"}`), nil
		}
		return twilioTestReply(201, `{"sid":"SM123","status":"queued"}`), nil
	})}
	res := (&TwilioProvider{}).Notify(context.Background(), req)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "21211") || !strings.Contains(res.Response, "+14155552672: accepted") || calls != 2 {
		t.Fatalf("lost partial results: %+v calls=%d", res, calls)
	}
	calls = 1
	req.Sendtos = []string{"13800138000", "+14155552671", " "}
	res = (&TwilioProvider{}).Notify(context.Background(), req)
	if res.Err == nil || !strings.Contains(res.Err.Error(), "13800138000") || !strings.Contains(res.Response, "+14155552671: accepted") || calls != 2 || res.Target != "13800138000,+14155552671" {
		t.Fatalf("a malformed recipient must not block valid ones: %+v calls=%d", res, calls)
	}
	calls = 0
	req.Sendtos = []string{"13800138000"}
	if res := (&TwilioProvider{}).Notify(context.Background(), req); res.Err == nil || calls != 0 || res.Target != "13800138000" {
		t.Fatalf("no valid recipient must fail without sending: %+v", res)
	}
	req.Sendtos = nil
	if res := (&TwilioProvider{}).Notify(context.Background(), req); res.Err == nil {
		t.Fatal("empty recipients accepted")
	}
	req.Sendtos = []string{"+14155552671"}
	req.TplContent["content"] = strings.Repeat("界", 1601)
	if res := (&TwilioProvider{}).Notify(context.Background(), req); res.Err == nil || calls != 0 {
		t.Fatal("oversized SMS sent")
	}
	for _, body := range []string{"not json", `{}`, `{"sid":"SM123","status":"failed","error_code":30001,"error_message":"failed"}`} {
		if _, err := checkTwilioResponse(&nativeResponse{StatusCode: 201, Body: []byte(body)}); err == nil {
			t.Fatalf("invalid response accepted: %s", body)
		}
	}
}

func TestTeamsTwilioRegistryNetworkAndTemplates(t *testing.T) {
	for _, rt := range []string{models.Teams, models.Twilio} {
		nc := &models.NotifyChannelConfig{Name: rt, Ident: rt, RequestType: rt}
		if rt == models.Twilio {
			nc = twilioReq("voice").Config
		}
		if err := nc.Verify(); err != nil {
			t.Fatal(err)
		}
		if p, ok := DefaultRegistry.Resolve(nc); !ok || p.Ident() != rt {
			t.Fatalf("missing native provider %s", rt)
		}
		legacy := &models.NotifyChannelConfig{Ident: rt, RequestType: "http", RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://example.com", Method: "POST"}}}
		if p, ok := DefaultRegistry.Resolve(legacy); !ok || p.Ident() != "callback" {
			t.Fatalf("legacy route changed: %s", rt)
		}
		nc.RequestConfig = &models.RequestConfig{TeamsRequestConfig: &models.TeamsRequestConfig{NativeNetworkConfig: models.NativeNetworkConfig{Timeout: 321}}, TwilioRequestConfig: &models.TwilioRequestConfig{NativeNetworkConfig: models.NativeNetworkConfig{Timeout: 321}}}
		client, err := models.GetHTTPClient(nc)
		if err != nil || client.Timeout != 321*time.Millisecond {
			t.Fatalf("network configuration ignored: %v", err)
		}
		tpl := &models.MessageTemplate{NotifyChannelIdent: rt, Content: map[string]string{"content": models.NewTplMap[rt]}}
		for _, recovered := range []bool{false, true} {
			got := tpl.RenderEventForChannel(rt, []*models.AlertCurEvent{{RuleName: "cpu > 90% & hot", RuleNote: "cpu > 90% & hot", Severity: 1, IsRecovered: recovered}}, "http://n9e")["content"].(string)
			if !strings.Contains(got, "cpu > 90% & hot") || strings.Contains(got, "&gt;") {
				t.Fatalf("native template double escaped: %s", got)
			}
			want := "Triggered"
			if recovered {
				want = "Recovered"
			}
			if rt == models.Twilio && !strings.Contains(got, want) {
				t.Fatalf("missing state: %s", got)
			}
		}
	}
}

func TestTwilioConfigValidationAndCancellation(t *testing.T) {
	p := &TwilioProvider{}
	for _, mutate := range []func(*models.TwilioRequestConfig){
		func(c *models.TwilioRequestConfig) { c.AccountSID = "bad" },
		func(c *models.TwilioRequestConfig) { c.AuthToken = "" },
		func(c *models.TwilioRequestConfig) { c.Mode = "fax" },
		func(c *models.TwilioRequestConfig) { c.From = "" },
		func(c *models.TwilioRequestConfig) { c.MessagingServiceSID = "bad" },
		func(c *models.TwilioRequestConfig) { c.Mode = "voice"; c.From = "SenderID" },
		func(c *models.TwilioRequestConfig) { c.RetryTimes = -1 },
	} {
		req := twilioReq("sms")
		mutate(req.Config.RequestConfig.TwilioRequestConfig)
		if err := p.Check(req.Config); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	req := twilioReq("sms")
	cfg := req.Config.RequestConfig.TwilioRequestConfig
	cfg.AccountSID = "{{.account_sid}}"
	cfg.AuthToken = "{{.auth_token}}"
	cfg.From = "{{.from}}"
	if err := p.Check(req.Config); err != nil {
		t.Fatalf("variable references rejected before send: %v", err)
	}
	req = twilioReq("sms")
	req.Sendtos = req.Sendtos[:1]
	calls := 0
	ctx, cancel := context.WithCancel(context.Background())
	req.HttpClient = &http.Client{Transport: twilioTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		cancel()
		resp := twilioTestReply(429, `{"message":"rate limited"}`)
		resp.Header.Set("Retry-After", "30")
		return resp, nil
	})}
	if res := p.Notify(ctx, req); res.Err == nil || calls != 1 {
		t.Fatalf("cancelled retry did not stop: %+v, %d", res, calls)
	}
	req.Config.RequestConfig.TwilioRequestConfig.RetryTimes = 0
	calls = 0
	if res := p.Notify(context.Background(), req); res.Err == nil || calls != 1 {
		t.Fatalf("zero retries not honored: %+v, %d", res, calls)
	}
}

func TestTeamsExplicitZeroRetries(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusTooManyRequests) }))
	defer srv.Close()
	req := webhookReq(models.Teams, &models.RequestConfig{TeamsRequestConfig: &models.TeamsRequestConfig{}}, map[string]string{"webhook_url": srv.URL + "/workflow"})
	if res := (&TeamsProvider{}).Notify(context.Background(), req); res.Err == nil || calls != 1 {
		t.Fatalf("zero retries not honored: %+v, %d", res, calls)
	}
}

func TestTwilioIgnoresInactiveModeVariables(t *testing.T) {
	for _, mode := range []string{"sms", "voice"} {
		cfg := *twilioReq(mode).Config.RequestConfig.TwilioRequestConfig
		if mode == "sms" {
			cfg.Voice = "{{.removed_voice}}"
			cfg.Language = "{{.removed_language}}"
		} else {
			cfg.MessagingServiceSID = "{{.removed_service}}"
		}
		if _, err := expandTwilioConfig(cfg); err != nil {
			t.Fatalf("unused %s settings blocked sending: %v", mode, err)
		}
	}
}
