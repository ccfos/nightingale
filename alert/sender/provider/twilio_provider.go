package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ccfos/nightingale/v6/models"
)

// TwilioProvider creates SMS messages or outbound calls. Acceptance is not delivery.
// https://www.twilio.com/docs/messaging/api/message-resource
// https://www.twilio.com/docs/voice/api/call-resource
// https://www.twilio.com/docs/voice/twiml/say
type TwilioProvider struct{}

var twilioAccountSID = regexp.MustCompile(`^AC[0-9a-fA-F]{32}$`)
var twilioServiceSID = regexp.MustCompile(`^MG[0-9a-fA-F]{32}$`)
var twilioPhone = regexp.MustCompile(`^\+[1-9][0-9]{1,14}$`)

func (*TwilioProvider) Ident() string { return models.Twilio }
func (*TwilioProvider) Check(c *models.NotifyChannelConfig) error {
	if c == nil || c.RequestType != models.RequestTypeTwilio {
		return errors.New("twilio provider requires request_type: twilio")
	}
	if c.RequestConfig == nil || c.RequestConfig.TwilioRequestConfig == nil {
		return errors.New("twilio request config cannot be nil")
	}
	return validateTwilioConfig(c.RequestConfig.TwilioRequestConfig, true)
}

func validateTwilioConfig(c *models.TwilioRequestConfig, allowVars bool) error {
	variable := func(v string) bool { return allowVars && strings.Contains(v, "{{") }
	if !variable(c.AccountSID) && !twilioAccountSID.MatchString(strings.TrimSpace(c.AccountSID)) {
		return errors.New("Twilio Account SID must start with AC and contain 32 hexadecimal characters")
	}
	if strings.TrimSpace(c.AuthToken) == "" {
		return errors.New("Twilio Auth Token is required")
	}
	if c.Mode != "" && c.Mode != "sms" && c.Mode != "voice" {
		return errors.New("Twilio mode must be sms or voice")
	}
	if c.Mode == "voice" {
		if !variable(c.From) && !twilioPhone.MatchString(strings.TrimSpace(c.From)) {
			return errors.New("Twilio voice From must be a phone number in E.164 format, for example +14155552671")
		}
	} else {
		if strings.TrimSpace(c.From) == "" && strings.TrimSpace(c.MessagingServiceSID) == "" {
			return errors.New("Twilio SMS requires From or Messaging Service SID")
		}
		if c.MessagingServiceSID != "" && !variable(c.MessagingServiceSID) && !twilioServiceSID.MatchString(strings.TrimSpace(c.MessagingServiceSID)) {
			return errors.New("Twilio Messaging Service SID must start with MG and contain 32 hexadecimal characters")
		}
	}
	if c.RetryTimes < 0 || c.RetrySleep < 0 {
		return errors.New("Twilio retry settings must not be negative")
	}
	return nil
}

func expandTwilioConfig(c models.TwilioRequestConfig) (*models.TwilioRequestConfig, error) {
	fields := []*string{&c.AccountSID, &c.AuthToken, &c.From}
	if c.Mode == "voice" {
		fields = append(fields, &c.Voice, &c.Language)
	} else {
		fields = append(fields, &c.MessagingServiceSID)
	}
	// Switching modes preserves the other mode's form values; unused references
	// must not prevent sending (for example a deleted SMS service variable).
	for _, field := range fields {
		v, err := expandUserVars(strings.TrimSpace(*field))
		if err != nil {
			return nil, err
		}
		*field = v
	}
	if err := validateTwilioConfig(&c, false); err != nil {
		return nil, err
	}
	return &c, nil
}

type twilioSay struct {
	Voice    string `xml:"voice,attr"`
	Language string `xml:"language,attr"`
	Text     string `xml:",chardata"`
}
type twilioTwiML struct {
	XMLName xml.Name  `xml:"Response"`
	Say     twilioSay `xml:"Say"`
}

func (p *TwilioProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req == nil {
		return &NotifyResult{Err: errors.New("Twilio notify request is required")}
	}
	if err := p.Check(req.Config); err != nil {
		return &NotifyResult{Err: err}
	}
	cfg, err := expandTwilioConfig(*req.Config.RequestConfig.TwilioRequestConfig)
	if err != nil {
		return &NotifyResult{Err: err}
	}
	// One malformed contact must not block the rest of the team from being paged.
	var recipients, targets []string
	var failures []error
	seen := map[string]bool{}
	for _, raw := range req.Sendtos {
		phone := strings.TrimSpace(raw)
		if phone == "" || seen[phone] {
			continue
		}
		seen[phone] = true
		targets = append(targets, phone)
		if !twilioPhone.MatchString(phone) {
			failures = append(failures, fmt.Errorf("%s: phone number must be in E.164 format, for example +14155552671", phone))
			continue
		}
		recipients = append(recipients, phone)
	}
	target := strings.Join(targets, ",")
	if len(recipients) == 0 {
		failures = append(failures, errors.New("Twilio requires at least one recipient with a phone number in E.164 format"))
		return &NotifyResult{Target: target, Err: errors.Join(failures...)}
	}
	content := strings.TrimSpace(tplString(req.TplContent, "content"))
	if content == "" || utf8.RuneCountInString(content) > 1600 {
		return &NotifyResult{Target: target, Err: errors.New("Twilio message content must contain between 1 and 1600 characters")}
	}
	form := url.Values{}
	resource := "Messages"
	if cfg.Mode == "voice" {
		resource = "Calls"
		voice, language := cfg.Voice, cfg.Language
		if voice == "" {
			voice = "woman"
		}
		if language == "" {
			language = "en-US"
		}
		// XML encoding makes alert text safe even when it contains markup or quotes.
		twiml, err := xml.Marshal(twilioTwiML{Say: twilioSay{Voice: voice, Language: language, Text: content}})
		if err != nil {
			return &NotifyResult{Target: target, Err: err}
		}
		if utf8.RuneCount(twiml) > 4000 {
			return &NotifyResult{Target: target, Err: errors.New("Twilio voice content exceeds the 4000-character TwiML limit; shorten the message template")}
		}
		form.Set("Twiml", string(twiml))
		form.Set("From", cfg.From)
	} else {
		form.Set("Body", content)
		if cfg.From != "" {
			form.Set("From", cfg.From)
		}
		if cfg.MessagingServiceSID != "" {
			form.Set("MessagingServiceSid", cfg.MessagingServiceSID)
		}
	}
	endpoint := "https://api.twilio.com/2010-04-01/Accounts/" + cfg.AccountSID + "/" + resource + ".json"
	var results []string
	for _, phone := range recipients {
		if ctx != nil && ctx.Err() != nil {
			failures = append(failures, ctx.Err())
			break
		}
		form.Set("To", phone)
		resp, err := sendTwilio(ctx, req.HttpClient, cfg, endpoint, form.Encode())
		if err != nil {
			msg := redactTwilioError(err.Error(), cfg)
			failures = append(failures, fmt.Errorf("%s: %s", phone, msg))
			continue
		}
		var result twilioResponse
		_ = json.Unmarshal(resp.Body, &result) // validated by checkTwilioResponse
		results = append(results, fmt.Sprintf("%s: accepted %s (%s)", phone, result.SID, result.Status))
	}
	return &NotifyResult{Target: target, Response: strings.Join(results, "; "), Err: errors.Join(failures...)}
}

type twilioResponse struct {
	SID          string `json:"sid"`
	Status       string `json:"status"`
	Code         int    `json:"code"`
	Message      string `json:"message"`
	ErrorCode    *int   `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

func checkTwilioResponse(r *nativeResponse) (bool, error) {
	var v twilioResponse
	parseErr := json.Unmarshal(r.Body, &v)
	if r.StatusCode != http.StatusCreated {
		detail := truncateBody(r.Body)
		if v.Message != "" {
			detail = fmt.Sprintf("Twilio error %d: %s", v.Code, v.Message)
		}
		return false, &HTTPStatusError{StatusCode: r.StatusCode, Detail: detail}
	}
	if parseErr != nil || v.SID == "" || v.Status == "" {
		return false, errors.New("Twilio returned an invalid creation response")
	}
	if (v.ErrorCode != nil && *v.ErrorCode != 0) || v.Status == "failed" || v.Status == "undelivered" || v.Status == "canceled" {
		return false, fmt.Errorf("Twilio creation failed (%s): %s", v.Status, v.ErrorMessage)
	}
	return false, nil
}

// Retry only an explicit 429 rejection. Retrying a timeout, read failure or 5xx
// could create duplicate, billable messages/calls after Twilio already accepted them.
func sendTwilio(ctx context.Context, client *http.Client, cfg *models.TwilioRequestConfig, endpoint, body string) (*nativeResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for attempt := 0; ; attempt++ {
		resp, err := doWithRetry(ctx, client, 0, 0, func(ctx context.Context) (*http.Request, error) {
			r, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
			if err != nil {
				return nil, err
			}
			r.SetBasicAuth(cfg.AccountSID, cfg.AuthToken)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			return r, nil
		}, checkTwilioResponse)
		if err == nil || resp == nil || resp.StatusCode != http.StatusTooManyRequests || attempt >= cfg.RetryTimes {
			return resp, err
		}
		wait := time.Duration(cfg.RetrySleep) * time.Millisecond
		if wait <= 0 {
			wait = time.Second
		}
		if after, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			wait = after
		}
		if wait > maxRetryAfter {
			wait = maxRetryAfter
		}
		select {
		case <-ctx.Done():
			return resp, ctx.Err()
		case <-time.After(wait):
		}
	}
}

func redactTwilioError(msg string, cfg *models.TwilioRequestConfig) string {
	for _, secret := range []string{base64.StdEncoding.EncodeToString([]byte(cfg.AccountSID + ":" + cfg.AuthToken)), url.QueryEscape(cfg.AuthToken), cfg.AuthToken} {
		if secret != "" {
			msg = strings.ReplaceAll(msg, secret, redactedMark)
		}
	}
	return msg
}
