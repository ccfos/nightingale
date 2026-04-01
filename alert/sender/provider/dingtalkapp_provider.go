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
	"github.com/toolkits/pkg/logger"
)

var (
	dingtalkAppAccessTokenURL  = "https://api.dingtalk.com/v1.0/oauth2/accessToken"
	dingtalkAppUserByMobileURL = "https://oapi.dingtalk.com/topapi/v2/user/getbymobile"
	dingtalkAppMediaUploadURL  = "https://oapi.dingtalk.com/media/upload"
	dingtalkRobotBatchSendURL  = "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"
	dingtalkRobotGroupSendURL  = "https://api.dingtalk.com/v1.0/robot/groupMessages/send"
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
	logger.Infof("dingtalkapp notify start: sendtos=%d groups=%d contact_key=%s", len(req.Sendtos), len(req.ImGroupIDs), appConfig.ContactKey)
	_, err := p.GetAccessToken(ctx, req.HttpClient)
	if err != nil {
		return &NotifyResult{
			Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
			Response: "get access token failed: " + err.Error(),
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
					Response: "get user id by mobile failed: " + userErr.Error(),
					Err:      userErr,
				}
			}
			userIDs = append(userIDs, uid)
		} else {
			userIDs = append(userIDs, s)
		}
	}
	groupIDs := make([]string, 0, len(req.ImGroupIDs))
	for _, gid := range req.ImGroupIDs {
		if s := strings.TrimSpace(gid); s != "" {
			groupIDs = append(groupIDs, s)
		}
	}

	if len(userIDs) == 0 && len(groupIDs) == 0 {
		return &NotifyResult{
			Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
			Response: "",
			Err:      errors.New("no valid dingtalk target found"),
		}
	}

	tplData := buildDingtalkAppTplData(req, userIDs, groupIDs)
	title := getMapString(req.TplContent, "title")
	content := getMapString(req.TplContent, "content")
	if needsTemplateRendering(title) {
		title = getParsedString("dingtalkapp_title", title, tplData)
	}
	if needsTemplateRendering(content) {
		content = getParsedString("dingtalkapp_content", content, tplData)
	}
	if title == "" {
		title = "Alert"
	}
	if content == "" {
		content = "-"
	}
	imageMediaID := ""
	imageBase64 := pickImageBase64(req.Events)
	if imageBase64 != "" {
		var uploadErr error
		imageMediaID, uploadErr = p.UploadMedia(ctx, req.HttpClient, "image", imageBase64)
		if uploadErr != nil {
			logger.Errorf("dingtalkapp upload image failed: %v", uploadErr)
			return &NotifyResult{
				Target:   strings.Join(append(userIDs, groupIDs...), ","),
				Response: "",
				Err:      uploadErr,
			}
		}
	}

	parts := make([]string, 0, 2)
	targets := make([]string, 0, len(userIDs)+len(groupIDs))
	targets = append(targets, userIDs...)
	targets = append(targets, groupIDs...)

	if len(userIDs) > 0 {
		if imageMediaID != "" {
			imageResp, imageErr := p.sendRobotOTOImageMessage(ctx, req.HttpClient, userIDs, imageMediaID, req.CustomParams)
			if imageErr != nil {
				return &NotifyResult{
					Target:   strings.Join(targets, ","),
					Response: imageResp,
					Err:      imageErr,
				}
			}
			parts = append(parts, "user_image:"+imageResp)
		}
		msgResp, sendErr := p.sendRobotOTOActionCardMessage(ctx, req.HttpClient, userIDs, title, content, req.CustomParams)
		if sendErr != nil {
			return &NotifyResult{
				Target:   strings.Join(targets, ","),
				Response: msgResp,
				Err:      sendErr,
			}
		}
		parts = append(parts, "user:"+msgResp)
	}
	if len(groupIDs) > 0 {
		if imageMediaID != "" {
			imageResp, imageErr := p.sendRobotGroupImageMessage(ctx, req.HttpClient, groupIDs, imageMediaID, req.CustomParams)
			if imageErr != nil {
				return &NotifyResult{
					Target:   strings.Join(targets, ","),
					Response: strings.Join(parts, "; "),
					Err:      imageErr,
				}
			}
			parts = append(parts, "group_image:"+imageResp)
		}
		msgResp, sendErr := p.sendRobotGroupActionCardMessage(ctx, req.HttpClient, groupIDs, title, content, req.CustomParams)
		if sendErr != nil {
			return &NotifyResult{
				Target:   strings.Join(targets, ","),
				Response: strings.Join(parts, "; "),
				Err:      sendErr,
			}
		}
		parts = append(parts, "group:"+msgResp)
	}

	logger.Infof("dingtalkapp notify success: target_count=%d", len(targets))
	return &NotifyResult{
		Target:   strings.Join(targets, ","),
		Response: strings.Join(parts, "; "),
		Err:      nil,
	}
}

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

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err = writer.WriteField("type", mediaType); err != nil {
		return "", fmt.Errorf("write media type failed: %w", err)
	}
	part, err := writer.CreateFormFile("media", defaultMediaFileName(mediaType))
	if err != nil {
		return "", fmt.Errorf("create form file failed: %w", err)
	}
	if _, err = part.Write(decoded); err != nil {
		return "", fmt.Errorf("write image content failed: %w", err)
	}
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("close multipart writer failed: %w", err)
	}

	uploadURL := fmt.Sprintf("%s?access_token=%s", dingtalkAppMediaUploadURL, url.QueryEscape(p.AccessToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
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
		ErrCode json.RawMessage `json:"errcode"`
		ErrMsg  string          `json:"errmsg"`
		MediaID string          `json:"media_id"`
	}
	if err = json.Unmarshal(bs, &out); err != nil {
		return "", fmt.Errorf("parse dingtalk upload media response failed: %w, body: %s", err, string(bs))
	}
	if code := normalizedDingtalkCode(out.ErrCode); code != "" && code != "0" {
		return "", fmt.Errorf("dingtalk upload media failed: code=%s message=%s", code, out.ErrMsg)
	}
	if strings.TrimSpace(out.MediaID) == "" {
		return "", fmt.Errorf("dingtalk upload media got empty media_id, body: %s", string(bs))
	}
	logger.Infof("dingtalkapp upload media success: media_id=%s", out.MediaID)
	return strings.TrimSpace(out.MediaID), nil
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
	logger.Infof("dingtalkapp get access token success")
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

func (p *DingtalkAppProvider) buildRobotActionCardMessage(title, content string, customParams map[string]string) (string, string) {
	singleTitle := strings.TrimSpace(customParams["single_title"])
	if singleTitle == "" {
		singleTitle = "查看详情"
	}
	singleURL := strings.TrimSpace(customParams["single_url"])
	if singleURL == "" {
		singleURL = "https://www.dingtalk.com/"
	}
	msgParamObj := map[string]string{
		"title":       title,
		"text":        content,
		"singleTitle": singleTitle,
		"singleURL":   singleURL,
	}
	msgParamBytes, _ := json.Marshal(msgParamObj)
	return "sampleActionCard", string(msgParamBytes)
}

func (p *DingtalkAppProvider) sendRobotOTOActionCardMessage(ctx context.Context, client *http.Client, userIDs []string, title, content string, customParams map[string]string) (string, error) {
	robotCode := strings.TrimSpace(p.appConfig.AppKey)
	if robotCode == "" {
		return "", errors.New("app_key cannot be empty when sending dingtalk robot oto message")
	}
	msgKey, msgParam := p.buildRobotActionCardMessage(title, content, customParams)
	payload := map[string]interface{}{
		"robotCode": robotCode,
		"userIds":   userIDs,
		"msgKey":    msgKey,
		"msgParam":  msgParam,
	}
	return p.sendRobotMessage(ctx, client, dingtalkRobotBatchSendURL, payload)
}

func (p *DingtalkAppProvider) sendRobotOTOImageMessage(ctx context.Context, client *http.Client, userIDs []string, mediaID string, customParams map[string]string) (string, error) {
	robotCode := strings.TrimSpace(p.appConfig.AppKey)
	if robotCode == "" {
		return "", errors.New("app_key cannot be empty when sending dingtalk robot oto image")
	}
	msgParamBytes, _ := json.Marshal(map[string]string{"mediaId": mediaID})
	payload := map[string]interface{}{
		"robotCode": robotCode,
		"userIds":   userIDs,
		"msgKey":    "sampleImageMsg",
		"msgParam":  string(msgParamBytes),
	}
	return p.sendRobotMessage(ctx, client, dingtalkRobotBatchSendURL, payload)
}

func (p *DingtalkAppProvider) sendRobotGroupActionCardMessage(ctx context.Context, client *http.Client, groupIDs []string, title, content string, customParams map[string]string) (string, error) {
	robotCode := strings.TrimSpace(p.appConfig.AppKey)
	if robotCode == "" {
		return "", errors.New("app_key cannot be empty when sending dingtalk robot group message")
	}
	msgKey, msgParam := p.buildRobotActionCardMessage(title, content, customParams)
	results := make([]string, 0, len(groupIDs))
	for _, gid := range groupIDs {
		payload := map[string]interface{}{
			"robotCode":          robotCode,
			"openConversationId": gid,
			"msgKey":             msgKey,
			"msgParam":           msgParam,
		}
		resp, err := p.sendRobotMessage(ctx, client, dingtalkRobotGroupSendURL, payload)
		if err != nil {
			return strings.Join(results, "; "), err
		}
		results = append(results, resp)
	}
	return strings.Join(results, "; "), nil
}

func (p *DingtalkAppProvider) sendRobotGroupImageMessage(ctx context.Context, client *http.Client, groupIDs []string, mediaID string, customParams map[string]string) (string, error) {
	robotCode := strings.TrimSpace(p.appConfig.AppKey)
	if robotCode == "" {
		return "", errors.New("app_key cannot be empty when sending dingtalk robot group image")
	}
	msgParamBytes, _ := json.Marshal(map[string]string{"mediaId": mediaID})
	results := make([]string, 0, len(groupIDs))
	for _, gid := range groupIDs {
		payload := map[string]interface{}{
			"robotCode":          robotCode,
			"openConversationId": gid,
			"msgKey":             "sampleImageMsg",
			"msgParam":           string(msgParamBytes),
		}
		resp, err := p.sendRobotMessage(ctx, client, dingtalkRobotGroupSendURL, payload)
		if err != nil {
			return strings.Join(results, "; "), err
		}
		results = append(results, resp)
	}
	return strings.Join(results, "; "), nil
}

func (p *DingtalkAppProvider) sendRobotMessage(ctx context.Context, client *http.Client, endpoint string, payload map[string]interface{}) (string, error) {
	return p.sendAppMessage(ctx, client, endpoint, payload)
}

func (p *DingtalkAppProvider) sendAppMessage(ctx context.Context, client *http.Client, endpoint string, payload map[string]interface{}) (string, error) {
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

	retrySleep := time.Second
	if p.appConfig != nil && p.appConfig.RetrySleep > 0 {
		retrySleep = time.Duration(p.appConfig.RetrySleep) * time.Millisecond
	}
	retryTimes := 3
	if p.appConfig != nil && p.appConfig.RetryTimes > 0 {
		retryTimes = p.appConfig.RetryTimes
	}
	var lastErrorMessage string
	for i := 0; i <= retryTimes; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-acs-dingtalk-access-token", p.AccessToken)

		resp, err := client.Do(req)
		if err != nil {
			lastErrorMessage = err.Error()
			if i < retryTimes {
				time.Sleep(retrySleep)
			}
			continue
		}

		bs, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErrorMessage = readErr.Error()
			if i < retryTimes {
				time.Sleep(retrySleep)
				continue
			}
			return "", readErr
		}

		var result struct {
			ErrCode json.RawMessage `json:"errcode"`
			ErrMsg  string          `json:"errmsg"`
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
			Success *bool           `json:"success"`
		}
		if err = json.Unmarshal(bs, &result); err != nil {
			return "", fmt.Errorf("parse dingtalk send message response failed: %w, body: %s", err, string(bs))
		}
		code := normalizedDingtalkCode(result.ErrCode)
		if code == "" {
			code = normalizedDingtalkCode(result.Code)
		}
		if code != "" && code != "0" {
			msg := strings.TrimSpace(result.ErrMsg)
			if msg == "" {
				msg = result.Message
			}
			return string(bs), fmt.Errorf("dingtalk send message failed: code=%s message=%s", code, msg)
		}
		if result.Success != nil && !*result.Success {
			msg := strings.TrimSpace(result.ErrMsg)
			if msg == "" {
				msg = result.Message
			}
			return string(bs), fmt.Errorf("dingtalk send message failed: success=false message=%s", msg)
		}
		return string(bs), nil
	}

	logger.Errorf("dingtalkapp send failed after retries: endpoint=%s last_error=%s", endpoint, lastErrorMessage)
	return lastErrorMessage, errors.New("failed to send dingtalk action_card message")
}

func decodeBase64Payload(payload string) ([]byte, error) {
	data := strings.TrimSpace(payload)
	if idx := strings.Index(data, ","); idx >= 0 && strings.Contains(data[:idx], ";base64") {
		data = data[idx+1:]
	}
	return base64.StdEncoding.DecodeString(data)
}

func normalizedDingtalkCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	return strings.Trim(strings.TrimSpace(string(raw)), "\"")
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

func pickImageBase64(events []*models.AlertCurEvent) string {
	// 优先从事件注解中提取图片字段。
	for _, evt := range events {
		if evt == nil {
			continue
		}
		for _, image := range evt.ShotImageBase64 {
			if v := strings.TrimSpace(image); v != "" {
				return v
			}
		}
	}

	return ""
}

func getMapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func buildDingtalkAppTplData(req *NotifyRequest, userIDs, groupIDs []string) map[string]interface{} {
	data := map[string]interface{}{
		"tpl":        req.TplContent,
		"params":     req.CustomParams,
		"events":     req.Events,
		"sendtos":    req.Sendtos,
		"imGroupIDs": req.ImGroupIDs,
		"userIDs":    userIDs,
		"groupIDs":   groupIDs,
	}
	if len(req.Events) > 0 {
		data["event"] = req.Events[0]
	}
	if len(req.Sendtos) > 0 {
		data["sendto"] = req.Sendtos[0]
	}
	return data
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
						{Key: "single_title", CName: "Action Button Title", Type: "string"},
						{Key: "single_url", CName: "Action Jump URL", Type: "string"},
					},
				},
			},
		},
	}
}
