package provider

import (
	"context"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestCardNotify_UploadsScreenshotBeforeRenderingWebhook(t *testing.T) {
	cases := []struct {
		name     string
		provider NotifyChannelProvider
	}{
		{name: "Feishu", provider: &FeishuCardProvider{}},
		{name: "Lark", provider: &LarkCardProvider{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
				switch r.Path {
				case "/open-apis/auth/v3/tenant_access_token/internal":
					return 200, `{"code":0,"tenant_access_token":"local-token"}`
				case "/open-apis/im/v1/images":
					return 200, `{"code":0,"data":{"image_key":"image-1"}}`
				case "/webhook":
					return 200, `{"code":0}`
				default:
					return 404, `{}`
				}
			})
			destination, err := url.Parse(srv.URL)
			require.NoError(t, err)
			// Reuse the existing transport fake and send every request to the local server.
			transport := http.DefaultTransport
			client := &http.Client{Transport: flashDutyTestTransport(func(req *http.Request) (*http.Response, error) {
				local := req.Clone(req.Context())
				local.URL.Scheme, local.URL.Host, local.Host = destination.Scheme, destination.Host, destination.Host
				return transport.RoundTrip(local)
			})}
			req := webhookReq("http", &models.RequestConfig{
				FeishuRequestConfig: &models.FeishuRequestConfig{AppID: "local-app", AppSecret: "local-secret"},
				HTTPRequestConfig: &models.HTTPRequestConfig{
					URL: srv.URL + "/webhook", Method: "POST", Headers: map[string]string{"Content-Type": "application/json"}, RetryTimes: 1,
					Request: models.RequestDetail{Body: `{"image":"{{$params.shot_image_key}}","title":"{{$tpl.title}}"}`},
				},
			}, nil)
			image := []byte("local-image-bytes")
			req.Events[0].ShotImageBase64 = map[string]string{"shot": "data:image/png;base64," + base64.StdEncoding.EncodeToString(image)}
			req.HttpClient = client
			req.TplContent = map[string]interface{}{"title": "CPU high"}

			result := tc.provider.Notify(context.Background(), req)

			require.NoError(t, result.Err)
			calls := got()
			require.Len(t, calls, 3)
			require.Equal(t, "/open-apis/auth/v3/tenant_access_token/internal", calls[0].Path)
			require.Equal(t, http.MethodPost, calls[0].Method)
			require.Equal(t, "application/json", calls[0].Header.Get("Content-Type"))
			require.JSONEq(t, `{"app_id":"local-app","app_secret":"local-secret"}`, string(calls[0].Body))
			require.Equal(t, "/open-apis/im/v1/images", calls[1].Path)
			require.Equal(t, "Bearer local-token", calls[1].Header.Get("Authorization"))
			mediaType, params, err := mime.ParseMediaType(calls[1].Header.Get("Content-Type"))
			require.NoError(t, err)
			require.Equal(t, "multipart/form-data", mediaType)
			reader := multipart.NewReader(strings.NewReader(string(calls[1].Body)), params["boundary"])
			form, err := reader.ReadForm(1024)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, form.RemoveAll()) })
			require.Equal(t, []string{"message"}, form.Value["image_type"])
			require.Len(t, form.File["image"], 1)
			file, err := form.File["image"][0].Open()
			require.NoError(t, err)
			uploaded, err := io.ReadAll(file)
			require.NoError(t, err)
			require.NoError(t, file.Close())
			require.Equal(t, image, uploaded)
			require.Equal(t, "/webhook", calls[2].Path)
			require.JSONEq(t, `{"image":"image-1","title":"CPU high"}`, string(calls[2].Body))
		})
	}
}

func TestOpenPlatformToken_RejectsInvalidResponse(t *testing.T) {
	cases := []struct {
		name     string
		response string
		want     string
	}{
		{name: "invalid JSON", response: `{`, want: "parse token response failed"},
		{name: "API failure", response: `{"code":999,"msg":"denied"}`, want: "code=999 msg=denied"},
		{name: "missing token", response: `{"code":0}`, want: "get token failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, always(200, tc.response))

			token, err := getOpenPlatformTenantAccessToken(context.Background(), srv.Client(), "app", "secret", srv.URL)

			require.Empty(t, token)
			require.ErrorContains(t, err, tc.want)
			require.Len(t, got(), 1)
		})
	}
}

func TestOpenPlatformImage_RejectsInvalidResponse(t *testing.T) {
	cases := []struct {
		name     string
		response string
		want     string
	}{
		{name: "invalid JSON", response: `{`, want: "parse upload image response failed"},
		{name: "API failure", response: `{"code":999,"msg":"denied"}`, want: "code=999 msg=denied"},
		{name: "missing image key", response: `{"code":0,"data":{}}`, want: "upload image failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, always(200, tc.response))

			key, err := uploadOpenPlatformImage(context.Background(), srv.Client(), "local-token", "aW1hZ2U=", srv.URL)

			require.Empty(t, key)
			require.ErrorContains(t, err, tc.want)
			require.Len(t, got(), 1)
		})
	}
}

func TestOpenPlatformImage_ValidatesInputBeforeSending(t *testing.T) {
	cases := []struct {
		name  string
		token string
		image string
		want  string
	}{
		{name: "missing token", image: "aW1hZ2U=", want: "token cannot be empty"},
		{name: "invalid base64", token: "local-token", image: "%%%", want: "base64"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := newCaptureServer(t, always(200, `{}`))

			key, err := uploadOpenPlatformImage(context.Background(), srv.Client(), tc.token, tc.image, srv.URL)

			require.Empty(t, key)
			require.ErrorContains(t, err, tc.want)
			require.Empty(t, got())
		})
	}
}

func TestWecomNotify_PostsScreenshotWithDigestAfterText(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"errcode":0}`))
	req := webhookReq("http", &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
		URL: srv.URL + "/robot", Method: "POST", Headers: map[string]string{"Content-Type": "application/json"}, RetryTimes: 1,
		Request: models.RequestDetail{Body: `{"msgtype":"markdown","markdown":{"content":"{{$tpl.content}}"}}`},
	}}, map[string]string{"bot_name": "Operations"})
	req.TplContent = map[string]interface{}{"content": "CPU high"}
	req.Sendtos = []string{"Operations"}
	req.Events[0].ShotImageBase64 = map[string]string{"shot": "aW1hZ2U="}

	result := (&WecomProvider{}).Notify(context.Background(), req)

	require.NoError(t, result.Err)
	require.Equal(t, "Operations", result.Target)
	require.Contains(t, result.Response, "image_send:")
	require.Len(t, got(), 2)
	require.JSONEq(t, `{"msgtype":"markdown","markdown":{"content":"CPU high"}}`, string(got()[0].Body))
	// MD5("image") is the digest required by the WeCom robot image API.
	require.JSONEq(t, `{"msgtype":"image","image":{"base64":"aW1hZ2U=","md5":"78805a221a988e79ef3f42d7c5bfd418"}}`,
		string(got()[1].Body))
	require.Equal(t, http.MethodPost, got()[1].Method)
	require.Equal(t, "/robot", got()[1].Path)
}

func TestWecomNotify_ReportsInvalidScreenshotAfterText(t *testing.T) {
	srv, got := newCaptureServer(t, always(200, `{"errcode":0}`))
	req := webhookReq("http", &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
		URL: srv.URL, Method: "POST", RetryTimes: 1, Request: models.RequestDetail{Body: `{"msgtype":"text"}`},
	}}, nil)
	req.Events[0].ShotImageBase64 = map[string]string{"shot": "%%%"}

	result := (&WecomProvider{}).Notify(context.Background(), req)

	require.ErrorContains(t, result.Err, "decode wecom image payload failed")
	require.Contains(t, result.Response, "status_code:200")
	require.Len(t, got(), 1)
}
