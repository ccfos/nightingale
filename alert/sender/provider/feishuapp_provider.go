package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

var (
	feishuAppTokenURL    = "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal"
	feishuImageURL       = "https://open.feishu.cn/open-apis/im/v1/images"
	feishuMessageAPIURL  = "https://open.feishu.cn/open-apis/im/v1/messages"
	feishuChatSearchURL  = "https://open.feishu.cn/open-apis/im/v1/chats/search"
	feishuBatchUserIDURL = "https://open.feishu.cn/open-apis/contact/v3/users/batch_get_id"
)

type FeishuChatItem struct {
	Avatar      string `json:"avatar"`
	ChatID      string `json:"chat_id"`
	ChatStatus  string `json:"chat_status"`
	Description string `json:"description"`
	External    bool   `json:"external"`
	Name        string `json:"name"`
	OwnerID     string `json:"owner_id"`
	OwnerIDType string `json:"owner_id_type"`
	TenantKey   string `json:"tenant_key"`
}

type FeishuChatSearchResult struct {
	HasMore   bool             `json:"has_more"`
	Items     []FeishuChatItem `json:"items"`
	PageToken string           `json:"page_token"`
}

type FeishuUserIDItem struct {
	Email  string `json:"email"`
	Mobile string `json:"mobile"`
	UserID string `json:"user_id"`
}

type FeishuAppProvider struct{}

func (p *FeishuAppProvider) Ident() string { return "feishuapp" }

func (p *FeishuAppProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != p.Ident() {
		return fmt.Errorf("feishu app provider requires request_type=%s, got %q", p.Ident(), config.RequestType)
	}
	if config.RequestConfig == nil || config.RequestConfig.FeishuAppRequestConfig == nil {
		return errors.New("feishu app request config cannot be nil")
	}
	c := config.RequestConfig.FeishuAppRequestConfig
	if strings.TrimSpace(c.AppID) == "" {
		return errors.New("feishu app provider requires app_id")
	}
	if strings.TrimSpace(c.AppSecret) == "" {
		return errors.New("feishu app provider requires app_secret")
	}
	if v := strings.TrimSpace(c.ReceiveIDType); v != "" {
		if !isFeishuReceiveIDTypeAllowed(strings.ToLower(v)) {
			return errors.New("feishu app provider receive_id_type must be one of user_id/email/chat_id/open_id/union_id")
		}
	}
	if c.Timeout <= 0 {
		c.Timeout = 10000
	}
	if c.RetryTimes <= 0 {
		c.RetryTimes = 1
	}
	if c.RetrySleep < 0 {
		c.RetrySleep = 1
	}
	return nil
}

func (p *FeishuAppProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req == nil || req.Config == nil || req.Config.RequestConfig == nil || req.Config.RequestConfig.FeishuAppRequestConfig == nil {
		return &NotifyResult{Err: errors.New("feishu app request config cannot be nil")}
	}
	appConfig := req.Config.RequestConfig.FeishuAppRequestConfig
	token, err := GetFeishuTenantAccessToken(ctx, req.HttpClient, appConfig.AppID, appConfig.AppSecret)
	if err != nil {
		return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: "", Err: err}
	}

	title := getMapString(req.TplContent, "title")
	content := getMapString(req.TplContent, "content")
	if title == "" {
		title = "Alert"
	}

	imageBase64 := pickImageBase64(req.Events)
	imageKey := ""
	if imageBase64 != "" {
		imgKey, upErr := UploadFeishuImage(ctx, req.HttpClient, token, imageBase64)
		if upErr != nil {
			return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: "upload image failed: " + upErr.Error(), Err: upErr}
		}
		imageKey = imgKey
	}

	cardContent, err := renderFeishuCardJSON(req, title, content, imageKey)
	if err != nil {
		return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: "render feishu card json failed: " + err.Error(), Err: err}
	}

	targets := make([]string, 0, len(req.Sendtos)+len(req.ImGroupIDs))
	resps := make([]string, 0, len(req.Sendtos)+len(req.ImGroupIDs))

	// 个人: 使用 feishuapp_request_config.receive_id_type；未填时与历史一致，按 contact_key 推断。
	// ParamConfig/UserInfo 均为指针，群发或未配 UserInfo 的场景为 nil，必须守卫，避免告警协程 panic。
	contactKey := ""
	if req.Config.ParamConfig != nil && req.Config.ParamConfig.UserInfo != nil {
		contactKey = req.Config.ParamConfig.UserInfo.ContactKey
	}
	receiveIDType := resolveFeishuReceiveIDType(appConfig.ReceiveIDType, contactKey)
	if receiveIDType == "" && len(req.Sendtos) > 0 {
		err := fmt.Errorf("feishu app unsupported contact_key=%q, please configure receive_id_type or use user_id/email/chat_id", contactKey)
		return &NotifyResult{Target: getNotifyTarget(req.CustomParams, req.Sendtos), Response: "", Err: err}
	}
	for _, rid := range req.Sendtos {
		receiveID := strings.TrimSpace(rid)
		if receiveID == "" {
			continue
		}
		resp, sendErr := SendFeishuCardMessage(ctx, req.HttpClient, token, receiveIDType, receiveID, cardContent)
		if sendErr != nil {
			return &NotifyResult{Target: strings.Join(targets, ","), Response: strings.Join(resps, "; "), Err: sendErr}
		}
		targets = append(targets, receiveID)
		resps = append(resps, resp)
	}

	// 群聊: ImGroupIDs 固定按 chat_id 发送。
	for _, gid := range req.ImGroupIDs {
		chatID := strings.TrimSpace(gid)
		if chatID == "" {
			continue
		}
		resp, sendErr := SendFeishuCardMessage(ctx, req.HttpClient, token, "chat_id", chatID, cardContent)
		if sendErr != nil {
			return &NotifyResult{Target: strings.Join(targets, ","), Response: strings.Join(resps, "; "), Err: sendErr}
		}
		targets = append(targets, chatID)
		resps = append(resps, resp)
	}

	if len(targets) == 0 {
		return &NotifyResult{Target: "", Response: "no valid feishu receive_id found", Err: errors.New("no valid feishu receive_id found")}
	}
	return &NotifyResult{Target: strings.Join(targets, ","), Response: strings.Join(resps, "; "), Err: nil}
}

func GetFeishuTenantAccessToken(ctx context.Context, client *http.Client, appID, appSecret string) (string, error) {
	return getOpenPlatformTenantAccessToken(ctx, client, appID, appSecret, feishuAppTokenURL)
}

func UploadFeishuImage(ctx context.Context, client *http.Client, token, imageBase64 string) (string, error) {
	return uploadOpenPlatformImage(ctx, client, token, imageBase64, feishuImageURL)
}

func SendFeishuCardMessage(ctx context.Context, client *http.Client, token, receiveIDType, receiveID, content string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	receiveIDType = normalizeFeishuReceiveIDType(receiveIDType)
	if receiveIDType == "" {
		return "", fmt.Errorf("feishu unsupported receive_id_type")
	}
	reqBody, _ := json.Marshal(map[string]string{
		"content":    content,
		"msg_type":   "interactive",
		"receive_id": receiveID,
		"uuid":       fmt.Sprintf("n9e-%d", time.Now().UnixNano()),
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, feishuMessageAPIURL+"?receive_id_type="+url.QueryEscape(receiveIDType), bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err = json.Unmarshal(bs, &out); err != nil {
		return "", fmt.Errorf("parse feishu send message response failed: %w, body: %s", err, string(bs))
	}
	if out.Code != 0 {
		return string(bs), fmt.Errorf("send feishu card message failed: code=%d msg=%s", out.Code, out.Msg)
	}
	return string(bs), nil
}

// resolveFeishuReceiveIDType 解析发送时的 receive_id_type：优先取 channel 上显式配置的
// ReceiveIDType（Check 已校验），否则回退到 UserInfo.ContactKey。返回空串表示无法推断
// （比如 contact_key=phone 等飞书不直接支持的类型），调用方必须显式处理以避免把手机号
// 当 user_id 发出。
func resolveFeishuReceiveIDType(receiveIDType, contactKey string) string {
	if v := strings.TrimSpace(receiveIDType); v != "" {
		return normalizeFeishuReceiveIDType(v)
	}
	return normalizeFeishuReceiveIDType(contactKey)
}

func isFeishuReceiveIDTypeAllowed(s string) bool {
	switch s {
	case "user_id", "email", "chat_id", "open_id", "union_id":
		return true
	default:
		return false
	}
}

// normalizeFeishuReceiveIDType 把输入标准化为飞书接口可接受的 receive_id_type；
// 对 phone 等未识别的值返回空串而非静默回退 user_id，调用方据此显式报错。
func normalizeFeishuReceiveIDType(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "user_id", "email", "chat_id", "open_id", "union_id":
		return s
	default:
		return ""
	}
}

func renderFeishuCardJSON(req *NotifyRequest, title, content, imageKey string) (string, error) {
	data := map[string]interface{}{
		"msg_title":      title,
		"msg_body":       content,
		"shot_image_key": imageKey,
		"tpl":            req.TplContent,
		"params":         req.CustomParams,
		"events":         req.Events,
		"event":          nil,
	}
	if len(req.Events) > 0 {
		data["event"] = req.Events[0]
	}
	rendered := getParsedString("feishu_app_card_json", cardJson, data)
	if strings.TrimSpace(rendered) == "" {
		return "", errors.New("rendered feishu card content is empty")
	}
	return rendered, nil
}

// SearchFeishuVisibleChats 搜索机器人可见群聊。
// 对应接口: GET /open-apis/im/v1/chats/search
func SearchFeishuVisibleChats(ctx context.Context, client *http.Client, token, query string, pageSize int, userIDType, pageToken string) (*FeishuChatSearchResult, error) {
	if client == nil {
		return nil, errors.New("http client not found")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("tenant access token cannot be empty")
	}
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("query cannot be empty")
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if strings.TrimSpace(userIDType) == "" {
		userIDType = "user_id"
	}

	q := url.Values{}
	q.Set("page_size", fmt.Sprintf("%d", pageSize))
	q.Set("query", query)
	q.Set("user_id_type", userIDType)
	if strings.TrimSpace(pageToken) != "" {
		q.Set("page_token", pageToken)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feishuChatSearchURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out struct {
		Code int                    `json:"code"`
		Msg  string                 `json:"msg"`
		Data FeishuChatSearchResult `json:"data"`
	}
	if err = json.Unmarshal(bs, &out); err != nil {
		return nil, fmt.Errorf("parse feishu chat search response failed: %w, body: %s", err, string(bs))
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("search feishu chats failed: code=%d msg=%s", out.Code, out.Msg)
	}
	return &out.Data, nil
}

// GetFeishuUserID 通过手机号/邮箱批量查询用户 ID。
// userIDType 可选 user_id/open_id，默认 open_id。
func GetFeishuUserID(ctx context.Context, client *http.Client, token string, emails, mobiles []string, includeResigned bool, userIDType string) ([]FeishuUserIDItem, error) {
	if client == nil {
		return nil, errors.New("http client not found")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("tenant access token cannot be empty")
	}
	if len(emails) == 0 && len(mobiles) == 0 {
		return nil, errors.New("emails and mobiles cannot both be empty")
	}
	if strings.TrimSpace(userIDType) == "" {
		userIDType = "open_id"
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"emails":           emails,
		"mobiles":          mobiles,
		"include_resigned": includeResigned,
	})
	if err != nil {
		return nil, err
	}

	u := feishuBatchUserIDURL + "?user_id_type=" + url.QueryEscape(userIDType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var out struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			UserList []FeishuUserIDItem `json:"user_list"`
		} `json:"data"`
	}
	if err = json.Unmarshal(bs, &out); err != nil {
		return nil, fmt.Errorf("parse feishu batch_get_id response failed: %w, body: %s", err, string(bs))
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("feishu batch_get_id failed: code=%d msg=%s", out.Code, out.Msg)
	}
	return out.Data.UserList, nil
}

var (
	cardJson = `
	{
    "schema": "2.0",
    "config": {
        "update_multi": true,
        "style": {
            "text_size": {
                "normal_v2": {
                    "default": "normal",
                    "pc": "normal",
                    "mobile": "heading"
                }
            }
        }
    },
    "body": {
        "direction": "vertical",
        "elements": [
            {
                "tag": "column_set",
                "flex_mode": "stretch",
                "horizontal_spacing": "12px",
                "horizontal_align": "left",
                "columns": [
                    {
                        "tag": "column",
                        "width": "weighted",
                        "elements": [
                            {
                                "tag": "markdown",
                                "content": {{ jsonMarshal .msg_body }},
                                "text_align": "left",
                                "text_size": "normal_v2"
                            }
                        ],
                        "vertical_spacing": "8px",
                        "horizontal_align": "left",
                        "vertical_align": "top",
                        "weight": 1
                    }
                ],
                "margin": "0px 0px 0px 0px"
            },
            {
                "tag": "hr",
                "margin": "0px 0px 0px 0px",
                "element_id": "e7TwGda0WH4_yR_IkeU5"
            },
            {
                "tag": "column_set",
                "flex_mode": "stretch",
                "horizontal_spacing": "8px",
                "horizontal_align": "left",
                "columns": [
                    {
                        "tag": "column",
                        "width": "auto",
                        "elements": [
                            {
                                "tag": "button",
                                "text": {
                                    "tag": "plain_text",
                                    "content": "查看详情"
                                },
                                "type": "primary_filled",
                                "width": "fill",
                                "behaviors": [
                                    {
                                        "type": "open_url",
                                        "default_url": "https://example.com/alert/handle",
                                        "pc_url": "",
                                        "ios_url": "",
                                        "android_url": ""
                                    }
                                ],
                                "margin": "4px 0px 4px 0px",
                                "element_id": "NVdaRT204HOQPtxfObaI"
                            }
                        ],
                        "vertical_spacing": "8px",
                        "horizontal_align": "left",
                        "vertical_align": "top"
                    },
                    {
                        "tag": "column",
                        "width": "auto",
                        "elements": [
                            {
                                "tag": "button",
                                "text": {
                                    "tag": "plain_text",
                                    "content": "屏蔽"
                                },
                                "type": "default",
                                "width": "fill",
                                "behaviors": [
                                    {
                                        "type": "callback",
                                        "value": ""
                                    }
                                ],
                                "margin": "4px 0px 4px 0px",
                                "element_id": "x8ODoO6HDBcViKTlnDHi"
                            }
                        ],
                        "vertical_spacing": "8px",
                        "horizontal_align": "left",
                        "vertical_align": "top"
                    },
                    {
                        "tag": "column",
                        "width": "auto",
                        "elements": [
                            {
                                "tag": "button",
                                "text": {
                                    "tag": "plain_text",
                                    "content": "关闭"
                                },
                                "type": "default",
                                "width": "fill",
                                "behaviors": [
                                    {
                                        "type": "open_url",
                                        "default_url": "https://example.com/alert/detail",
                                        "pc_url": "",
                                        "ios_url": "",
                                        "android_url": ""
                                    }
                                ],
                                "margin": "4px 0px 4px 0px",
                                "element_id": "xwflabjVxh2qQwphn9rN"
                            }
                        ],
                        "vertical_spacing": "8px",
                        "horizontal_align": "left",
                        "vertical_align": "top"
                    }
                ],
                "margin": "0px 0px 0px 0px"
            }{{ if .shot_image_key }},
            {
                "tag": "img",
                "img_key": {{ jsonMarshal .shot_image_key }},
                "preview": true,
                "transparent": false,
                "scale_type": "fit_horizontal",
                "margin": "0px 0px 0px 0px"
            }{{ end }}
        ]
    },
    "header": {
        "title": {
            "tag": "plain_text",
            "content": {{ jsonMarshal .msg_title }}
        },
        "subtitle": {
            "tag": "plain_text",
            "content": ""
        },
        "text_tag_list": [
            {
                "tag": "text_tag",
                "text": {
                    "tag": "plain_text",
                    "content": "紧急"
                },
                "color": "red"
            }
        ],
        "template": "red",
        "icon": {
            "tag": "standard_icon",
            "token": "alert-circle_outlined"
        },
        "padding": "12px 8px 12px 8px"
    }
}
	`
)
