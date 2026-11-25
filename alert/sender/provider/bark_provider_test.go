package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
)

const (
	testBarkKeyAlice = "NxXwPkTGq7sUUGDxbSJ2bV"
	testBarkKeyCarol = "Ab3dE6fG9hJ2kL5mN8pQrS"
)

func barkReq(server string, mutate func(*models.BarkRequestConfig), sendtos ...string) *NotifyRequest {
	cfg := &models.BarkRequestConfig{ServerURL: server, NativeNetworkConfig: fastRetry()}
	if mutate != nil {
		mutate(cfg)
	}
	req := webhookReq(models.RequestTypeBark, &models.RequestConfig{BarkRequestConfig: cfg}, map[string]string{})
	req.Config.ParamConfig = &models.NotifyParamConfig{UserInfo: &models.UserInfo{ContactKey: models.BarkKey}}
	req.Sendtos = sendtos
	return req
}

func barkBody(t *testing.T, r capturedRequest) barkMessage {
	t.Helper()
	var m barkMessage
	if err := json.Unmarshal(r.Body, &m); err != nil {
		t.Fatalf("request body is not JSON: %v: %s", err, r.Body)
	}
	return m
}

const barkOK = `{"code":200,"message":"success","timestamp":1700000000}`

func TestBarkResolvesWithEmptyConfig(t *testing.T) {
	nc := &models.NotifyChannelConfig{Ident: models.Bark, RequestType: models.RequestTypeBark,
		ParamConfig: &models.NotifyParamConfig{UserInfo: &models.UserInfo{ContactKey: models.BarkKey}}}
	if got, ok := DefaultRegistry.Resolve(nc); !ok || got.Ident() != models.RequestTypeBark {
		t.Fatalf("bark should resolve to its provider, got %v", got)
	}
	if _, err := models.GetHTTPClient(nc); err != nil {
		t.Fatalf("GetHTTPClient must accept an empty bark config: %v", err)
	}
	// Without a contact key nobody can be reached: saving must say so
	nc.ParamConfig = nil
	if err := VerifyChannelConfig(nc); err == nil || !strings.Contains(err.Error(), "contact key") {
		t.Fatalf("bark without a contact key must not pass verification: %v", err)
	}
	legacy := &models.NotifyChannelConfig{Ident: models.Bark, RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://x", Method: "POST"}}}
	if got, ok := DefaultRegistry.Resolve(legacy); !ok || got.Ident() != "callback" {
		t.Fatalf("a custom http channel named bark should fall back to callback, got %v", got)
	}
}

func TestBarkPushesOncePerRecipient(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, barkOK))
	req := barkReq(srv.URL+"/", func(c *models.BarkRequestConfig) {
		c.Username, c.Password, c.Sound, c.Group = "n9e", "pass-word-123", "alarm", "Nightingale"
	}, testBarkKeyAlice, " "+testBarkKeyAlice+" ", testBarkKeyCarol, "")
	req.TplContent["content"] = "Target: db-01\nValue: 92 > 90 & \"hot\""

	res := (&BarkProvider{}).Notify(context.Background(), req)
	if res.Err != nil || res.Target != "***J2bV, ***pQrS" || res.Response != "sent: ***J2bV, ***pQrS" {
		t.Fatalf("unexpected result: %+v", res)
	}
	noLeak(t, res, testBarkKeyAlice, testBarkKeyCarol, "pass-word-123")

	calls := got()
	if len(calls) != 2 {
		t.Fatalf("duplicate and empty keys must be skipped, got %d pushes", len(calls))
	}
	// Keys are pushed concurrently: the server may see them in any order
	if barkBody(t, calls[0]).DeviceKey != testBarkKeyAlice {
		calls[0], calls[1] = calls[1], calls[0]
	}
	r := calls[0]
	if r.Method != "POST" || r.Path != "/push" || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		t.Fatalf("unexpected request: %+v", r)
	}
	if user, pass, ok := (&http.Request{Header: r.Header}).BasicAuth(); !ok || user != "n9e" || pass != "pass-word-123" {
		t.Fatalf("basic auth missing: %v", r.Header)
	}
	m := barkBody(t, r)
	want := barkMessage{DeviceKey: testBarkKeyAlice, Title: "[S1] Triggered: cpu <high> & hot", Body: req.TplContent["content"].(string),
		Level: "timeSensitive", Sound: "alarm", Group: "Nightingale", URL: "http://n9e.example.com/share/alert-his-events/42"}
	if m != want {
		t.Fatalf("unexpected push:\n got %+v\nwant %+v", m, want)
	}
	if barkBody(t, calls[1]).DeviceKey != testBarkKeyCarol {
		t.Fatalf("second push must go to the second key: %s", calls[1].Body)
	}

	// Empty config: no auth header, optional keys left out
	srv, got = newCaptureServer(t, always(200, barkOK))
	if res := (&BarkProvider{}).Notify(context.Background(), barkReq(srv.URL, nil, testBarkKeyAlice)); res.Err != nil {
		t.Fatal(res.Err)
	}
	r = got()[0]
	if r.Header.Get("Authorization") != "" || strings.Contains(string(r.Body), `"sound"`) || strings.Contains(string(r.Body), `"group"`) {
		t.Fatalf("empty config must not send auth, sound or group: %v %s", r.Header, r.Body)
	}
}

func TestBarkFixedLevels(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, barkOK))
	cases := []struct {
		severity  int
		recovered bool
		want      string
	}{{1, false, "timeSensitive"}, {2, false, "active"}, {3, false, "active"}, {1, true, "passive"}, {3, true, "passive"}}
	for i, c := range cases {
		req := barkReq(srv.URL, nil, testBarkKeyAlice)
		req.Events[0].Severity, req.Events[0].IsRecovered = c.severity, c.recovered
		if res := (&BarkProvider{}).Notify(context.Background(), req); res.Err != nil {
			t.Fatal(res.Err)
		}
		m := barkBody(t, got()[i])
		if m.Level != c.want {
			t.Fatalf("S%d recovered=%v must use level %s, got %s", c.severity, c.recovered, c.want, m.Level)
		}
		if c.recovered && !strings.Contains(m.Title, "Recovered") {
			t.Fatalf("recovery title must say so: %q", m.Title)
		}
	}
}

// Quotes and backslashes double when escaped: the body must be cut by the size of the final
// APNs payload bark-server builds, not by its own length
func TestBarkPayloadFitsAPNsLimit(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, barkOK))
	bodies := []string{strings.Repeat(`"`, 2500), strings.Repeat(`\`, 2500), strings.Repeat("告", 2000), strings.Repeat(`"\告<&>`, 600)}
	for i, body := range bodies {
		req := barkReq(srv.URL, func(c *models.BarkRequestConfig) { c.Group = strings.Repeat(`"g`, 200) }, testBarkKeyAlice)
		req.TplContent["content"] = body
		if res := (&BarkProvider{}).Notify(context.Background(), req); res.Err != nil {
			t.Fatal(res.Err)
		}
		m := barkBody(t, got()[i])
		if size := barkAPNsSize(m); size > 4096 || !utf8.ValidString(m.Body) || !strings.HasSuffix(m.Body, "…") {
			t.Fatalf("body #%d: APNs payload %d bytes, valid=%v", i, size, utf8.ValidString(m.Body))
		}
		if len([]rune(m.Group)) > barkMaxOptionRunes {
			t.Fatalf("group must be capped: %d runes", len([]rune(m.Group)))
		}
		// not cut more than needed
		if barkAPNsSize(m) < barkMaxPayloadBytes-20 {
			t.Fatalf("body #%d cut too short: %d bytes", i, barkAPNsSize(m))
		}
	}

	req := barkReq(srv.URL, nil, testBarkKeyAlice)
	req.TplContent["content"] = "  "
	if res := (&BarkProvider{}).Notify(context.Background(), req); res.Err != nil {
		t.Fatal(res.Err)
	}
	if m := barkBody(t, got()[len(bodies)]); m.Body != m.Title || m.Body == "" {
		t.Fatalf("an empty body must fall back to the title: %+v", m)
	}
}

// One bad recipient must not keep the others from being notified, and the result names who was skipped
func TestBarkReportsSkippedRecipients(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, barkOK))
	req := barkReq(srv.URL, nil, testBarkKeyAlice, "https://api.day.app/"+testBarkKeyCarol+"/")
	req.MissingContactUsers = []string{"bob", "dave"}
	res := (&BarkProvider{}).Notify(context.Background(), req)
	if len(got()) != 1 || res.Response != "sent: ***J2bV" {
		t.Fatalf("the valid key must still be pushed: %+v calls=%d", res, len(got()))
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "no Bark device key in the profile of: bob, dave") ||
		!strings.Contains(res.Err.Error(), "invalid Bark device key") {
		t.Fatalf("skipped recipients must be reported: %v", res.Err)
	}
	// The record keeps only the error of a failed notification: it must still say who got the push,
	// after the reasons so that a cut never drops them
	if !strings.HasPrefix(res.Err.Error(), "no Bark device key") || !strings.HasSuffix(res.Err.Error(), "\nsent: ***J2bV") {
		t.Fatalf("a partial failure must list the reasons, then the delivered recipients: %v", res.Err)
	}
	noLeak(t, res, testBarkKeyAlice, testBarkKeyCarol)

	// Every hint in the joined error is translated, not only the first one
	msg := LocalizeError(res.Err, func(k string) string { return "TRANSLATED" })
	if strings.Count(msg, "(TRANSLATED)") != 2 {
		t.Fatalf("both hints must be localized: %s", msg)
	}

	// Nobody has a key: nothing is sent and the error says why
	srv, got = newCaptureServer(t, always(200, barkOK))
	req = barkReq(srv.URL, nil)
	req.MissingContactUsers = []string{"bob"}
	res = (&BarkProvider{}).Notify(context.Background(), req)
	if res.Err == nil || len(got()) != 0 || !strings.Contains(res.Err.Error(), "bob") {
		t.Fatalf("no recipient must fail without pushing: %+v", res)
	}
	res = (&BarkProvider{}).Notify(context.Background(), barkReq(srv.URL, nil))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "no recipient with a Bark device key") {
		t.Fatalf("an empty recipient list must fail: %+v", res)
	}
}

func TestBarkErrorsAndRetry(t *testing.T) {
	p := &BarkProvider{}

	// bark-server echoes the unknown key in its error: no retry, hinted, key redacted
	srv, got := newCaptureServer(t, always(400, `{"code":400,"message":"failed to get device token: failed to get [`+testBarkKeyAlice+`] device token from database","timestamp":1}`))
	res := p.Notify(context.Background(), barkReq(srv.URL, nil, testBarkKeyAlice))
	if res.Err == nil || len(got()) != 1 || !strings.Contains(res.Err.Error(), "copy it again from the Bark app") {
		t.Fatalf("unknown key must fail once with a hint: %+v calls=%d", res, len(got()))
	}
	noLeak(t, res, testBarkKeyAlice)

	// A transient APNs failure is retried
	srv, got = newCaptureServer(t, func(n int, _ capturedRequest) (int, string) {
		if n == 1 {
			return 500, `{"code":500,"message":"push failed: Post \"https://api.push.apple.com/3/device/x\": EOF","timestamp":1}`
		}
		return 200, barkOK
	})
	if res := p.Notify(context.Background(), barkReq(srv.URL, nil, testBarkKeyAlice)); res.Err != nil || len(got()) != 2 {
		t.Fatalf("5xx should be retried: %+v calls=%d", res, len(got()))
	}
	// A 5xx that is not a bark-server push failure must not blame Apple
	srv, _ = newCaptureServer(t, always(502, "<html>Bad Gateway</html>"))
	res = p.Notify(context.Background(), barkReq(srv.URL, nil, testBarkKeyAlice))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "returned a server error") {
		t.Fatalf("expected a generic server error hint: %+v", res)
	}
	// A dead device token is not
	srv, got = newCaptureServer(t, always(500, `{"code":500,"message":"push failed: BadDeviceToken","timestamp":1}`))
	res = p.Notify(context.Background(), barkReq(srv.URL, nil, testBarkKeyAlice))
	if res.Err == nil || len(got()) != 1 || !strings.Contains(res.Err.Error(), "Apple no longer accepts this device key") {
		t.Fatalf("a dead token must fail without retry: %+v calls=%d", res, len(got()))
	}

	// Wrong basic auth: bark-server answers 418 in plain text
	srv, _ = newCaptureServer(t, always(418, "I'm a teapot"))
	res = p.Notify(context.Background(), barkReq(srv.URL, func(c *models.BarkRequestConfig) { c.Username, c.Password = "n9e", "wrong-password" }, testBarkKeyAlice))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "check the username and password") {
		t.Fatalf("expected an auth hint: %+v", res)
	}
	noLeak(t, res, "wrong-password")

	// A website answering 200 to everything is not a Bark server
	srv, _ = newCaptureServer(t, always(200, "<html>welcome</html>"))
	res = p.Notify(context.Background(), barkReq(srv.URL, nil, testBarkKeyAlice))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "does not point at a Bark server") {
		t.Fatalf("a non-bark 200 must fail: %+v", res)
	}

	res = p.Notify(context.Background(), barkReq("http://127.0.0.1:1", nil, testBarkKeyAlice))
	var he *HintError
	if res.Err == nil || !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "cannot reach the Bark server") {
		t.Fatalf("expected an unreachable hint: %+v", res)
	}

	bad := []func(*models.BarkRequestConfig){
		func(c *models.BarkRequestConfig) { c.ServerURL = "api.day.app" },
		func(c *models.BarkRequestConfig) { c.Username = "only-user" },
		func(c *models.BarkRequestConfig) { c.RetryTimes = -1 },
	}
	for i, m := range bad {
		if err := p.Check(barkReq("", m).Config); err == nil {
			t.Fatalf("invalid config #%d passed", i)
		}
	}
}

func TestBarkDeviceKeysAreRedactedInLogs(t *testing.T) {
	got := RedactSendtosForLog(models.RequestTypeBark, []string{testBarkKeyAlice})
	if len(got) != 1 || got[0] != maskAPIKey(testBarkKeyAlice) {
		t.Fatalf("bark keys must be masked in logs: %v", got)
	}
	if got := RedactSendtosForLog(models.RequestTypeTwilio, []string{"+14155552671"}); got[0] != "+14155552671" {
		t.Fatalf("other channels are logged as before: %v", got)
	}
}

func TestBarkRateLimitHintAndBasicAuthKeepsSpaces(t *testing.T) {
	srv, _ := newCaptureServer(t, always(429, "Too Many Requests"))
	res := (&BarkProvider{}).Notify(context.Background(), barkReq(srv.URL, nil, testBarkKeyAlice))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "Rate limited by the Bark server") {
		t.Fatalf("expected a rate limit hint: %+v", res)
	}

	// bark-server compares the configured credentials byte for byte
	srv, got := newCaptureServer(t, always(200, barkOK))
	req := barkReq(srv.URL, func(c *models.BarkRequestConfig) { c.Username, c.Password = " n9e", " secret " }, testBarkKeyAlice)
	if res := (&BarkProvider{}).Notify(context.Background(), req); res.Err != nil {
		t.Fatal(res.Err)
	}
	if user, pass, ok := (&http.Request{Header: got()[0].Header}).BasicAuth(); !ok || user != " n9e" || pass != " secret " {
		t.Fatalf("credentials must be sent as entered: %q %q", user, pass)
	}
}

// Self-hosted servers accept short and non-ASCII custom keys: masks must stay valid UTF-8 and
// a short key echoed back by the server must not survive in the error
func TestBarkShortAndNonASCIIKeysAreMasked(t *testing.T) {
	srv, _ := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
		var m barkMessage
		json.Unmarshal(r.Body, &m)
		return 400, `{"code":400,"message":"failed to get device token: failed to get [` + m.DeviceKey + `] device token from database"}`
	})
	for _, key := range []string{"a", "ab", "abc", "abcd", "abcde", "abc汉字"} {
		res := (&BarkProvider{}).Notify(context.Background(), barkReq(srv.URL, nil, key))
		text := res.Target + "\n" + res.Err.Error()
		if res.Target != redactedMark || strings.Contains(text, "["+key+"]") || !utf8.ValidString(text) {
			t.Fatalf("key %q leaked or produced invalid UTF-8: %q", key, text)
		}
	}
	long := "自建服务的设备密钥一二三四"
	if got := barkMaskKey(long); got != "***一二三四" || !utf8.ValidString(got) {
		t.Fatalf("long non-ASCII keys keep their last 4 characters: %q", got)
	}
}

func TestBarkSendsConcurrentlyWithinLimit(t *testing.T) {
	var mu sync.Mutex
	inflight, peak := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		inflight++
		if inflight > peak {
			peak = inflight
		}
		mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		mu.Lock()
		inflight--
		mu.Unlock()
		w.Write([]byte(barkOK))
	}))
	t.Cleanup(srv.Close)
	var keys, masks []string
	for i := 0; i < 12; i++ {
		k := fmt.Sprintf("ConcurrentDeviceKey%03d", i)
		keys, masks = append(keys, k), append(masks, barkMaskKey(k))
	}
	start := time.Now()
	res := (&BarkProvider{}).Notify(context.Background(), barkReq(srv.URL, nil, keys...))
	if res.Err != nil || peak < 2 || peak > barkMaxConcurrency {
		t.Fatalf("expected between 2 and %d concurrent pushes, got %d: %v", barkMaxConcurrency, peak, res.Err)
	}
	if time.Since(start) > 400*time.Millisecond {
		t.Fatalf("pushes were not sent concurrently: %v", time.Since(start))
	}
	// results follow the order of the recipients, not of completion
	if res.Response != "sent: "+strings.Join(masks, ", ") {
		t.Fatalf("unstable result order: %s", res.Response)
	}
}

func TestBarkStopsWhenCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	var keys []string
	for i := 0; i < 20; i++ {
		keys = append(keys, fmt.Sprintf("CanceledDeviceKey%03d", i))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	res := (&BarkProvider{}).Notify(ctx, barkReq(srv.URL, nil, keys...))
	if res.Err == nil || time.Since(start) > 1500*time.Millisecond || !strings.Contains(res.Err.Error(), "context") {
		t.Fatalf("a canceled notification must stop quickly: %v after %v", res.Err, time.Since(start))
	}
}

// notification_record.target is varchar(1024) and details varchar(2048)
func TestBarkRecordFieldsStayWithinColumnLimits(t *testing.T) {
	keys := func(n int) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf("BigTeamDeviceKey%04d", i))
		}
		return out
	}
	srv, _ := newCaptureServer(t, always(200, barkOK))
	req := barkReq(srv.URL, nil, keys(130)...)
	req.MissingContactUsers = strings.Fields(strings.Repeat("someone-without-a-key ", 60))
	res := (&BarkProvider{}).Notify(context.Background(), req)
	if len(res.Target) > 1024 || len(res.Response) > 2048 || len(res.Err.Error()) > 2048 || !strings.Contains(res.Target, "more") {
		t.Fatalf("target %d / response %d / error %d bytes", len(res.Target), len(res.Response), len(res.Err.Error()))
	}

	// The same failure for everybody (e.g. a wrong server) is one line, not one per key
	srv, _ = newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
		var m barkMessage
		json.Unmarshal(r.Body, &m)
		return 400, `{"code":400,"message":"failed to get device token: failed to get [` + m.DeviceKey + `] device token from database"}`
	})
	res = (&BarkProvider{}).Notify(context.Background(), barkReq(srv.URL, nil, keys(12)...))
	if lines := strings.Count(res.Err.Error(), "\n") + 1; lines != 1 || len(res.Err.Error()) > 2048 {
		t.Fatalf("identical failures must share one line: %d lines, %d bytes", lines, len(res.Err.Error()))
	}

	// Distinct failures beyond the budget are counted, and the reasons come first
	srv, _ = newCaptureServer(t, func(n int, _ capturedRequest) (int, string) {
		return 400, fmt.Sprintf(`{"code":400,"message":"distinct failure %d %s"}`, n, strings.Repeat("x", 150))
	})
	req = barkReq(srv.URL, func(c *models.BarkRequestConfig) { c.RetryTimes = 1 }, keys(40)...)
	res = (&BarkProvider{}).Notify(context.Background(), req)
	if len(res.Err.Error()) > barkMaxErrorBytes || !strings.Contains(res.Err.Error(), "more not shown") || !strings.Contains(res.Err.Error(), "distinct failure") {
		t.Fatalf("an over-long error must be cut by whole items: %d bytes\n%s", len(res.Err.Error()), res.Err.Error())
	}
}
