package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

func TestScriptUploadErrorForLog(t *testing.T) {
	const secret = "private/token"
	// Run the logging path in a child process so capturing output cannot affect other tests.
	if os.Getenv("N9E_TEST_SCRIPT_UPLOAD_ERROR_LOG") == "1" {
		client := &http.Client{Transport: flashDutyTestTransport(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})}
		for _, key := range []string{"access_token", "integration_key"} {
			prepareImageEvents(context.Background(), []*models.AlertCurEvent{{ShotImageBase64: map[string]string{"panel": "image"}}}, func(string) (string, error) {
				_, err := client.Post("https://example.test/upload?"+key+"="+url.QueryEscape(secret), "image/png", nil)
				return "", err
			})
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestScriptUploadErrorForLog$")
	cmd.Env = append(os.Environ(), "N9E_TEST_SCRIPT_UPLOAD_ERROR_LOG=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("capture upload logs: %v", err)
	}
	msg := string(output)
	if strings.Count(msg, "deadline exceeded") != 2 {
		t.Fatal("expected both upload failures to be logged")
	}
	if strings.Contains(msg, secret) || strings.Contains(msg, url.QueryEscape(secret)) {
		t.Fatal("upload error log contains credentials")
	}
}

func TestScriptImageUpload(t *testing.T) {
	for _, platform := range []string{"dingtalk", "feishu", "flashduty", "flashduty_failure", "client_failure", "none", "upload_failure", "token_failure"} {
		t.Run(platform, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "stdin.json")
			script := filepath.Join(dir, "notify.sh")
			if err := os.WriteFile(script, []byte("#!/bin/sh\ncat > '"+output+"'\n"), 0700); err != nil {
				t.Fatal(err)
			}
			config := &models.NotifyChannelConfig{RequestType: ScriptIdent, RequestConfig: &models.RequestConfig{ScriptRequestConfig: &models.ScriptRequestConfig{Path: script, Timeout: 3000}}}
			if platform == "dingtalk" {
				config.RequestConfig.ScriptRequestConfig.ImageUpload = &models.ScriptImageUpload{Platform: "dingtalk", DingtalkRequestConfig: &models.DingtalkRequestConfig{AppKey: "key", AppSecret: "secret"}}
			} else if strings.HasPrefix(platform, "flashduty") {
				config.RequestConfig.ScriptRequestConfig.ImageUpload = &models.ScriptImageUpload{Platform: "flashduty", FlashDutyRequestConfig: &models.FlashDutyRequestConfig{IntegrationUrl: "https://flashduty.test/event/push?integration_key=test%2Fkey"}}
			} else if platform == "client_failure" {
				config.RequestConfig.ScriptRequestConfig.ImageUpload = &models.ScriptImageUpload{Platform: "flashduty", FlashDutyRequestConfig: &models.FlashDutyRequestConfig{IntegrationUrl: "https://flashduty.test/event?integration_key=test"}}
			} else if platform != "none" {
				config.RequestConfig.ScriptRequestConfig.ImageUpload = &models.ScriptImageUpload{Platform: "feishu", FeishuRequestConfig: &models.FeishuRequestConfig{AppID: "id", AppSecret: "secret"}}
			}
			config.RequestConfig.ScriptRequestConfig.Proxy = &models.ScriptProxy{Headers: map[string]string{"X-Upload-Test": "configured"}}
			var tokens, uploads atomic.Int32
			client := &http.Client{Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.Header.Get("X-Upload-Test") != "configured" {
					t.Error("missing configured upload header")
				}
				var body string
				switch r.URL.Path {
				case "/v1.0/oauth2/accessToken", "/open-apis/auth/v3/tenant_access_token/internal":
					tokens.Add(1)
					var credentials map[string]string
					if err := json.NewDecoder(r.Body).Decode(&credentials); err != nil {
						t.Error(err)
					}
					if platform == "dingtalk" {
						if credentials["appKey"] != "key" || credentials["appSecret"] != "secret" {
							t.Errorf("credentials: %v", credentials)
						}
					} else if credentials["app_id"] != "id" || credentials["app_secret"] != "secret" {
						t.Errorf("credentials: %v", credentials)
					}
					if platform == "token_failure" {
						return nil, fmt.Errorf("token failed")
					}
					body = `{"accessToken":"token","tenant_access_token":"token"}`
				case "/media/upload", "/open-apis/im/v1/images", "/push/image/upload":
					uploads.Add(1)
					if platform == "upload_failure" || platform == "flashduty_failure" {
						return nil, fmt.Errorf("upload failed")
					}
					field := "image"
					if platform == "dingtalk" {
						field = "media"
						if r.URL.Query().Get("access_token") != "token" {
							t.Error("missing token")
						}
					} else if platform == "flashduty" {
						if r.URL.Host != "flashduty.test" || r.URL.Query().Get("integration_key") != "test/key" {
							t.Errorf("unexpected FlashDuty URL: %s", r.URL)
						}
					} else if r.Header.Get("Authorization") != "Bearer token" {
						t.Error("missing token")
					}
					f, _, err := r.FormFile(field)
					if err != nil {
						t.Error(err)
						return nil, err
					}
					defer f.Close()
					data, _ := io.ReadAll(f)
					if string(data) != "image" {
						t.Errorf("image: %q", data)
					}
					body = `{"media_id":"img_id","data":{"image_key":"img_id"}}`
				default:
					return nil, fmt.Errorf("unexpected URL: %s", r.URL)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			if platform == "client_failure" {
				client = nil
			}
			event := &models.AlertCurEvent{ShotImageBase64: map[string]string{"a": base64.StdEncoding.EncodeToString([]byte("image"))}, RuleConfig: `{"existing":"value"}`, AnnotationsJSON: map[string]string{"existing": "value"}}
			result := (&ScriptProvider{}).Notify(context.Background(), &NotifyRequest{Config: config, Events: []*models.AlertCurEvent{event, event, nil}, HttpClient: client})
			if result.Err != nil {
				t.Fatal(result.Err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			var got struct {
				Event  *models.AlertCurEvent   `json:"event"`
				Events []*models.AlertCurEvent `json:"events"`
			}
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Events) != 3 || got.Events[2] != nil {
				t.Fatalf("events: %s", data)
			}
			for _, e := range []*models.AlertCurEvent{got.Event, got.Events[0], got.Events[1]} {
				if platform == "none" {
					if e.ShotImageBase64["a"] != event.ShotImageBase64["a"] || e.AnnotationsJSON["img_key"] != "" {
						t.Error("original screenshot was not passed to the script")
					}
				} else if e.ShotImageBase64 != nil {
					t.Error("base64 retained")
				}
				if strings.HasSuffix(platform, "_failure") && (e.AnnotationsJSON["img_key"] != "" || e.RuleConfigJson != nil) {
					t.Error("failed upload changed image metadata")
				}
				if platform == "dingtalk" || platform == "feishu" || platform == "flashduty" {
					if e.AnnotationsJSON["img_key"] != "img_id" {
						t.Errorf("annotations: %v", e.AnnotationsJSON)
					}
					rule, ok := e.RuleConfigJson.(map[string]interface{})
					if !ok {
						t.Fatalf("unexpected rule config: %v", e.RuleConfigJson)
					}
					if rule["snapshot_pic_url"] != "img_id" || rule["existing"] != "value" {
						t.Errorf("rule config: %v", rule)
					}
				}
			}
			if len(event.ShotImageBase64) != 1 || len(event.AnnotationsJSON) != 1 || event.RuleConfigJson != nil {
				t.Error("original mutated")
			}
			wantTokens, wantUploads := int32(1), int32(2)
			if platform == "none" || platform == "client_failure" {
				wantTokens, wantUploads = 0, 0
			}
			if strings.HasPrefix(platform, "flashduty") {
				wantTokens = 0
			}
			if platform == "token_failure" {
				wantUploads = 0
			}
			if tokens.Load() != wantTokens || uploads.Load() != wantUploads {
				t.Errorf("tokens=%d uploads=%d", tokens.Load(), uploads.Load())
			}
		})
	}
}

func TestScriptChannelSaveWithoutImageUpload(t *testing.T) {
	config := &models.NotifyChannelConfig{Ident: ScriptIdent, RequestType: ScriptIdent, RequestConfig: &models.RequestConfig{
		ScriptRequestConfig: &models.ScriptRequestConfig{ScriptType: "path", Path: "/bin/cat"},
	}}
	if err := VerifyChannelConfig(config); err != nil {
		t.Fatal(err)
	}
}

func TestScriptImageUploadInvalidConfig(t *testing.T) {
	for _, upload := range []models.ScriptImageUpload{
		{},
		{Platform: "fieshu"},
		{Platform: "dingtalk"},
		{Platform: "feishu", FeishuRequestConfig: &models.FeishuRequestConfig{AppID: "id", AppSecret: " "}},
		{Platform: "flashduty", FlashDutyRequestConfig: &models.FlashDutyRequestConfig{IntegrationUrl: "https://example.test"}},
	} {
		t.Run(upload.Platform, func(t *testing.T) {
			config := &models.NotifyChannelConfig{RequestType: ScriptIdent, RequestConfig: &models.RequestConfig{
				ScriptRequestConfig: &models.ScriptRequestConfig{ScriptType: "path", Path: "/bin/cat", Timeout: 3000, ImageUpload: &upload},
			}}
			if err := VerifyChannelConfig(config); err == nil {
				t.Fatal("invalid image upload config accepted")
			}
			if result := (&ScriptProvider{}).Notify(context.Background(), &NotifyRequest{Config: config}); result.Err != nil {
				t.Fatalf("invalid upload config blocked script: %v", result.Err)
			}
		})
	}
}

func TestScriptFlashDutyUploadUsesProxyAndTimeout(t *testing.T) {
	var uploads atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "flashduty.invalid" || r.URL.Path != "/push/image/upload" {
			t.Errorf("unexpected proxied URL %v", r.URL)
		}
		uploads.Add(1)
		fmt.Fprint(w, `{"data":{"image_key":"img_id"}}`)
	}))
	defer proxy.Close()
	upload := &models.ScriptImageUpload{Platform: "flashduty", FlashDutyRequestConfig: &models.FlashDutyRequestConfig{
		IntegrationUrl: "http://flashduty.invalid/event?integration_key=test",
	}}
	config := &models.NotifyChannelConfig{RequestType: ScriptIdent, RequestConfig: &models.RequestConfig{
		ScriptRequestConfig: &models.ScriptRequestConfig{
			Path: "/bin/cat", Timeout: 3000, ImageUpload: upload,
			Proxy: &models.ScriptProxy{Url: proxy.URL, Timeout: 2},
		},
	}}
	client, err := models.GetHTTPClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if client.Timeout != 2*time.Second {
		t.Fatalf("unexpected upload timeout %v", client.Timeout)
	}
	event := &models.AlertCurEvent{ShotImageBase64: map[string]string{"panel": "aW1hZ2U="}}
	if result := (&ScriptProvider{}).Notify(context.Background(), &NotifyRequest{Config: config, Events: []*models.AlertCurEvent{event}, HttpClient: client}); result.Err != nil {
		t.Fatal(result.Err)
	}
	if uploads.Load() != 1 {
		t.Fatalf("proxy received %d uploads", uploads.Load())
	}
}

func TestScriptFlashDutyCancelDuringBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var started atomic.Int32
	client := &http.Client{Timeout: 2 * time.Second, Transport: flashDutyTestTransport(func(r *http.Request) (*http.Response, error) {
		if started.Add(1) == 5 {
			cancel()
		}
		<-r.Context().Done()
		if !errors.Is(r.Context().Err(), context.Canceled) {
			t.Error("upload did not receive batch cancellation")
		}
		return nil, r.Context().Err()
	})}
	dir := t.TempDir()
	marker := filepath.Join(dir, "executed")
	script := filepath.Join(dir, "notify.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	config := &models.NotifyChannelConfig{RequestType: ScriptIdent, RequestConfig: &models.RequestConfig{
		ScriptRequestConfig: &models.ScriptRequestConfig{Path: script, Timeout: 3000, ImageUpload: &models.ScriptImageUpload{
			Platform: "flashduty", FlashDutyRequestConfig: &models.FlashDutyRequestConfig{IntegrationUrl: "https://flashduty.test/event?integration_key=test"},
		}},
	}}
	var events []*models.AlertCurEvent
	for i := 0; i < 12; i++ {
		events = append(events, &models.AlertCurEvent{ShotImageBase64: map[string]string{"panel": "aW1hZ2U="}})
	}
	result := (&ScriptProvider{}).Notify(ctx, &NotifyRequest{Config: config, Events: events, HttpClient: client})
	if !errors.Is(result.Err, context.Canceled) || started.Load() != 5 {
		t.Fatalf("uploads=%d err=%v", started.Load(), result.Err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("script execution marker: %v", err)
	}
}

func TestScriptImageUploadProxyAndHeaders(t *testing.T) {
	for _, withHeaders := range []bool{false, true} {
		t.Run(fmt.Sprintf("headers=%t", withHeaders), func(t *testing.T) {
			var headers map[string]string
			if withHeaders {
				headers = map[string]string{"X-Upload-Test": "configured"}
			}
			var uploads atomic.Int32
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Host != "flashduty.invalid" || r.URL.Path != "/push/image/upload" {
					t.Errorf("unexpected proxied URL: %v", r.URL)
				}
				if r.Header.Get("X-Upload-Test") != headers["X-Upload-Test"] {
					t.Error("missing configured upload header")
				}
				if f, _, err := r.FormFile("image"); err != nil {
					t.Errorf("invalid image multipart body: %v", err)
				} else {
					f.Close()
				}
				uploads.Add(1)
				fmt.Fprint(w, `{"data":{"image_key":"img_id"}}`)
			}))
			defer proxy.Close()
			config := &models.NotifyChannelConfig{RequestType: ScriptIdent, RequestConfig: &models.RequestConfig{
				ScriptRequestConfig: &models.ScriptRequestConfig{
					Proxy: &models.ScriptProxy{Url: proxy.URL, Headers: headers},
					ImageUpload: &models.ScriptImageUpload{Platform: "flashduty", FlashDutyRequestConfig: &models.FlashDutyRequestConfig{
						IntegrationUrl: "http://flashduty.invalid/event?integration_key=test",
					}},
				},
			}}
			client, err := models.GetHTTPClient(config)
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			originalTransport := client.Transport
			events := prepareScriptEvents(context.Background(), config, []*models.AlertCurEvent{{ShotImageBase64: map[string]string{"panel": "aW1hZ2U="}}}, client)
			if uploads.Load() != 1 || events[0].AnnotationsJSON["img_key"] != "img_id" {
				t.Fatalf("uploads=%d annotations=%v", uploads.Load(), events[0].AnnotationsJSON)
			}
			if client.Transport != originalTransport {
				t.Error("shared client transport was modified")
			}
		})
	}
}
