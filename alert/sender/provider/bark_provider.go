package provider

// Bark media type: pushes to the Bark iOS app through a bark-server. The server and its basic
// auth live in the media type (empty means the public server api.day.app). A device key belongs
// to one iPhone, so it is read from each recipient's contacts and one push is sent per key.
//
// Wire format follows bark-server's route_push.go (https://github.com/Finb/bark-server):
// POST {server}/push with a JSON body; success is HTTP 200 {"code":200,"message":"success"},
// failures carry the same status in the HTTP line and in "code". A wrong basic auth answers 418
// with a plain-text body, an APNs rejection answers 500 "push failed: <reason>".

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
)

const (
	barkDefaultServerURL = "https://api.day.app"

	barkMaxTitleRunes  = 120
	barkMaxOptionRunes = 64
	// APNs rejects payloads over 4096 bytes; keep headroom for keys bark-server adds on its own
	barkMaxPayloadBytes = 3900
	barkMaxConcurrency  = 5

	// notification_record.target is varchar(1024) and details varchar(2048): an over-long value
	// fails the whole batch insert on PostgreSQL or strict MySQL
	barkMaxTargetBytes = 1000
	barkMaxErrorBytes  = 1900
	barkMaxListBytes   = 300
	barkMaxReasonRunes = 400
)

type BarkProvider struct{}

func (p *BarkProvider) Ident() string { return models.RequestTypeBark }

func (p *BarkProvider) Check(config *models.NotifyChannelConfig) error {
	if config == nil || config.RequestType != models.RequestTypeBark {
		return errors.New("bark provider requires request_type: bark")
	}
	if config.ParamConfig == nil || config.ParamConfig.UserInfo == nil || strings.TrimSpace(config.ParamConfig.UserInfo.ContactKey) == "" {
		return errors.New("bark needs a contact key: choose the contact that holds each user's Bark device key")
	}
	// The request config may be empty: public server, no auth
	if config.RequestConfig == nil || config.RequestConfig.BarkRequestConfig == nil {
		return nil
	}
	c := config.RequestConfig.BarkRequestConfig
	if err := checkAPIURL("bark server", c.ServerURL); err != nil {
		return err
	}
	if (c.Username == "") != (c.Password == "") {
		return errors.New("bark username and password must be set together")
	}
	return checkNativeRetry("bark", &c.NativeNetworkConfig)
}

type barkMessage struct {
	DeviceKey string `json:"device_key"`
	Title     string `json:"title,omitempty"`
	Body      string `json:"body"`
	Level     string `json:"level"`
	Sound     string `json:"sound,omitempty"`
	Group     string `json:"group,omitempty"`
	URL       string `json:"url,omitempty"`
}

// barkLevel is fixed on purpose: S1 breaks through Focus, S2/S3 are normal pushes and recoveries
// arrive silently. critical is never used: it rings even when the phone is muted.
func barkLevel(event *models.AlertCurEvent) string {
	switch {
	case event.IsRecovered:
		return "passive"
	case event.Severity == 1:
		return "timeSensitive"
	default:
		return "active"
	}
}

func (p *BarkProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req == nil {
		return &NotifyResult{Err: errors.New("bark notify request is required")}
	}
	if err := p.Check(req.Config); err != nil {
		return &NotifyResult{Err: err}
	}
	if len(req.Events) == 0 {
		return &NotifyResult{Err: errors.New("events is empty")}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var cfg models.BarkRequestConfig
	if req.Config.RequestConfig != nil && req.Config.RequestConfig.BarkRequestConfig != nil {
		cfg = *req.Config.RequestConfig.BarkRequestConfig
	}
	// Basic auth credentials are used as entered: bark-server compares them byte for byte
	cfg.ServerURL = strings.TrimSpace(cfg.ServerURL)
	for _, field := range []*string{&cfg.ServerURL, &cfg.Username, &cfg.Password} {
		v, err := expandUserVars(*field)
		if err != nil {
			return &NotifyResult{Err: err}
		}
		*field = v
	}
	server := strings.TrimRight(cfg.ServerURL, "/")
	if server == "" {
		server = barkDefaultServerURL
	}
	if u, err := url.Parse(server); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return &NotifyResult{Err: errors.New("bark server url must start with http:// or https://")}
	}

	// Device keys are push credentials: records and errors only show a masked form.
	// One bad key or missing contact must not keep the rest of the team from being notified.
	var keys, targets, invalid []string
	seen := map[string]bool{}
	for _, raw := range req.Sendtos {
		key := strings.TrimSpace(raw)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, barkMaskKey(key))
		// Users tend to paste the whole URL the Bark app shows instead of the key
		if strings.ContainsAny(key, "/ \t\r\n") {
			invalid = append(invalid, barkMaskKey(key))
			continue
		}
		keys = append(keys, key)
	}
	var failures []error
	if len(req.MissingContactUsers) > 0 {
		failures = append(failures, withHint(fmt.Errorf("no Bark device key in the profile of: %s", barkJoinLimited(req.MissingContactUsers, barkMaxListBytes)),
			"Each recipient fills in their Bark device key in their profile, under Contacts"))
	}
	if len(invalid) > 0 {
		failures = append(failures, withHint(fmt.Errorf("invalid Bark device key: %s", barkJoinLimited(invalid, barkMaxListBytes)),
			"Fill in only the device key: the part right after the server address in the Bark app's URL, such as https://api.day.app/<device key>/"))
	}
	target := barkJoinLimited(targets, barkMaxTargetBytes)
	if len(keys) == 0 {
		if len(failures) == 0 {
			failures = append(failures, withHint(errors.New("no recipient with a Bark device key"),
				"Select users or teams in the notify rule, and have each of them fill in their Bark device key in their profile"))
		}
		return &NotifyResult{Target: target, Err: barkJoinWithin(failures, barkMaxErrorBytes)}
	}

	event := req.Events[0]
	title := firstEventTitle(req, event, barkMaxTitleRunes)
	body := tplString(req.TplContent, "content")
	if body == "" {
		// bark-server would show "Empty Message"
		body = title
	}
	msg := barkMessage{Title: title, Body: body, Level: barkLevel(event), URL: eventDetailURL(req.SiteUrl, event)}
	msg.Sound, _ = truncateInRunes(strings.TrimSpace(cfg.Sound), barkMaxOptionRunes)
	msg.Group, _ = truncateInRunes(strings.TrimSpace(cfg.Group), barkMaxOptionRunes)
	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)

	// Each key waits out its own timeout and retries when the server is down, so send a few at a
	// time instead of one after another; errs keeps the outcome of keys[i] at index i
	errs := make([]error, len(keys))
	sem := make(chan struct{}, barkMaxConcurrency)
	var wg sync.WaitGroup
	for i, key := range keys {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				errs[i] = ctx.Err()
				return
			}
			m := msg
			m.DeviceKey = key
			fitBarkBody(&m)
			payload, err := json.Marshal(m)
			if err != nil {
				errs[i] = err
				return
			}
			_, errs[i] = doWithRetry(ctx, req.HttpClient, retryTimes, sleep,
				func(ctx context.Context) (*http.Request, error) {
					r, err := http.NewRequestWithContext(ctx, http.MethodPost, server+"/push", bytes.NewReader(payload))
					if err != nil {
						return nil, err
					}
					r.Header.Set("Content-Type", "application/json; charset=utf-8")
					if cfg.Username != "" {
						r.SetBasicAuth(cfg.Username, cfg.Password)
					}
					return r, nil
				}, checkBark)
		}(i, key)
	}
	wg.Wait()

	// Keys failing for the same reason (e.g. one wrong server for everybody) share one line
	var sent []string
	var groups []string
	byReason := map[string]*barkFailure{}
	for i, err := range errs {
		if err == nil {
			sent = append(sent, barkMaskKey(keys[i]))
			continue
		}
		reason, _ := truncateInRunes(redactBarkKeys(err.Error(), cfg.Password, keys), barkMaxReasonRunes)
		hint := barkHint(err)
		id := reason + "\x00" + hint
		if byReason[id] == nil {
			byReason[id] = &barkFailure{reason: reason, hint: hint}
			groups = append(groups, id)
		}
		byReason[id].keys = append(byReason[id].keys, barkMaskKey(keys[i]))
	}
	for _, id := range groups {
		f := byReason[id]
		failures = append(failures, withHint(fmt.Errorf("%s: %s", barkJoinLimited(f.keys, barkMaxListBytes), f.reason), f.hint))
	}
	response := ""
	if len(sent) > 0 {
		response = "sent: " + barkJoinLimited(sent, barkMaxListBytes)
		if len(failures) > 0 {
			// A failed notification keeps only its error in the record and the test result:
			// say who did get the push, or a partial failure reads as nobody being notified
			failures = append(failures, errors.New(response))
		}
	}
	return &NotifyResult{Target: target, Response: response, Err: barkJoinWithin(failures, barkMaxErrorBytes)}
}

type barkFailure struct {
	reason, hint string
	keys         []string
}

// barkAPNsSize estimates the payload bark-server sends to APNs for a push (apns/apns.go): the alert
// title and body, the sound with ".caf", the group as thread-id, a fixed category, and every other
// key as a custom string. JSON escaping is counted: quotes and backslashes double in size.
func barkAPNsSize(m barkMessage) int {
	b, _ := json.Marshal(map[string]interface{}{
		"aps": map[string]interface{}{
			"alert":     map[string]string{"title": m.Title, "body": m.Body},
			"sound":     m.Sound + ".caf",
			"thread-id": m.Group,
			"category":  "myNotificationCategory",
		},
		"level":      m.Level,
		"url":        m.URL,
		"device_key": m.DeviceKey,
	})
	return len(b)
}

// fitBarkBody shortens the body on a character boundary until the APNs payload fits; Apple
// rejects larger payloads and a retry with the same content cannot succeed
func fitBarkBody(m *barkMessage) {
	if barkAPNsSize(*m) <= barkMaxPayloadBytes {
		return
	}
	runes := []rune(m.Body)
	cut := func(n int) string { return string(runes[:n]) + "…" }
	lo, hi := 0, len(runes)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		m.Body = cut(mid)
		if barkAPNsSize(*m) <= barkMaxPayloadBytes {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	m.Body = cut(lo)
}

// barkMaskKey keeps the last 4 characters of a key only when enough of it stays hidden;
// self-hosted servers accept short and non-ASCII custom keys, so it counts runes, not bytes
func barkMaskKey(key string) string {
	r := []rune(key)
	if len(r) < 12 {
		return redactedMark
	}
	return redactedMark + string(r[len(r)-4:])
}

// redactBarkKeys removes the password and device keys from an error text. redactSecrets skips
// values shorter than 6 characters, so keys are also replaced wherever they stand as a whole token,
// such as the "[key]" bark-server echoes for an unknown key.
func redactBarkKeys(msg, password string, keys []string) string {
	msg = redactSecrets(msg, append([]string{password}, keys...)...)
	for _, k := range keys {
		msg = replaceToken(msg, k, redactedMark)
	}
	return msg
}

// replaceToken replaces old in s where it is not part of a longer key-like word
func replaceToken(s, old, repl string) string {
	isKeyChar := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' }
	var b strings.Builder
	for {
		i := strings.Index(s, old)
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		after, _ := utf8.DecodeRuneInString(s[i+len(old):])
		b.WriteString(s[:i])
		if (i == 0 || !isKeyChar(before)) && (i+len(old) == len(s) || !isKeyChar(after)) {
			b.WriteString(repl)
		} else {
			b.WriteString(old)
		}
		s = s[i+len(old):]
	}
}

// barkJoinLimited joins items with ", " within max bytes; the items that do not fit become "+N more"
func barkJoinLimited(items []string, max int) string {
	out := ""
	for i, it := range items {
		next := it
		if out != "" {
			next = out + ", " + it
		}
		suffix := ""
		if rest := len(items) - i - 1; rest > 0 {
			suffix = fmt.Sprintf(" +%d more", rest)
		}
		if len(next)+len(suffix) > max {
			return strings.TrimSpace(out + fmt.Sprintf(" +%d more", len(items)-i))
		}
		out = next
	}
	return out
}

// barkJoinWithin joins the errors while the text fits the notification record, dropping the last
// ones first: the reasons come before the summary of who was pushed
func barkJoinWithin(errs []error, max int) error {
	for n := len(errs); n > 1; n-- {
		kept := append([]error(nil), errs[:n]...)
		if n < len(errs) {
			kept = append(kept, fmt.Errorf("+%d more not shown", len(errs)-n))
		}
		if err := errors.Join(kept...); len(err.Error()) <= max {
			return err
		}
	}
	if len(errs) > 1 {
		return errors.Join(errs[0], fmt.Errorf("+%d more not shown", len(errs)-1))
	}
	return errors.Join(errs...)
}

type barkResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// barkDeadToken: APNs reasons meaning the device token behind the key will never work again
func barkDeadToken(detail string) bool {
	for _, reason := range []string{"BadDeviceToken", "Unregistered", "DeviceTokenNotForTopic"} {
		if strings.Contains(detail, reason) {
			return true
		}
	}
	return false
}

// checkBark adds two cases to the default check: a 2xx without {"code":200} means the URL is not
// a bark-server (e.g. a website answering every path), and a dead device token is not retried.
func checkBark(r *nativeResponse) (bool, error) {
	var resp barkResponse
	_ = json.Unmarshal(r.Body, &resp)
	if r.StatusCode/100 == 2 {
		if resp.Code != http.StatusOK {
			return false, &HTTPStatusError{StatusCode: r.StatusCode, Detail: "not a Bark server response: " + truncateBody(r.Body)}
		}
		return false, nil
	}
	retry, err := checkStatus(r, func(*nativeResponse) string {
		if resp.Message == "" {
			return ""
		}
		return fmt.Sprintf("%d %s", resp.Code, resp.Message)
	})
	if retry && barkDeadToken(resp.Message) {
		retry = false
	}
	return retry, err
}

func barkHint(err error) string {
	var se *HTTPStatusError
	if !errors.As(err, &se) {
		if strings.Contains(err.Error(), "x509") || strings.Contains(err.Error(), "certificate") {
			return "The Bark server uses a certificate Nightingale does not trust; enable skip certificate verification in the media type's advanced settings"
		}
		return "Nightingale cannot reach the Bark server; check the server URL and the network, or set a proxy in the media type"
	}
	switch {
	case se.StatusCode == http.StatusBadRequest && strings.Contains(se.Detail, "device token"):
		return "The device key is wrong or not registered on this Bark server; copy it again from the Bark app"
	case se.StatusCode == http.StatusTooManyRequests:
		return "Rate limited by the Bark server, retried automatically; try again later"
	case se.StatusCode == http.StatusUnauthorized || se.StatusCode == http.StatusTeapot:
		return "The Bark server requires authentication; check the username and password in the media type"
	case se.StatusCode/100 == 2 || se.StatusCode == http.StatusNotFound || se.StatusCode == http.StatusMethodNotAllowed:
		return "The server URL does not point at a Bark server; fill in the server root address, e.g. https://api.day.app"
	case se.StatusCode/100 == 5 && barkDeadToken(se.Detail):
		return "Apple no longer accepts this device key; open the Bark app on the iPhone and copy the key again"
	case se.StatusCode/100 == 5 && strings.Contains(se.Detail, "push failed"):
		return "The Bark server failed to deliver the push to Apple; it was retried, try again later"
	case se.StatusCode/100 == 5:
		return "The Bark server returned a server error; it was retried, check the server URL or try again later"
	}
	return ""
}
