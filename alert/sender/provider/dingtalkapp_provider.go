package provider

// TODO(dingtalkapp): 钉钉应用本次不上线。本文件中的 DingtalkAppProvider 类型及其方法、buildDingtalkAppTplData
// 已被注释（见下方 /* ... */ 包裹段），只保留 GetAccessToken/UploadMedia/pickImageBase64/getMapString
// 等共享工具函数——dingtalk/wecom/feishu 等 Provider 仍会使用这些工具函数。上线时去掉下面的 /* 和 */ 即可恢复。

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
	// TODO(dingtalkapp): time 仅在已注释的 DingtalkAppProvider 方法中使用，上线时一并恢复。
	// "time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

var (
	dingtalkAppAccessTokenURL   = "https://api.dingtalk.com/v1.0/oauth2/accessToken"
	dingtalkAppUserByMobileURL  = "https://oapi.dingtalk.com/topapi/v2/user/getbymobile"
	dingtalkScenarioGroupGetURL = "https://oapi.dingtalk.com/topapi/im/chat/scenegroup/get"
	dingtalkAppMediaUploadURL   = "https://oapi.dingtalk.com/media/upload"
	dingtalkRobotBatchSendURL   = "https://api.dingtalk.com/v1.0/robot/oToMessages/batchSend"
	dingtalkRobotGroupSendURL   = "https://api.dingtalk.com/v1.0/robot/groupMessages/send"
)

// TODO(dingtalkapp): 钉钉应用本次不上线，DingtalkAppProvider 及其 Ident/Check/Notify 整段注释；上线时去掉 /* 和 */。
/*
// DingtalkAppProvider 对接钉钉应用消息发送接口。
// 采用 HTTP 通道发送，支持通过参数传入 access_token 和 agent_id。
type DingtalkAppProvider struct{}

func (p *DingtalkAppProvider) Ident() string {
	return "dingtalkapp"
}

func (p *DingtalkAppProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != p.Ident() {
		return fmt.Errorf("dingtalk app provider requires request_type=%s, got %q", p.Ident(), config.RequestType)
	}
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

	logger.Infof("dingtalkapp notify start: sendtos=%d groups=%d", len(req.Sendtos), len(req.ImGroupIDs))
	accessToken, err := GetAccessToken(ctx, req.HttpClient, appConfig.AppKey, appConfig.AppSecret)
	if err != nil {
		return &NotifyResult{
			Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
			Response: "get access token failed: " + err.Error(),
			Err:      err,
		}
	}
	userIDs := make([]string, 0, len(req.Sendtos))
	contactKey := ""
	if req.Config.ParamConfig != nil && req.Config.ParamConfig.UserInfo != nil {
		contactKey = req.Config.ParamConfig.UserInfo.ContactKey
	}
	// contact key 仅用于把 sendtos 解析成 userid，只发群（im_group_ids）时不需要
	if len(req.Sendtos) > 0 && contactKey == "" {
		return &NotifyResult{
			Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
			Response: "contact key cannot be empty",
			Err:      errors.New("contact key cannot be empty"),
		}
	}
	for _, sendto := range req.Sendtos {
		s := strings.TrimSpace(sendto)
		if s == "" {
			continue
		}
		if isPhoneContactKey(contactKey) {
			uid, userErr := p.GetUserIDByMobile(ctx, req.HttpClient, accessToken, s)
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
		imageMediaID, uploadErr = UploadMedia(ctx, req.HttpClient, accessToken, "image", imageBase64)
		if uploadErr != nil {
			logger.Errorf("dingtalkapp upload image failed: %v", uploadErr)
		}
	}

	parts := make([]string, 0, 2)
	targets := make([]string, 0, len(userIDs)+len(groupIDs))
	targets = append(targets, userIDs...)
	targets = append(targets, groupIDs...)

	if len(userIDs) > 0 {
		// OTO 消息的 robotCode 必须由调用方通过 CustomParams["robot_code"] 指定：
		// 与群消息不同，OTO 没有对应的 dingtalk_group 记录可供查询，AppKey 也不等于 robotCode。
		otoRobotCode := strings.TrimSpace(req.CustomParams["robot_code"])
		if otoRobotCode == "" {
			err := errors.New("dingtalkapp OTO message requires custom_params.robot_code")
			return &NotifyResult{
				Target:   strings.Join(targets, ","),
				Response: strings.Join(parts, "; "),
				Err:      err,
			}
		}
		if imageMediaID != "" {
			imageResp, imageErr := p.sendRobotOTOImageMessage(ctx, req.HttpClient, appConfig, accessToken, otoRobotCode, userIDs, imageMediaID)
			if imageErr != nil {
				return &NotifyResult{
					Target:   strings.Join(targets, ","),
					Response: imageResp,
					Err:      imageErr,
				}
			}
			parts = append(parts, "user_image:"+imageResp)
		}
		msgResp, sendErr := p.sendRobotOTOActionCardMessage(ctx, req.HttpClient, appConfig, accessToken, otoRobotCode, userIDs, title, content, req.CustomParams)
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
		// 群聊 robotCode 来源于 dingtalk_group 表（由 Stream 安装事件写入），按 gid 映射。
		// 映射缺失意味着该群未完成酷应用安装或 RobotCode 为空，发送必然失败，这里直接拦下来。
		if imageMediaID != "" {
			imageResp, imageErr := p.sendRobotGroupImageMessage(ctx, req.HttpClient, appConfig, accessToken, req.ImGroupRobotCodes, groupIDs, imageMediaID)
			if imageErr != nil {
				return &NotifyResult{
					Target:   strings.Join(targets, ","),
					Response: strings.Join(parts, "; "),
					Err:      imageErr,
				}
			}
			parts = append(parts, "group_image:"+imageResp)
		}
		msgResp, sendErr := p.sendRobotGroupActionCardMessage(ctx, req.HttpClient, appConfig, accessToken, req.ImGroupRobotCodes, groupIDs, title, content, req.CustomParams)
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
*/

func UploadMedia(ctx context.Context, client *http.Client, accessToken, mediaType, imageBase64 string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if accessToken == "" {
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

	uploadURL := fmt.Sprintf("%s?access_token=%s", dingtalkAppMediaUploadURL, url.QueryEscape(accessToken))
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
func GetAccessToken(ctx context.Context, client *http.Client, appKey, appSecret string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if appKey == "" {
		return "", errors.New("app key cannot be empty")
	}
	if appSecret == "" {
		return "", errors.New("app secret cannot be empty")
	}

	reqBody, err := json.Marshal(map[string]string{
		"appKey":    appKey,
		"appSecret": appSecret,
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
	return result.AccessToken, nil
}

// TODO(dingtalkapp): 钉钉应用本次不上线，场景群查询 + GetUserIDByMobile + buildRobot* + sendRobot* 一起注释；上线时去掉 /* 和 */。
/*
// DingtalkScenarioGroupInfo 场景群基本信息，对应开放平台「查询群信息」接口 result 中的常用字段。
// 文档：https://open.dingtalk.com/document/development/queries-the-basic-information-of-a-scenario-group
type DingtalkScenarioGroupInfo struct {
	OpenConversationID string   `json:"open_conversation_id"`
	Title              string   `json:"title"`
	Icon               string   `json:"icon"`
	OwnerStaffID       string   `json:"owner_staff_id"`
	GroupURL           string   `json:"group_url"`
	MemberAmount       int      `json:"member_amount"`
	SubAdminStaffIDs   []string `json:"sub_admin_staff_ids"`
}

// GetScenarioGroupInfo 根据 open_conversation_id 查询场景群基本信息（需 chat 相关读权限）。
func GetScenarioGroupInfo(ctx context.Context, client *http.Client, accessToken, openConversationID string) (*DingtalkScenarioGroupInfo, error) {
	if client == nil {
		return nil, errors.New("http client not found")
	}
	if strings.TrimSpace(accessToken) == "" {
		return nil, errors.New("access token cannot be empty")
	}
	if strings.TrimSpace(openConversationID) == "" {
		return nil, errors.New("open_conversation_id cannot be empty")
	}

	reqBody, err := json.Marshal(map[string]string{
		"open_conversation_id": strings.TrimSpace(openConversationID),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal scenegroup get request failed: %w", err)
	}

	u := fmt.Sprintf("%s?access_token=%s", dingtalkScenarioGroupGetURL, url.QueryEscape(accessToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var envelope struct {
		ErrCode json.RawMessage            `json:"errcode"`
		ErrMsg  string                     `json:"errmsg"`
		Result  *DingtalkScenarioGroupInfo `json:"result"`
	}
	if err = json.Unmarshal(respBytes, &envelope); err != nil {
		return nil, fmt.Errorf("parse dingtalk scenegroup get response failed: %w, body: %s", err, string(respBytes))
	}
	if code := normalizedDingtalkCode(envelope.ErrCode); code != "" && code != "0" {
		return nil, fmt.Errorf("dingtalk scenegroup get failed: errcode=%s errmsg=%s", code, envelope.ErrMsg)
	}
	if envelope.Result == nil {
		return nil, fmt.Errorf("dingtalk scenegroup get succeeded but result is empty, body: %s", string(respBytes))
	}
	return envelope.Result, nil
}

// GetUserIDByMobile 根据手机号查询钉钉 userid。
func (p *DingtalkAppProvider) GetUserIDByMobile(ctx context.Context, client *http.Client, accessToken, mobile string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if accessToken == "" {
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

	u := fmt.Sprintf("%s?access_token=%s", dingtalkAppUserByMobileURL, url.QueryEscape(accessToken))
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

func (p *DingtalkAppProvider) sendRobotOTOActionCardMessage(ctx context.Context, client *http.Client, appConfig *models.DingtalkAppRequestConfig, accessToken, robotCode string, userIDs []string, title, content string, customParams map[string]string) (string, error) {
	if strings.TrimSpace(robotCode) == "" {
		return "", errors.New("robot_code cannot be empty when sending dingtalk robot oto message")
	}
	msgKey, msgParam := p.buildRobotActionCardMessage(title, content, customParams)
	payload := map[string]interface{}{
		"robotCode": robotCode,
		"userIds":   userIDs,
		"msgKey":    msgKey,
		"msgParam":  msgParam,
	}
	return p.sendRobotMessage(ctx, client, appConfig, accessToken, dingtalkRobotBatchSendURL, payload)
}

func (p *DingtalkAppProvider) sendRobotOTOImageMessage(ctx context.Context, client *http.Client, appConfig *models.DingtalkAppRequestConfig, accessToken, robotCode string, userIDs []string, mediaID string) (string, error) {
	if strings.TrimSpace(robotCode) == "" {
		return "", errors.New("robot_code cannot be empty when sending dingtalk robot oto image")
	}
	msgParamBytes, _ := json.Marshal(map[string]string{"mediaId": mediaID})
	payload := map[string]interface{}{
		"robotCode": robotCode,
		"userIds":   userIDs,
		"msgKey":    "sampleImageMsg",
		"msgParam":  string(msgParamBytes),
	}
	return p.sendRobotMessage(ctx, client, appConfig, accessToken, dingtalkRobotBatchSendURL, payload)
}

func (p *DingtalkAppProvider) sendRobotGroupActionCardMessage(ctx context.Context, client *http.Client, appConfig *models.DingtalkAppRequestConfig, accessToken string, robotCodes map[string]string, groupIDs []string, title, content string, customParams map[string]string) (string, error) {
	msgKey, msgParam := p.buildRobotActionCardMessage(title, content, customParams)
	results := make([]string, 0, len(groupIDs))
	for _, gid := range groupIDs {
		robotCode := strings.TrimSpace(robotCodes[gid])
		if robotCode == "" {
			return strings.Join(results, "; "),
				fmt.Errorf("dingtalk robot_code missing for open_conversation_id=%s; 请确认酷应用已安装到该群", gid)
		}
		payload := map[string]interface{}{
			"robotCode":          robotCode,
			"openConversationId": gid,
			"msgKey":             msgKey,
			"msgParam":           msgParam,
		}
		resp, err := p.sendRobotMessage(ctx, client, appConfig, accessToken, dingtalkRobotGroupSendURL, payload)
		if err != nil {
			return strings.Join(results, "; "), err
		}
		results = append(results, resp)
	}
	return strings.Join(results, "; "), nil
}

func (p *DingtalkAppProvider) sendRobotGroupImageMessage(ctx context.Context, client *http.Client, appConfig *models.DingtalkAppRequestConfig, accessToken string, robotCodes map[string]string, groupIDs []string, mediaID string) (string, error) {
	msgParamBytes, _ := json.Marshal(map[string]string{"mediaId": mediaID})
	results := make([]string, 0, len(groupIDs))
	for _, gid := range groupIDs {
		robotCode := strings.TrimSpace(robotCodes[gid])
		if robotCode == "" {
			return strings.Join(results, "; "),
				fmt.Errorf("dingtalk robot_code missing for open_conversation_id=%s; 请确认酷应用已安装到该群", gid)
		}
		payload := map[string]interface{}{
			"robotCode":          robotCode,
			"openConversationId": gid,
			"msgKey":             "sampleImageMsg",
			"msgParam":           string(msgParamBytes),
		}
		resp, err := p.sendRobotMessage(ctx, client, appConfig, accessToken, dingtalkRobotGroupSendURL, payload)
		if err != nil {
			return strings.Join(results, "; "), err
		}
		results = append(results, resp)
	}
	return strings.Join(results, "; "), nil
}

func (p *DingtalkAppProvider) sendRobotMessage(ctx context.Context, client *http.Client, appConfig *models.DingtalkAppRequestConfig, accessToken, endpoint string, payload map[string]interface{}) (string, error) {
	return p.sendAppMessage(ctx, client, appConfig, accessToken, endpoint, payload)
}

func (p *DingtalkAppProvider) sendAppMessage(ctx context.Context, client *http.Client, appConfig *models.DingtalkAppRequestConfig, accessToken, endpoint string, payload map[string]interface{}) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if accessToken == "" {
		return "", errors.New("access token cannot be empty")
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	retrySleep := time.Second
	if appConfig != nil && appConfig.RetrySleep > 0 {
		retrySleep = time.Duration(appConfig.RetrySleep) * time.Millisecond
	}
	retryTimes := 3
	if appConfig != nil && appConfig.RetryTimes > 0 {
		retryTimes = appConfig.RetryTimes
	}
	var lastErrorMessage string
	for i := 0; i <= retryTimes; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-acs-dingtalk-access-token", accessToken)

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
*/

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

// TODO(dingtalkapp): 钉钉应用本次不上线，buildDingtalkAppTplData 仅 Notify 使用，随 Provider 一起注释。
/*
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
*/

