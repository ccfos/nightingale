package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func localFeishuEndpoints(t *testing.T, base string) {
	t.Helper()
	oldToken, oldImage, oldMessage := feishuAppTokenURL, feishuImageURL, feishuMessageAPIURL
	oldSearch, oldUser := feishuChatSearchURL, feishuBatchUserIDURL
	feishuAppTokenURL, feishuImageURL, feishuMessageAPIURL = base+"/token", base+"/image", base+"/message"
	feishuChatSearchURL, feishuBatchUserIDURL = base+"/search", base+"/users"
	t.Cleanup(func() {
		feishuAppTokenURL, feishuImageURL, feishuMessageAPIURL = oldToken, oldImage, oldMessage
		feishuChatSearchURL, feishuBatchUserIDURL = oldSearch, oldUser
	})
}

func TestFeishuAppNotify_SendsScreenshotCardToUserAndGroup(t *testing.T) {
	server, requests := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
		switch r.Path {
		case "/token":
			return 200, `{"code":0,"tenant_access_token":"local-token"}`
		case "/image":
			return 200, `{"code":0,"data":{"image_key":"image-1"}}`
		default:
			return 200, `{"code":0,"data":{"message_id":"message-1"}}`
		}
	})
	localFeishuEndpoints(t, server.URL)
	req := &NotifyRequest{
		Config: &models.NotifyChannelConfig{RequestConfig: &models.RequestConfig{
			FeishuAppRequestConfig: &models.FeishuAppRequestConfig{
				AppID: "local-app", AppSecret: "local-secret", ReceiveIDType: " EMAIL ",
			},
		}},
		Events:     []*models.AlertCurEvent{{ShotImageBase64: map[string]string{"shot": "aW1hZ2U="}}},
		TplContent: map[string]interface{}{"content": "CPU high"},
		Sendtos:    []string{" ", " alice@example.test "}, ImGroupIDs: []string{"", " chat-1 "},
		HttpClient: server.Client(),
	}

	result := (&FeishuAppProvider{}).Notify(context.Background(), req)

	require.NoError(t, result.Err)
	require.Equal(t, "alice@example.test,chat-1", result.Target)
	calls := requests()
	require.Len(t, calls, 4)
	require.JSONEq(t, `{"app_id":"local-app","app_secret":"local-secret"}`, string(calls[0].Body))
	require.Equal(t, "/image", calls[1].Path)
	require.Equal(t, "Bearer local-token", calls[1].Header.Get("Authorization"))
	for i, want := range []string{"email", "chat_id"} {
		call := calls[i+2]
		require.Equal(t, http.MethodPost, call.Method)
		require.Equal(t, "/message", call.Path)
		require.Equal(t, want, call.Query.Get("receive_id_type"))
		require.Equal(t, "application/json", call.Header.Get("Content-Type"))
		require.Equal(t, "Bearer local-token", call.Header.Get("Authorization"))
		body := call.json(t)
		require.Equal(t, "interactive", body["msg_type"])
		require.Equal(t, []string{"alice@example.test", "chat-1"}[i], body["receive_id"])
		var card map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(body["content"].(string)), &card))
		header := card["header"].(map[string]interface{})
		require.Equal(t, "Alert", header["title"].(map[string]interface{})["content"])
		elements := card["body"].(map[string]interface{})["elements"].([]interface{})
		require.Equal(t, "image-1", elements[1].(map[string]interface{})["img_key"])
		require.Contains(t, body["content"], "CPU high")
	}
	require.Equal(t, `{"code":0,"data":{"message_id":"message-1"}}; `+
		`{"code":0,"data":{"message_id":"message-1"}}`, result.Response)
}

func TestFeishuAppNotify_RejectsUnsupportedPersonalContact(t *testing.T) {
	server, requests := newCaptureServer(t, always(200, `{"code":0,"tenant_access_token":"token"}`))
	localFeishuEndpoints(t, server.URL)
	req := &NotifyRequest{
		Config: &models.NotifyChannelConfig{
			ParamConfig:   &models.NotifyParamConfig{UserInfo: &models.UserInfo{ContactKey: "phone"}},
			RequestConfig: &models.RequestConfig{FeishuAppRequestConfig: &models.FeishuAppRequestConfig{}},
		},
		Sendtos: []string{"123"}, HttpClient: server.Client(),
	}

	result := (&FeishuAppProvider{}).Notify(context.Background(), req)

	require.ErrorContains(t, result.Err, `unsupported contact_key="phone"`)
	require.Len(t, requests(), 1)
}

func TestFeishuAppNotify_RejectsEmptyRecipients(t *testing.T) {
	server, requests := newCaptureServer(t, always(200, `{"code":0,"tenant_access_token":"token"}`))
	localFeishuEndpoints(t, server.URL)
	req := &NotifyRequest{
		Config: &models.NotifyChannelConfig{RequestConfig: &models.RequestConfig{
			FeishuAppRequestConfig: &models.FeishuAppRequestConfig{ReceiveIDType: "user_id"},
		}},
		Sendtos: []string{" "}, ImGroupIDs: []string{" "}, HttpClient: server.Client(),
	}

	result := (&FeishuAppProvider{}).Notify(context.Background(), req)

	require.EqualError(t, result.Err, "no valid feishu receive_id found")
	require.Empty(t, result.Target)
	require.Len(t, requests(), 1)
}

func TestFeishuAppCheck_RejectsInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name   string
		config models.NotifyChannelConfig
		want   string
	}{
		{name: "wrong type", config: models.NotifyChannelConfig{RequestType: "http"}, want: "requires request_type"},
		{name: "missing config", config: models.NotifyChannelConfig{RequestType: "feishuapp"}, want: "cannot be nil"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := (&FeishuAppProvider{}).Check(&tc.config)

			require.ErrorContains(t, err, tc.want)
		})
	}
	for _, tc := range []struct {
		name string
		app  models.FeishuAppRequestConfig
		want string
	}{
		{name: "missing id", app: models.FeishuAppRequestConfig{AppSecret: "secret"}, want: "requires app_id"},
		{name: "missing secret", app: models.FeishuAppRequestConfig{AppID: "app"}, want: "requires app_secret"},
		{
			name: "unsupported contact", want: "receive_id_type must be",
			app: models.FeishuAppRequestConfig{AppID: "app", AppSecret: "secret", ReceiveIDType: "phone"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &models.NotifyChannelConfig{RequestType: "feishuapp", RequestConfig: &models.RequestConfig{
				FeishuAppRequestConfig: &tc.app,
			}}

			err := (&FeishuAppProvider{}).Check(config)

			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestSearchFeishuVisibleChats_EncodesSearchAndReturnsPagination(t *testing.T) {
	server, requests := newCaptureServer(t, always(200,
		`{"code":0,"data":{"has_more":true,"page_token":"next","items":[{"chat_id":"chat-1","name":"Ops"}]}}`))
	localFeishuEndpoints(t, server.URL)

	result, err := SearchFeishuVisibleChats(context.Background(), server.Client(), "token", "Ops & Dev", 0, "", "page+1")

	require.NoError(t, err)
	require.True(t, result.HasMore)
	require.Equal(t, "next", result.PageToken)
	require.Equal(t, []FeishuChatItem{{ChatID: "chat-1", Name: "Ops"}}, result.Items)
	calls := requests()
	require.Len(t, calls, 1)
	require.Equal(t, http.MethodGet, calls[0].Method)
	require.Equal(t, "Bearer token", calls[0].Header.Get("Authorization"))
	require.Equal(t, "Ops & Dev", calls[0].Query.Get("query"))
	require.Equal(t, "20", calls[0].Query.Get("page_size"))
	require.Equal(t, "user_id", calls[0].Query.Get("user_id_type"))
	require.Equal(t, "page+1", calls[0].Query.Get("page_token"))
}

func TestGetFeishuUserID_PostsContactsAndReturnsUserList(t *testing.T) {
	server, requests := newCaptureServer(t, always(200,
		`{"code":0,"data":{"user_list":[{"email":"a@example.test","mobile":"123","user_id":"ou-1"}]}}`))
	localFeishuEndpoints(t, server.URL)

	users, err := GetFeishuUserID(context.Background(), server.Client(), "token",
		[]string{"a@example.test"}, []string{"123"}, true, "")

	require.NoError(t, err)
	require.Equal(t, []FeishuUserIDItem{{Email: "a@example.test", Mobile: "123", UserID: "ou-1"}}, users)
	calls := requests()
	require.Len(t, calls, 1)
	require.Equal(t, http.MethodPost, calls[0].Method)
	require.Equal(t, "open_id", calls[0].Query.Get("user_id_type"))
	require.Equal(t, "Bearer token", calls[0].Header.Get("Authorization"))
	require.Equal(t, "application/json", calls[0].Header.Get("Content-Type"))
	require.JSONEq(t, `{"emails":["a@example.test"],"mobiles":["123"],"include_resigned":true}`, string(calls[0].Body))
}

func assertFeishuResponseFailure(t *testing.T, invoke func(*http.Client) error) {
	t.Helper()
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "invalid JSON", body: `{`, want: "parse feishu"},
		{name: "rejected", body: `{"code":999,"msg":"denied"}`, want: "code=999 msg=denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, requests := newCaptureServer(t, always(200, tc.body))
			localFeishuEndpoints(t, server.URL)

			err := invoke(server.Client())

			require.ErrorContains(t, err, tc.want)
			require.Len(t, requests(), 1)
		})
	}
}

func TestSendFeishuCardMessage_ReportsInvalidAndRejectedResponses(t *testing.T) {
	assertFeishuResponseFailure(t, func(client *http.Client) error {
		_, err := SendFeishuCardMessage(context.Background(), client, "token", "user_id", "u-1", `{}`)
		return err
	})
}

func TestSearchFeishuVisibleChats_ReportsInvalidAndRejectedResponses(t *testing.T) {
	assertFeishuResponseFailure(t, func(client *http.Client) error {
		_, err := SearchFeishuVisibleChats(context.Background(), client, "token", "Ops", 10, "user_id", "")
		return err
	})
}

func TestGetFeishuUserID_ReportsInvalidAndRejectedResponses(t *testing.T) {
	assertFeishuResponseFailure(t, func(client *http.Client) error {
		_, err := GetFeishuUserID(context.Background(), client, "token", []string{"a"}, nil, false, "user_id")
		return err
	})
}

func TestFeishuAppCheck_AppliesNetworkDefaults(t *testing.T) {
	app := &models.FeishuAppRequestConfig{
		AppID: "app", AppSecret: "secret", ReceiveIDType: " EMAIL ", RetrySleep: -1,
	}
	config := &models.NotifyChannelConfig{RequestType: "feishuapp", RequestConfig: &models.RequestConfig{
		FeishuAppRequestConfig: app,
	}}

	err := (&FeishuAppProvider{}).Check(config)

	require.NoError(t, err)
	require.Equal(t, 10000, app.Timeout)
	require.Equal(t, 1, app.RetryTimes)
	require.Equal(t, 1, app.RetrySleep)
}
