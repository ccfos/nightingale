package provider

import (
	"os"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	
	"time"

	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestFlashDutyUploadBeforeNotify(t *testing.T) {
	uploads, sends := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Query().Get("integration_key") != "test/key" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if r.URL.Path == "/push/image/upload" {
			uploads++
			if len(r.URL.Query()) != 1 {
				t.Errorf("unexpected upload query: %s", r.URL.RawQuery)
			}
			f, _, err := r.FormFile("image")
			if err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			defer f.Close()
			b, _ := io.ReadAll(f)
			if string(b) != "image bytes" {
				t.Errorf("unexpected image: %q", b)
			}
			io.WriteString(w, `{"data":{"image_key":"img_test"}}`)
			return
		}
		sends++
		if uploads != 1 {
			t.Errorf("uploads before send: %d", uploads)
		}
		if r.URL.Path != "/event/push" || r.URL.Query().Get("channel_id") == "" {
			t.Errorf("unexpected event URL: %s", r.URL)
		}
		var events []struct {
			RuleConfig struct {
				Snapshot string `json:"snapshot_pic_url"`
				Existing string `json:"existing"`
			} `json:"rule_config"`
			Shot        map[string]string `json:"shot_image_base64"`
			Annotations map[string]string `json:"annotations"`
		}
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			t.Error(err)
			return
		}
		if len(events) != 1 {
			t.Errorf("events: %v", events)
			return
		}
		if events[0].Annotations["img_key"] != "img_test" || events[0].Shot != nil || events[0].RuleConfig.Snapshot != "img_test" || events[0].RuleConfig.Existing != "preserved" {
			t.Errorf("unexpected payload: %+v", events[0])
		}
		io.WriteString(w, `{}`)
	}))
	defer server.Close()
	original := &models.AlertCurEvent{
		RuleConfigJson:  map[string]interface{}{"existing": "preserved"},
		ShotImageBase64: map[string]string{"panel": "data:image/png;base64,aW1hZ2UgYnl0ZXM="},
	}
	result := (&FlashDutyProvider{}).Notify(context.Background(), &NotifyRequest{
		Config: &models.NotifyChannelConfig{RequestConfig: &models.RequestConfig{FlashDutyRequestConfig: &models.FlashDutyRequestConfig{IntegrationUrl: server.URL + "/event/push?integration_key=test%2Fkey"}}},
		Events: []*models.AlertCurEvent{original}, HttpClient: server.Client(), FlashDutyChannelIDs: []int64{1, 2},
	})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if uploads != 1 || sends != 2 {
		t.Fatalf("uploads=%d sends=%d", uploads, sends)
	}
	if len(original.ShotImageBase64) != 1 || len(original.RuleConfigJson.(map[string]interface{})) != 1 {
		t.Fatal("original event was changed")
	}
}

func TestFlashDutyImagePreparation(t *testing.T) {
	for _, tc := range []struct {
		name, response, image string
		status                int
		uploadFails           bool
	}{
		{"raw base64", `{"data":{"image_key":"img_test"}}`, "aW1hZ2U=", 200, false},
		{"HTTP error", `{}`, "aW1hZ2U=", 429, true},
		{"API error", `{"error":{"code":"InvalidParameter"}}`, "aW1hZ2U=", 200, true},
		{"missing key", `{"data":{}}`, "aW1hZ2U=", 200, true},
		{"invalid JSON", `{`, "aW1hZ2U=", 200, true},
		{"invalid base64", `{}`, "!!!", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sends := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/push/image/upload" {
					sends++
					var sent []models.AlertCurEvent
					if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
						t.Error(err)
						return
					}
					if len(sent) != 2 || sent[0].ShotImageBase64 != nil || sent[1].Id != 2 {
						t.Errorf("unexpected sent events: %+v", sent)
					}
					io.WriteString(w, `{}`)
					return
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.response)
			}))
			defer server.Close()
			event := &models.AlertCurEvent{RuleConfig: `{"existing":1234567890123456789}`, ShotImageBase64: map[string]string{"panel": tc.image}}
			config := &models.FlashDutyRequestConfig{IntegrationUrl: server.URL + "/push?integration_key=test"}
			prepared, err := prepareFlashDutyEvents(config, []*models.AlertCurEvent{event}, server.Client())
			if err != nil {
				t.Fatalf("error=%v", err)
			}
			if !tc.uploadFails {
				b, _ := json.Marshal(prepared[0])
				if !strings.Contains(string(b), `"existing":1234567890123456789`) || !strings.Contains(string(b), `"snapshot_pic_url":"img_test"`) {
					t.Fatalf("unexpected payload %s", b)
				}
			} else if prepared[0].RuleConfigJson != nil {
				t.Fatal("failed upload changed rule config")
			}
			if prepared[0].ShotImageBase64 != nil || len(event.ShotImageBase64) != 1 {
				t.Fatal("screenshot was not stripped from copy or original was changed")
			}
			if _, err := SendFlashDuty(config, []*models.AlertCurEvent{event, {Id: 2}}, 0, server.Client()); err != nil {
				t.Fatalf("image upload must not block alerts: %v", err)
			}
			if sends != 1 {
				t.Fatalf("expected alert batch to be sent, got %d sends", sends)
			}
		})
	}
	event := &models.AlertCurEvent{RuleConfigJson: map[string]interface{}{"existing": "value"}}
	prepared, err := prepareFlashDutyEvents(nil, []*models.AlertCurEvent{nil, event}, http.DefaultClient)
	if err != nil || prepared[0] != nil || prepared[1].RuleConfigJson == nil {
		t.Fatalf("no-image preparation failed: %v", err)
	}
}

// Exercise transport failures without relying on timing or external endpoints.
type flashDutyTestTransport func(*http.Request) (*http.Response, error)

func (f flashDutyTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type flashDutyFailedBody struct{}

func (flashDutyFailedBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (flashDutyFailedBody) Close() error             { return nil }

func TestFlashDutyOptionalImageFailures(t *testing.T) {
	for _, failure := range []string{"connection", "response read", "non-object config", "malformed stored config"} {
		t.Run(failure, func(t *testing.T) {
			uploads, attempts := 0, 0
			channels := make(map[string]int)
			original := &models.AlertCurEvent{
				Id:              1,
				RuleConfigJson:  map[string]interface{}{"existing": "preserved"},
				ShotImageBase64: map[string]string{"panel": "aW1hZ2U="},
			}
			if failure == "non-object config" {
				original.RuleConfigJson = []string{"preserved"}
			}
			if failure == "malformed stored config" {
				original.RuleConfigJson = nil
				original.RuleConfig = "{"
			}
			originalConfig, _ := json.Marshal(original.RuleConfigJson)
			client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/push/image/upload" {
					uploads++
					if failure == "connection" {
						return nil, io.ErrUnexpectedEOF
					}
					return &http.Response{StatusCode: 200, Body: flashDutyFailedBody{}, Header: make(http.Header)}, nil
				}
				attempts++
				if attempts == 1 {
					return nil, io.ErrUnexpectedEOF // Existing event retry must still work.
				}
				var sent []models.AlertCurEvent
				if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
					t.Fatal(err)
				}
				if len(sent) != 2 || sent[0].ShotImageBase64 != nil || sent[1].Id != 2 {
					t.Fatalf("unexpected batch: %+v", sent)
				}
				config, _ := json.Marshal(sent[0].RuleConfigJson)
				if string(config) != string(originalConfig) {
					t.Errorf("rule config changed: %s -> %s", originalConfig, config)
				}
				channels[r.URL.Query().Get("channel_id")]++
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`)), Header: make(http.Header)}, nil
			})}
			result := (&FlashDutyProvider{}).Notify(context.Background(), &NotifyRequest{
				Config: &models.NotifyChannelConfig{RequestConfig: &models.RequestConfig{FlashDutyRequestConfig: &models.FlashDutyRequestConfig{
					IntegrationUrl: "http://flashduty.test/event/push?integration_key=test", RetryTimes: 1, RetrySleep: 1,
				}}},
				Events: []*models.AlertCurEvent{original, {Id: 2}}, HttpClient: client, FlashDutyChannelIDs: []int64{1, 2},
			})
			if result.Err != nil || attempts != 3 || channels["1"] != 1 || channels["2"] != 1 {
				t.Fatalf("result=%+v attempts=%d channels=%v", result, attempts, channels)
			}
			wantUploads := 1
			if failure == "non-object config" || failure == "malformed stored config" {
				wantUploads = 0
			}
			if uploads != wantUploads || len(original.ShotImageBase64) != 1 {
				t.Fatalf("uploads=%d; original screenshots=%v", uploads, original.ShotImageBase64)
			}
		})
	}
}

func TestFlashDutyPreservesEventData(t *testing.T) {
	for _, tc := range []struct {
		name, ruleConfig string
		uploadSucceeds   bool
	}{
		{"add snapshot", `{"queries":[{"limit":1234567890123456789}],"name":"rule"}`, true},
		{"replace snapshot object", `{"snapshot_pic_url":{"url":"https://example.com/image.png","extra":{"value":1234567890123456789}},"name":"rule"}`, true},
		{"existing image key", `{"snapshot_pic_url":{"img_key":"img_existing","url":"keep"}}`, true},
		{"existing string snapshot", `{"snapshot_pic_url":"https://example.com/image.png"}`, true},
		{"existing annotation key", `{"name":"rule"}`, true},
		{"empty snapshot", `{"snapshot_pic_url":""}`, true},
		{"null snapshot", `{"snapshot_pic_url":null}`, true},
		{"upload failure preserves old keys", `{"snapshot_pic_url":"img_existing"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := &models.AlertCurEvent{
				Id: 42, RuleName: "rule", RuleConfig: tc.ruleConfig,
				RuleConfigJson:  json.RawMessage(tc.ruleConfig),
				AnnotationsJSON: map[string]string{"note": "kept"},
				TagsJSON:        []string{"env=prod"}, ShotImageBase64: map[string]string{"panel": "aW1hZ2U="},
			}
			if tc.name == "existing annotation key" || !tc.uploadSucceeds {
				original.AnnotationsJSON["img_key"] = "img_existing"
			}
			before, _ := json.Marshal(original)
			uploads := 0
			client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) {
				uploads++
				if !tc.uploadSucceeds {
					return nil, io.ErrUnexpectedEOF
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"image_key":"img_new"}}`))}, nil
			})}
			prepared, err := prepareFlashDutyEvents(&models.FlashDutyRequestConfig{IntegrationUrl: "http://flashduty.test/push?integration_key=test"}, []*models.AlertCurEvent{original}, client)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(original)
			if string(before) != string(after) {
				t.Fatal("original event changed")
			}
			if prepared[0].ShotImageBase64 != nil {
				t.Fatal("outgoing screenshot not cleared")
			}
			if uploads != 1 {
				t.Fatalf("uploads=%d", uploads)
			}
			out, _ := json.Marshal(prepared[0])
			var got, want map[string]json.RawMessage
			json.Unmarshal(out, &got)
			json.Unmarshal(before, &want)
			// Remove only the intended changes, then compare all original event data.
			delete(got, "shot_image_base64")
			delete(want, "shot_image_base64")
			if tc.uploadSucceeds {
				var config map[string]json.RawMessage
				json.Unmarshal(got["rule_config"], &config)
				if string(config["snapshot_pic_url"]) != `"img_new"` {
					t.Fatalf("unexpected snapshot: %s", config["snapshot_pic_url"])
				}
				var originalConfig map[string]json.RawMessage
				json.Unmarshal(want["rule_config"], &originalConfig)
				if old, exists := originalConfig["snapshot_pic_url"]; exists {
					config["snapshot_pic_url"] = old
				} else {
					delete(config, "snapshot_pic_url")
				}
				var annotations map[string]string
				json.Unmarshal(got["annotations"], &annotations)
				if annotations["img_key"] != "img_new" {
					t.Fatalf("unexpected annotations: %v", annotations)
				}
				if old, exists := original.AnnotationsJSON["img_key"]; exists {
					annotations["img_key"] = old
				} else {
					delete(annotations, "img_key")
				}
				got["annotations"], _ = json.Marshal(annotations)
				got["rule_config"], _ = json.Marshal(config)
			}
			// Decode using json.Number to retain integer precision while ignoring object key order.
			decode := func(v interface{}) interface{} {
				b, _ := json.Marshal(v)
				d := json.NewDecoder(strings.NewReader(string(b)))
				d.UseNumber()
				var result interface{}
				if err := d.Decode(&result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			if !reflect.DeepEqual(decode(got), decode(want)) {
				t.Fatalf("existing event data changed: %s", out)
			}
		})
	}
}

var testEvents = []*models.AlertCurEvent{}

func TestFlashDutySend(t *testing.T) {

	for _, event := range testEvents {
		// 读取 ～/workspace/tmp/3.png
		imageData, err := os.ReadFile("/Users/liangyb/workspace/tmp/3.png")
		if err != nil {
			t.Fatal(err)
		}
		event.ShotImageBase64 = map[string]string{"panel": base64.StdEncoding.EncodeToString(imageData)}
	}

	// 创建NotifyChannelConfig对象
	config := &models.RequestConfig{
		FlashDutyRequestConfig: &models.FlashDutyRequestConfig{
			IntegrationUrl: "https://api.flashcat.cloud/event/push/alert/n9e?integration_key=xxx",
		},
	}

	httpClient := &http.Client{
		Timeout: time.Duration(10) * time.Second,
	}

	flashDutyChannelID := int64(4344322009498)
	resp, err := SendFlashDuty(config.FlashDutyRequestConfig, testEvents, flashDutyChannelID, httpClient)
	t.Logf("flashduty resp=%q err=%v", resp, err)
}
