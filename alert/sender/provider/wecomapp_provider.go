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
	"github.com/toolkits/pkg/logger"
)

const (
	wecomGetTokenURL    = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"
	wecomGetUserIDURL   = "https://qyapi.weixin.qq.com/cgi-bin/user/getuserid"
	wecomGetUserByEmail = "https://qyapi.weixin.qq.com/cgi-bin/user/get_userid_by_email"
	wecomMessageSendURL = "https://qyapi.weixin.qq.com/cgi-bin/message/send"
	wecomMarkdownMaxLen = 4096
)

// WecomAppProvider 企业微信应用消息：成员使用 message/send（markdown）。
type WecomAppProvider struct{}

func (p *WecomAppProvider) Ident() string { return "wecomapp" }

func (p *WecomAppProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != p.Ident() {
		return fmt.Errorf("wecom app provider requires request_type=%s, got %q", p.Ident(), config.RequestType)
	}
	if config.RequestConfig == nil || config.RequestConfig.WecomAppRequestConfig == nil {
		return errors.New("wecom app request config cannot be nil")
	}
	c := config.RequestConfig.WecomAppRequestConfig
	if strings.TrimSpace(c.CorpID) == "" {
		return errors.New("wecom app provider requires corp_id")
	}
	if strings.TrimSpace(c.CorpSecret) == "" {
		return errors.New("wecom app provider requires corp_secret")
	}
	if c.AgentID <= 0 {
		return errors.New("wecom app provider requires agent_id > 0")
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

func (p *WecomAppProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	if req == nil || req.Config == nil || req.Config.RequestConfig == nil || req.Config.RequestConfig.WecomAppRequestConfig == nil {
		return &NotifyResult{Err: errors.New("wecom app request config cannot be nil")}
	}
	appConfig := req.Config.RequestConfig.WecomAppRequestConfig

	token, err := p.getAccessToken(ctx, req.HttpClient, appConfig)
	if err != nil {
		return &NotifyResult{
			Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
			Response: "get access token failed: " + err.Error(),
			Err:      fmt.Errorf("wecom get access_token: %w", err),
		}
	}
	contactKey := ""
	if req.Config.ParamConfig != nil && req.Config.ParamConfig.UserInfo != nil && req.Config.ParamConfig.UserInfo.ContactKey != "" {
		contactKey = req.Config.ParamConfig.UserInfo.ContactKey
	}

	userIDs := make([]string, 0, len(req.Sendtos))
	for _, sendto := range req.Sendtos {
		s := strings.TrimSpace(sendto)
		if s == "" {
			continue
		}

		if isPhoneContactKey(contactKey) {
			uid, e := p.getUserIDByMobile(ctx, req.HttpClient, token, s)
			if e != nil {
				return &NotifyResult{
					Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
					Response: "",
					Err:      fmt.Errorf("wecom get userid by mobile: %w", e),
				}
			}
			userIDs = append(userIDs, uid)
		} else if isEmailContactKey(contactKey) {
			uid, e := p.getUserIDByEmail(ctx, req.HttpClient, token, s)
			if e != nil {
				return &NotifyResult{
					Target:   getNotifyTarget(req.CustomParams, req.Sendtos),
					Response: "get userid by email failed: " + e.Error(),
					Err:      fmt.Errorf("wecom get userid by email: %w", e),
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
			Response: "no valid wecom target found",
			Err:      errors.New("no valid wecom target found"),
		}
	}

	tplData := buildWecomAppTplData(req, userIDs)
	title := getMapString(req.TplContent, "title")
	content := getMapString(req.TplContent, "content")
	if needsTemplateRendering(title) {
		title = getParsedString("wecomapp_title", title, tplData)
	}
	if needsTemplateRendering(content) {
		content = getParsedString("wecomapp_content", content, tplData)
	}
	if title == "" {
		title = "Alert"
	}
	markdown := buildWecomMarkdownContent(title, content)

	targets := make([]string, 0, len(userIDs))
	resps := make([]string, 0, 1)

	if len(userIDs) > 0 {
		targets = append(targets, userIDs...)
		touser := strings.Join(userIDs, "|")
		resp, sendErr := p.sendMarkdownToUsers(ctx, req.HttpClient, appConfig, token, touser, markdown)
		if sendErr != nil {
			return &NotifyResult{Target: strings.Join(targets, ","), Response: resp, Err: sendErr}
		}
		resps = append(resps, "user:"+resp)
	}
	return &NotifyResult{Target: strings.Join(targets, ","), Response: strings.Join(resps, "; "), Err: nil}
}

func buildWecomAppTplData(req *NotifyRequest, userIDs []string) map[string]interface{} {
	data := map[string]interface{}{
		"tpl":        req.TplContent,
		"params":     req.CustomParams,
		"events":     req.Events,
		"sendtos":    req.Sendtos,
		"imGroupIDs": req.ImGroupIDs,
		"userIDs":    userIDs,
	}
	if len(req.Events) > 0 {
		data["event"] = req.Events[0]
	}
	if len(req.Sendtos) > 0 {
		data["sendto"] = req.Sendtos[0]
	}
	return data
}

func buildWecomMarkdownContent(title, body string) string {
	var b strings.Builder
	if title != "" {
		b.WriteString("## ")
		b.WriteString(title)
		b.WriteString("\n\n")
	}
	b.WriteString(body)
	return truncateWecomMarkdown(b.String())
}

func truncateWecomMarkdown(s string) string {
	if len(s) <= wecomMarkdownMaxLen {
		return s
	}
	s = s[:wecomMarkdownMaxLen]
	for len(s) > 0 && s[len(s)-1]&0xc0 == 0x80 {
		s = s[:len(s)-1]
	}
	return s
}

func (p *WecomAppProvider) getAccessToken(ctx context.Context, client *http.Client, cfg *models.WecomAppRequestConfig) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	u, err := url.Parse(wecomGetTokenURL)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("corpid", cfg.CorpID)
	q.Set("corpsecret", cfg.CorpSecret)
	u.RawQuery = q.Encode()

	var lastErr error
	for range cfg.RetryTimes {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(cfg.RetrySleep) * time.Millisecond)
			continue
		}
		bs, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(cfg.RetrySleep) * time.Millisecond)
			continue
		}
		logger.Infof("wecom gettoken response: %s", string(bs))
		var out struct {
			ErrCode     int    `json:"errcode"`
			ErrMsg      string `json:"errmsg"`
			AccessToken string `json:"access_token"`
		}
		if err = json.Unmarshal(bs, &out); err != nil {
			lastErr = fmt.Errorf("parse wecom token response: %w, body: %s", err, string(bs))
			time.Sleep(time.Duration(cfg.RetrySleep) * time.Millisecond)
			continue
		}
		if out.ErrCode != 0 || out.AccessToken == "" {
			lastErr = fmt.Errorf("wecom gettoken failed: errcode=%d errmsg=%s", out.ErrCode, out.ErrMsg)
			time.Sleep(time.Duration(cfg.RetrySleep) * time.Millisecond)
			continue
		}
		return out.AccessToken, nil
	}
	if lastErr == nil {
		lastErr = errors.New("wecom gettoken failed after retries")
	}
	return "", lastErr
}

func (p *WecomAppProvider) getUserIDByMobile(ctx context.Context, client *http.Client, accessToken, mobile string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if accessToken == "" {
		return "", errors.New("access token cannot be empty")
	}
	body, err := json.Marshal(map[string]string{"mobile": mobile})
	if err != nil {
		return "", err
	}
	u := fmt.Sprintf("%s?access_token=%s", wecomGetUserIDURL, url.QueryEscape(accessToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
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
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		UserID  string `json:"userid"`
	}
	if err = json.Unmarshal(bs, &out); err != nil {
		return "", fmt.Errorf("parse wecom getuserid response: %w, body: %s", err, string(bs))
	}
	if out.ErrCode != 0 || out.UserID == "" {
		return "", fmt.Errorf("wecom getuserid failed: errcode=%d errmsg=%s body=%s", out.ErrCode, out.ErrMsg, string(bs))
	}
	return out.UserID, nil
}

func (p *WecomAppProvider) getUserIDByEmail(ctx context.Context, client *http.Client, accessToken, email string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	if accessToken == "" {
		return "", errors.New("access token cannot be empty")
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return "", errors.New("email cannot be empty")
	}
	body, err := json.Marshal(map[string]interface{}{
		"email":      email,
		"email_type": 1, // 默认企业邮箱
	})
	if err != nil {
		return "", err
	}
	u := fmt.Sprintf("%s?access_token=%s", wecomGetUserByEmail, url.QueryEscape(accessToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
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
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
		UserID  string `json:"userid"`
	}
	if err = json.Unmarshal(bs, &out); err != nil {
		return "", fmt.Errorf("parse wecom get_userid_by_email response: %w, body: %s", err, string(bs))
	}
	if out.ErrCode != 0 || out.UserID == "" {
		return "", fmt.Errorf("wecom get_userid_by_email failed: errcode=%d errmsg=%s body=%s", out.ErrCode, out.ErrMsg, string(bs))
	}
	return out.UserID, nil
}

func isEmailContactKey(contactKey string) bool {
	key := strings.ToLower(strings.TrimSpace(contactKey))
	return key == models.Email || strings.Contains(key, "email")
}

func (p *WecomAppProvider) sendMarkdownToUsers(ctx context.Context, client *http.Client, cfg *models.WecomAppRequestConfig, accessToken, touser, markdown string) (string, error) {
	if client == nil {
		return "", errors.New("http client not found")
	}
	payload := map[string]interface{}{
		"touser":  touser,
		"msgtype": "markdown",
		"agentid": cfg.AgentID,
		"markdown": map[string]string{
			"content": markdown,
		},
	}
	return p.postWecomAPI(ctx, client, cfg, accessToken, wecomMessageSendURL, payload)
}

func (p *WecomAppProvider) postWecomAPI(ctx context.Context, client *http.Client, cfg *models.WecomAppRequestConfig, accessToken, apiURL string, payload interface{}) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	u := fmt.Sprintf("%s?access_token=%s", apiURL, url.QueryEscape(accessToken))

	var lastErr error
	var lastBody string
	for range cfg.RetryTimes {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(cfg.RetrySleep) * time.Millisecond)
			continue
		}
		bs, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(cfg.RetrySleep) * time.Millisecond)
			continue
		}
		lastBody = string(bs)
		logger.Debugf("wecom api %s response: %s", apiURL, lastBody)
		var out struct {
			ErrCode     int    `json:"errcode"`
			ErrMsg      string `json:"errmsg"`
			InvalidUser string `json:"invaliduser"`
		}
		if err = json.Unmarshal(bs, &out); err != nil {
			lastErr = fmt.Errorf("parse wecom response: %w", err)
			time.Sleep(time.Duration(cfg.RetrySleep) * time.Millisecond)
			continue
		}
		if out.ErrCode != 0 {
			lastErr = fmt.Errorf("wecom api error: errcode=%d errmsg=%s", out.ErrCode, out.ErrMsg)
			time.Sleep(time.Duration(cfg.RetrySleep) * time.Millisecond)
			continue
		}
		if out.InvalidUser != "" {
			lastBody = fmt.Sprintf("%s (invaliduser=%s)", lastBody, out.InvalidUser)
		}
		return lastBody, nil
	}
	if lastErr == nil {
		lastErr = errors.New("wecom api failed after retries")
	}
	return lastBody, lastErr
}
