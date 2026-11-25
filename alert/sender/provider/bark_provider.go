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
	barkMaxReasonBytes = 600
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
	if err := fitBarkPayload(&msg); err != nil {
		failures = append(failures, err)
		return &NotifyResult{Target: target, Err: barkJoinWithin(failures, barkMaxErrorBytes)}
	}
	retryTimes, sleep := nativeRetrySettings(&cfg.NativeNetworkConfig)

	push := func(key string) (err error) {
		// recover only works in the goroutine that panics: without it a panic in the HTTP stack
		// would take the whole process down
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("bark push panicked: %v", r)
			}
		}()
		m := msg
		m.DeviceKey = key
		payload, err := json.Marshal(m)
		if err != nil {
			return err
		}
		_, err = doWithRetry(ctx, req.HttpClient, retryTimes, sleep,
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
		return err
	}

	// Each key waits out its own timeout and retries when the server is down, so a fixed number of
	// workers send in parallel; errs keeps the outcome of keys[i] at index i
	errs := make([]error, len(keys))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < barkMaxConcurrency && w < len(keys); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				errs[i] = push(keys[i])
			}
		}()
	}
	for i := range keys {
		select {
		case jobs <- i:
		case <-ctx.Done():
			errs[i] = ctx.Err()
		}
	}
	close(jobs)
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
		// the error of keys[i] can only echo keys[i]
		reason, _ := truncateInBytes(redactBarkKeys(err.Error(), cfg.Password, keys[i]), barkMaxReasonBytes)
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

// barkAPNsSize is the size of the payload bark-server sends to APNs for a push, rebuilt after its
// apns/apns.go: title, body, category, mutable-content and the sound (default "1107", ".caf" appended)
// under aps; the group as aps thread-id and again as a root key, like every other extra key (level,
// url). JSON escaping is counted: quotes, backslashes and <>& grow when encoded.
func barkAPNsSize(m barkMessage) int {
	sound := "1107"
	if m.Sound != "" {
		sound = m.Sound + ".caf"
	}
	aps := map[string]interface{}{
		"alert":           map[string]string{"title": m.Title, "body": m.Body},
		"category":        "myNotificationCategory",
		"mutable-content": 1,
		"sound":           sound,
	}
	root := map[string]interface{}{"aps": aps, "level": m.Level}
	if m.Group != "" {
		aps["thread-id"] = m.Group
		root["group"] = m.Group
	}
	if m.URL != "" {
		root["url"] = m.URL
	}
	b, _ := json.Marshal(root)
	return len(b)
}

// fitBarkPayload cuts the body on a character boundary until the APNs payload fits; Apple rejects
// larger payloads and a retry with the same content cannot succeed. The event link is dropped when
// the other fields alone are too large (a very long site URL).
func fitBarkPayload(m *barkMessage) error {
	if barkAPNsSize(*m) <= barkMaxPayloadBytes {
		return nil
	}
	runes := []rune(m.Body)
	cut := func(n int) string { return string(runes[:n]) + "…" }
	probe := *m
	probe.Body = cut(0)
	if barkAPNsSize(probe) > barkMaxPayloadBytes {
		m.URL = ""
		if barkAPNsSize(*m) <= barkMaxPayloadBytes {
			return nil
		}
	}
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
	if barkAPNsSize(*m) > barkMaxPayloadBytes {
		return errors.New("the Bark push exceeds the 4096-byte APNs limit even with an empty body; shorten the title, group or sound")
	}
	return nil
}

// barkMaskKey keeps the last 4 characters of a key only when enough of it stays hidden;
// self-hosted servers accept short and non-ASCII custom keys, so it counts runes, not bytes
func barkMaskKey(key string) string {
	r := []rune(strings.TrimSpace(key))
	if len(r) < 12 {
		return redactedMark
	}
	return redactedMark + string(r[len(r)-4:])
}

// redactBarkKeys removes the password and a device key from an error text. redactSecrets skips
// values shorter than 6 characters, so the key is also replaced wherever it stands as a whole token,
// such as the "[key]" bark-server echoes for an unknown key.
func redactBarkKeys(msg, password, key string) string {
	return replaceToken(redactSecrets(msg, password, key), key, redactedMark)
}

// replaceToken replaces old in s where it is not part of a longer key-like word; the boundaries
// are checked against the original text
func replaceToken(s, old, repl string) string {
	if old == "" {
		return s
	}
	isKeyChar := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' }
	var b strings.Builder
	last, pos := 0, 0
	for {
		i := strings.Index(s[pos:], old)
		if i < 0 {
			break
		}
		i += pos
		end := i + len(old)
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		after, _ := utf8.DecodeRuneInString(s[end:])
		if (i == 0 || !isKeyChar(before)) && (end == len(s) || !isKeyChar(after)) {
			b.WriteString(s[last:i])
			b.WriteString(repl)
			last, pos = end, end
		} else {
			_, w := utf8.DecodeRuneInString(s[i:])
			pos = i + w
		}
	}
	b.WriteString(s[last:])
	return b.String()
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

// barkJoinWithin joins the errors in order within max bytes, in one pass: the ones that do not fit
// are counted at the end. Reasons come before the summary of who was pushed, so the summary goes first.
func barkJoinWithin(errs []error, max int) error {
	if len(errs) == 0 {
		return nil
	}
	note := func(n int) error { return fmt.Errorf("+%d more not shown", n) }
	reserve := len(note(len(errs)).Error()) + 1
	var kept []error
	used := 0
	for i, e := range errs {
		n := len(e.Error())
		if i > 0 {
			n++
		}
		room := max - reserve
		if i == len(errs)-1 {
			room = max
		}
		if used+n > room {
			break
		}
		kept = append(kept, e)
		used += n
	}
	if len(kept) == 0 {
		// the per-item caps keep a single error well below max; cut it anyway rather than overflow the record
		first, _ := truncateInBytes(errs[0].Error(), max-reserve)
		kept = append(kept, errors.New(first))
	}
	if n := len(errs) - len(kept); n > 0 {
		kept = append(kept, note(n))
	}
	return errors.Join(kept...)
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
