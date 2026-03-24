package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

var (
	dingtalkAppAccessTokenURL  = "https://api.dingtalk.com/v1.0/oauth2/accessToken"
	dingtalkAppUserByMobileURL = "https://oapi.dingtalk.com/topapi/v2/user/getbymobile"
	dingtalkAppMediaUploadURL  = "https://oapi.dingtalk.com/media/upload"
	dingtalkAppSendMessageURL  = "https://api.dingtalk.com/v1.0/card/instances/createAndDeliver"
)

// DingtalkAppProvider 对接钉钉应用消息发送接口。
// 采用 HTTP 通道发送，支持通过参数传入 access_token 和 agent_id。
type DingtalkAppProvider struct {
	appConfig   *models.DingtalkAppRequestConfig
	AccessToken string
}

func (p *DingtalkAppProvider) Ident() string {
	return "dingtalkapp"
}

func (p *DingtalkAppProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestConfig == nil || config.RequestConfig.DingtalkAppRequestConfig == nil {
		return errors.New("dingtalk app request config cannot be nil")
	}

	appConfig := config.RequestConfig.DingtalkAppRequestConfig
	if strings.TrimSpace(appConfig.AppKey) == "" {
		return errors.New("dingtalk app provider requires app_key")
	}
	if strings.TrimSpace(appConfig.AppSecret) == "" {
		return errors.New("dingtalk app provider requires app_secret")
	}
	if strings.TrimSpace(appConfig.ContactKey) == "" {
		return errors.New("dingtalk app provider requires contact_key")
	}
	if appConfig.Timeout <= 0 {
		appConfig.Timeout = 10000
	}
	if appConfig.RetryTimes <= 0 {
		appConfig.RetryTimes = 1
	}
	if appConfig.RetrySleep < 0 {
		appConfig.RetrySleep = 1
	}

	return nil
}

func (p *DingtalkAppProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req == nil || req.Config == nil || req.Config.RequestConfig == nil || req.Config.RequestConfig.DingtalkAppRequestConfig == nil {
		return &NotifyResult{Err: errors.New("dingtalk app request config cannot be nil")}
	}
	appConfig := req.Config.RequestConfig.DingtalkAppRequestConfig
	p.appConfig = appConfig
	_, err := p.GetAccessToken(ctx, req.HttpClient)
	if err != nil {
		return &NotifyResult{
			Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
			Response: "",
			Err:      err,
		}
	}

	userIDs := make([]string, 0, len(req.Sendtos))
	for _, sendto := range req.Sendtos {
		s := strings.TrimSpace(sendto)
		if s == "" {
			continue
		}
		if isPhoneContactKey(appConfig.ContactKey) {
			uid, userErr := p.GetUserIDByMobile(ctx, req.HttpClient, s)
			if userErr != nil {
				return &NotifyResult{
					Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
					Response: "",
					Err:      userErr,
				}
			}
			userIDs = append(userIDs, uid)
		} else {
			userIDs = append(userIDs, s)
		}
	}
	if len(userIDs) == 0 {
		return &NotifyResult{
			Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
			Response: "",
			Err:      errors.New("no valid dingtalk user id found"),
		}
	}

	title := strings.TrimSpace(fmt.Sprint(req.TplContent["title"]))
	content := strings.TrimSpace(fmt.Sprint(req.TplContent["content"]))
	if title == "" {
		title = "Alert"
	}

	imageBase64 := pickImageBase64(req.CustomParams, req.TplContent)
	mediaID := ""
	if imageBase64 != "" {
		var uploadErr error
		mediaID, uploadErr = p.UploadMedia(ctx, req.HttpClient, "image", imageBase64)
		if uploadErr != nil {
			return &NotifyResult{
				Target:   strings.Join(userIDs, ","),
				Response: "",
				Err:      uploadErr,
			}
		}
	}

	msgResp, sendErr := p.sendInteractiveCardMessage(ctx, req.HttpClient, userIDs, title, content, mediaID, req.CustomParams)
	if sendErr != nil {
		return &NotifyResult{
			Target:   strings.Join(userIDs, ","),
			Response: msgResp,
			Err:      sendErr,
		}
	}

	return &NotifyResult{
		Target:   strings.Join(userIDs, ","),
		Response: msgResp,
		Err:      nil,
	}
}

// GetAccessToken 获取钉钉应用 access_token。
func (p *DingtalkAppProvider) GetAccessToken(ctx context.Context, client *http.Client) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if p.appConfig.AppKey == "" {
		return "", errors.New("app key cannot be empty")
	}
	if p.appConfig.AppSecret == "" {
		return "", errors.New("app secret cannot be empty")
	}

	reqBody, err := json.Marshal(map[string]string{
		"appKey":    p.appConfig.AppKey,
		"appSecret": p.appConfig.AppSecret,
	})
	if err != nil {
		return "", fmt.Errorf("marshal dingtalk gettoken request failed: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dingtalkAppAccessTokenURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		AccessToken string `json:"accessToken"`
	}
	if err = json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("parse dingtalk gettoken response failed: %w, body: %s", err, string(respBytes))
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("dingtalk gettoken failed or accessToken is empty, body: %s", string(respBytes))
	}
	p.AccessToken = result.AccessToken
	return p.AccessToken, nil
}

// GetUserIDByMobile 根据手机号查询钉钉 userid。
func (p *DingtalkAppProvider) GetUserIDByMobile(ctx context.Context, client *http.Client, mobile string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if p.AccessToken == "" {
		return "", errors.New("access token cannot be empty")
	}
	if strings.TrimSpace(mobile) == "" {
		return "", errors.New("mobile cannot be empty")
	}

	reqBody, err := json.Marshal(map[string]string{
		"mobile": mobile,
	})
	if err != nil {
		return "", fmt.Errorf("marshal get user by mobile request failed: %w", err)
	}

	u := fmt.Sprintf("%s?access_token=%s", dingtalkAppUserByMobileURL, url.QueryEscape(p.AccessToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		ErrCode json.RawMessage `json:"errcode"`
		ErrMsg  string          `json:"errmsg"`
		Result  struct {
			UserID string `json:"userid"`
		} `json:"result"`
	}
	if err = json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("parse dingtalk get user by mobile response failed: %w, body: %s", err, string(respBytes))
	}

	// errcode 可能是字符串 "0" 或数字 0，统一按字符串比较。
	errCode := strings.Trim(string(result.ErrCode), "\"")
	if errCode != "0" {
		return "", fmt.Errorf("dingtalk get user by mobile failed: errcode=%s errmsg=%s", errCode, result.ErrMsg)
	}
	if result.Result.UserID == "" {
		return "", fmt.Errorf("dingtalk get user by mobile succeeded but userid is empty, body: %s", string(respBytes))
	}

	return result.Result.UserID, nil
}

// UploadMedia 上传钉钉应用消息媒体文件并返回 media_id。
// mediaType 常见值: image/file/voice。
// imageBase64 支持纯 base64 字符串和 data URL（如 data:image/png;base64,xxxx）。
func (p *DingtalkAppProvider) UploadMedia(ctx context.Context, client *http.Client, mediaType, imageBase64 string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if p.AccessToken == "" {
		return "", errors.New("access token cannot be empty")
	}
	if mediaType == "" {
		mediaType = "image"
	}
	if imageBase64 == "" {
		return "", errors.New("image base64 cannot be empty")
	}

	decoded, err := decodeBase64Payload(imageBase64)
	if err != nil {
		return "", fmt.Errorf("decode image base64 failed: %w", err)
	}
	if len(decoded) == 0 {
		return "", errors.New("decoded image content cannot be empty")
	}

	fileName := defaultMediaFileName(mediaType)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("type", mediaType); err != nil {
		return "", fmt.Errorf("write media type failed: %w", err)
	}
	part, err := writer.CreateFormFile("media", fileName)
	if err != nil {
		return "", fmt.Errorf("create form file failed: %w", err)
	}
	if _, err = part.Write(decoded); err != nil {
		return "", fmt.Errorf("write file content failed: %w", err)
	}
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer failed: %w", err)
	}

	u := fmt.Sprintf("%s?access_token=%s",
		dingtalkAppMediaUploadURL,
		url.QueryEscape(p.AccessToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		MediaID string `json:"media_id"`
	}
	if err = json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("parse dingtalk upload response failed: %w, body: %s", err, string(respBytes))
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk upload media failed: errcode=%d errmsg=%s", result.ErrCode, result.ErrMsg)
	}
	if result.MediaID == "" {
		return "", fmt.Errorf("dingtalk upload media succeeded but media_id is empty, body: %s", string(respBytes))
	}
	return result.MediaID, nil
}

func (p *DingtalkAppProvider) sendInteractiveCardMessage(ctx context.Context, client *http.Client, userIDs []string, title, content, mediaID string, customParams map[string]string) (string, error) {
	cardTemplateID := strings.TrimSpace(customParams["card_template_id"])

	if cardTemplateID == "" {
		return "", errors.New("card_template_id cannot be empty when sending dingtalk interactive card")
	}

	cardData := map[string]string{
		"title":   title,
		"content": content,
	}
	if mediaID != "" {
		cardData["image_media_id"] = mediaID
	}

	isGroup := isGroupContactKey(p.appConfig.ContactKey)
	robotCode := p.appConfig.AppKey

	results := make([]string, 0, len(userIDs))
	for _, userID := range userIDs {
		payload := map[string]interface{}{
			"cardTemplateId": cardTemplateID,
			"outTrackId":     fmt.Sprintf("n9e_%d_%s", time.Now().UnixNano(), userID),
			"cardData": map[string]interface{}{
				"cardParamMap": cardData,
			},
		}
		if isGroup {
			payload["openSpaceId"] = fmt.Sprintf("dtv1.card//IM_GROUP.%s", userID)
			payload["imGroupOpenSpaceModel"] = map[string]bool{"supportForward": true}
			payload["imGroupOpenDeliverModel"] = map[string]string{"robotCode": robotCode}
		} else {
			payload["openSpaceId"] = fmt.Sprintf("dtv1.card//IM_ROBOT.%s", userID)
			payload["imRobotOpenSpaceModel"] = map[string]bool{"supportForward": true}
			payload["imRobotOpenDeliverModel"] = map[string]string{"spaceType": "IM_ROBOT"}
		}

		resp, err := p.sendAppMessage(ctx, client, payload)
		if err != nil {
			return strings.Join(results, "; "), err
		}
		results = append(results, resp)
	}
	return strings.Join(results, "; "), nil
}

func (p *DingtalkAppProvider) sendAppMessage(ctx context.Context, client *http.Client, payload map[string]interface{}) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if p.AccessToken == "" {
		return "", errors.New("access token cannot be empty")
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dingtalkAppSendMessageURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-acs-dingtalk-access-token", p.AccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// 互动卡片新接口返回格式：
	// {"success": true, "result": {"deliverResults":[{"success": true, ...}]}}
	var result struct {
		Success bool `json:"success"`
		Result  struct {
			OutTrackID     string `json:"outTrackId"`
			DeliverResults []struct {
				Success   bool   `json:"success"`
				SpaceType string `json:"spaceType"`
				SpaceID   string `json:"spaceId"`
				CarrierID string `json:"carrierId"`
				ErrorMsg  string `json:"errorMsg"`
			} `json:"deliverResults"`
		} `json:"result"`
	}
	if err = json.Unmarshal(bs, &result); err != nil {
		return "", fmt.Errorf("parse dingtalk send message response failed: %w, body: %s", err, string(bs))
	}
	if !result.Success {
		return string(bs), fmt.Errorf("dingtalk send message failed: success=false body=%s", string(bs))
	}
	if len(result.Result.DeliverResults) > 0 {
		for _, dr := range result.Result.DeliverResults {
			if !dr.Success {
				return string(bs), fmt.Errorf("dingtalk deliver failed: space_id=%s error=%s", dr.SpaceID, dr.ErrorMsg)
			}
		}
	}
	return string(bs), nil
}

func decodeBase64Payload(payload string) ([]byte, error) {
	data := strings.TrimSpace(payload)
	if idx := strings.Index(data, ","); idx >= 0 && strings.Contains(data[:idx], ";base64") {
		data = data[idx+1:]
	}
	return base64.StdEncoding.DecodeString(data)
}

func defaultMediaFileName(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image":
		return "image.png"
	case "voice":
		return "voice.amr"
	case "file":
		return "file.bin"
	default:
		return "media.bin"
	}
}

func isPhoneContactKey(contactKey string) bool {
	key := strings.ToLower(strings.TrimSpace(contactKey))
	return key == models.Phone || strings.Contains(key, "phone") || strings.Contains(key, "mobile")
}

func isGroupContactKey(contactKey string) bool {
	key := strings.ToLower(strings.TrimSpace(contactKey))
	return strings.Contains(key, "group") || strings.Contains(key, "conversation")
}

func pickImageBase64(customParams map[string]string, tpl map[string]interface{}) string {
	if v := strings.TrimSpace(customParams["image_base64"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(customParams["alert_image_base64"]); v != "" {
		return v
	}
	if raw, ok := tpl["image_base64"]; ok {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	if raw, ok := tpl["alert_image_base64"]; ok {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func normalizeErrCode(code json.RawMessage) string {
	return strings.Trim(strings.TrimSpace(string(code)), "\"")
}

func (p *DingtalkAppProvider) DefaultChannels() []*models.NotifyChannelConfig {
	return []*models.NotifyChannelConfig{
		{
			Name: "DingtalkApp", Ident: p.Ident(), RequestType: "http", Weight: 3, Enable: true,
			RequestConfig: &models.RequestConfig{
				DingtalkAppRequestConfig: &models.DingtalkAppRequestConfig{
					AppKey:     "app_key_for_test",
					AppSecret:  "app_secret_for_test",
					ContactKey: "dingtalk_userid",
					Timeout:    10000,
					RetryTimes: 1,
					RetrySleep: 1,
				},
			},
			ParamConfig: &models.NotifyParamConfig{
				Custom: models.Params{
					Params: []models.ParamItem{
						{Key: "card_template_id", CName: "Card Template ID", Type: "string"},
					},
				},
			},
		},
	}
}
