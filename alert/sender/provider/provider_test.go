package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

// 创建测试事件
var events = []*models.AlertCurEvent{
	{
		Id:           1,
		Hash:         "test-hash",
		RuleName:     "夜莺告警单元测试",
		RuleNote:     "这是一个测试告警规则",
		Severity:     3,
		GroupId:      1,
		GroupName:    "测试业务组",
		TriggerTime:  time.Now().Unix(),
		TriggerValue: "100",
		TagsMap: map[string]string{
			"host":     "test-host",
			"app":      "test-app",
			"service":  "test-service",
			"env":      "test",
			"instance": "127.0.0.1",
		},
		RuleConfigJson: map[string]interface{}{
			"summary":     "夜莺告警测试",
			"description": "这是一个详细的告警描述",
		},
		AnnotationsJSON: map[string]string{
			"summary":     "测试告警摘要",
			"description": "这是一个详细的告警描述",
		},
		Target: &models.Target{
			Ident: "test-target",
		},
		NotifyGroupsObj: []*models.UserGroup{
			{
				Name: "运维组",
			},
		},
		FirstTriggerTime: time.Now().Unix() - 3600, // 1小时前首次触发
	},
}

func TestSendDingTalkNotification(t *testing.T) {
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Skipf("跳过：读取 ../.env.json 失败: %v", err)
	}
	// 创建钉钉通知配置
	notifyChannel := &models.NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://oapi.dingtalk.com/robot/send", // 使用测试服务器的URL
				Timeout: 10000,
				Request: models.RequestDetail{
					Body: `{"msgtype": "markdown", "markdown": {"title": "{{$tpl.title}}", "text": "{{$tpl.content}}\n{{batchContactsAts $sendtos}}"}, "at": {"atMobiles": {{batchContactsJsonMarshal $sendtos}} }}`,
					Parameters: map[string]string{
						"access_token": "{{ $params.access_token }}",
					},
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				RetryTimes:    2,
				RetryInterval: 1,
			},
		},
		ParamConfig: &models.NotifyParamConfig{
			Custom: models.Params{
				Params: []models.ParamItem{
					{
						Key: "access_token",
					},
				},
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"title":   "测试告警消息",
		"content": "测试告警消息",
	}

	// 创建通知参数
	params := map[string]string{
		"access_token": data["DingTalkAccessToken"],
	}

	// 创建HTTP客户端
	client, err := models.GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := SendHTTPRequest(notifyChannel.RequestConfig.HTTPRequestConfig, events, tpl, params, []string{data["Phone"]}, client)
	if err != nil {
		t.Fatalf("SendHTTP failed: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "errmsg") {
		t.Errorf("Response does not contain expected content, got: %s", resp)
	}
}

// TestSendWecomNotificationWithImage 真实调用企业微信机器人：
// 先发 markdown，再发 image（当事件里有截图时）。
// 需要在 alert/sender/.env.json 配置 WecomBotKey。
func TestSendWecomNotificationWithImage(t *testing.T) {
	env := readSenderDotEnv(t)
	wecomBotKey := senderEnvString(env, "WecomBotKey")
	if wecomBotKey == "" {
		t.Skip("跳过：在 .env.json 中填写 WecomBotKey 后重跑")
	}

	notifyChannel := &models.NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://qyapi.weixin.qq.com/cgi-bin/webhook/send",
				Timeout: 10000,
				Request: models.RequestDetail{
					Parameters: map[string]string{
						"key": "{{ $params.key }}",
					},
					Body: `{"msgtype": "markdown", "markdown": {"content": "{{$tpl.content}}"}}`,
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				RetryTimes:    2,
				RetryInterval: 100,
			},
		},
		ParamConfig: &models.NotifyParamConfig{
			Custom: models.Params{
				Params: []models.ParamItem{
					{Key: "key"},
				},
			},
		},
	}

	client, err := models.GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}

	req := &NotifyRequest{
		Config: notifyChannel,
		Events: []*models.AlertCurEvent{
			{
				Hash: "wecom-image-test",
				ShotImageBase64: map[string]string{
					"shot_1": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
				},
			},
		},
		TplContent: map[string]interface{}{
			"title":   "wecom image test",
			"content": "wecom image test from provider test",
		},
		CustomParams: map[string]string{
			"key": wecomBotKey,
		},
		HttpClient: client,
	}

	result := (&WecomProvider{}).Notify(context.Background(), req)
	if result.Err != nil {
		t.Fatalf("Wecom Notify failed: %v, response: %s", result.Err, result.Response)
	}
	if !strings.Contains(result.Response, "image_send") {
		t.Fatalf("expect image send response, got: %s", result.Response)
	}
}

func TestSendTencentVoiceNotification(t *testing.T) {
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Skipf("跳过：读取 ../.env.json 失败: %v", err)
	}

	// 创建腾讯云语音通知配置
	notifyChannel := &models.NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Method:  "POST",
				URL:     data["TencentVoiceUrl"], // 使用测试服务器的URL
				Timeout: 5,
				Request: models.RequestDetail{
					Body: `{"TemplateId":"1475778","CalledNumber":"{{ $sendto }}","VoiceSdkAppid":"1400655317","TemplateParamSet":["测试"],"PlayTimes":2}`,
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Host":         "vms.tencentcloudapi.com",
					"X-TC-Action":  "SendTtsVoice",
					"X-TC-Version": "2020-09-02",
					"X-TC-Region":  "ap-beijing",
					"Service":      "vms",
					"Secret_ID":    "test-id",
					"Secret_Key":   "test-key",
				},
				RetryTimes:    2,
				RetryInterval: 1,
			},
		},
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{
				ContactKey: "phone",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"code": "123456",
	}

	// 创建HTTP客户端
	client, err := models.GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := SendHTTPRequest(notifyChannel.RequestConfig.HTTPRequestConfig, events, tpl, map[string]string{}, []string{"+8618021015257"}, client)
	if err != nil {
		t.Fatalf("SendHTTP失败: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "RequestId") || !strings.Contains(resp, "SendStatus") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendTencentSMSNotification(t *testing.T) {
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Skipf("跳过：读取 ../.env.json 失败: %v", err)
	}

	// 创建腾讯云短信通知配置
	notifyChannel := &models.NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Method:  "POST",
				URL:     data["TencentSMSUrl"], // 使用测试服务器的URL
				Timeout: 5,
				Request: models.RequestDetail{
					Body: `{"PhoneNumberSet":["{{ $sendto }}"],"SignName":"测试签名","SmsSdkAppId":"1400000000","TemplateId":"1000000","TemplateParamSet":["测试"]}`,
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
					"Host":         "sms.tencentcloudapi.com",
					"X-TC-Action":  "SendSms",
					"X-TC-Version": "2021-01-11",
					"X-TC-Region":  "ap-guangzhou",
					"Service":      "sms",
					"Secret_ID":    "test-id",
					"Secret_Key":   "test-key",
				},
				RetryTimes:    2,
				RetryInterval: 1,
			},
		},
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{
				ContactKey: "phone",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"code": "123456",
	}

	// 创建HTTP客户端
	client, err := models.GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := SendHTTPRequest(notifyChannel.RequestConfig.HTTPRequestConfig, events, tpl, map[string]string{}, []string{"+8618021015257"}, client)
	if err != nil {
		t.Fatalf("SendHTTP失败: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "RequestId") || !strings.Contains(resp, "SendStatusSet") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendAliYunVoiceNotification(t *testing.T) {
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Skipf("跳过：读取 ../.env.json 失败: %v", err)
	}

	// 创建阿里云语音通知配置
	notifyChannel := &models.NotifyChannelConfig{
		Ident:       "ali-voice",
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Method:  "POST",
				URL:     "http://dyvmsapi.aliyuncs.com",
				Timeout: 10,
				Request: models.RequestDetail{
					Parameters: map[string]string{
						"AccessKeyId":     data["AccessKeyId"],
						"AccessKeySecret": data["AccessKeySecret"],
						"TtsCode":         data["TtsCode"],
						"CalledNumber":    `{{ $sendto }}`,
						"TtsParam":        `{"alert_name":"test"}`,
					},
				},
				RetryTimes:    2,
				RetryInterval: 1,
			},
		},
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{
				ContactKey: "phone",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"code": data["TtsCode"],
	}

	// 创建HTTP客户端
	client, err := models.GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := SendHTTPRequest(notifyChannel.RequestConfig.HTTPRequestConfig, events, tpl, map[string]string{}, []string{data["Phone"]}, client)
	if err != nil {
		t.Fatalf("SendHTTP失败: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "RequestId") || !strings.Contains(resp, "CallId") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendAliYunSMSNotification(t *testing.T) {
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Skipf("跳过：读取 ../.env.json 失败: %v", err)
	}

	notifyChannel := &models.NotifyChannelConfig{
		Ident:       "ali-sms",
		RequestType: "http",
		RequestConfig: &models.RequestConfig{
			HTTPRequestConfig: &models.HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://dysmsapi.aliyuncs.com",
				Timeout: 10000,
				Request: models.RequestDetail{
					Parameters: map[string]string{
						"PhoneNumbers":    "18291906071",
						"SignName":        data["SignName"],
						"TemplateCode":    data["TemplateCode"],
						"TemplateParam":   `{"name":"text","tag":"text"}`,
						"AccessKeyId":     data["AccessKeyId"],
						"AccessKeySecret": data["AccessKeySecret"],
					},
				},
				Headers: map[string]string{
					"Content-Type": "application/json",
				},
				RetryTimes:    2,
				RetryInterval: 1,
			},
		},
		ParamConfig: &models.NotifyParamConfig{
			UserInfo: &models.UserInfo{
				ContactKey: "phone",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"code": data["TemplateCode"],
	}

	// 创建HTTP客户端
	client, err := models.GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	p := AliyunSmsProvider{}
	result := p.Notify(context.Background(), &NotifyRequest{
		Config:       notifyChannel,
		Events:       events,
		TplContent:   tpl,
		CustomParams: map[string]string{},
		Sendtos:      []string{data["Phone"]},
		HttpClient:   client,
	})
	if result.Err != nil {
		t.Fatalf("SendHTTP失败: %v", result.Err)
	}

	//resp, err := SendHTTPRequest(notifyChannel.RequestConfig.HTTPRequestConfig, events, tpl, map[string]string{}, []string{data["Phone"]}, client)
	//if err != nil {
	//	t.Fatalf("SendHTTP失败: %v", err)
	//}

	// 验证响应
	resp := result.Response
	if !strings.Contains(resp, "BizId") || !strings.Contains(resp, "RequestId") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendFlashDuty(t *testing.T) {
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Skipf("跳过：读取 ../.env.json 失败: %v", err)
	}

	// 创建NotifyChannelConfig对象
	notifyChannel := &models.NotifyChannelConfig{
		ID:          1,
		Name:        "FlashDuty测试",
		Ident:       "flashduty-test",
		RequestType: "flashduty",
		RequestConfig: &models.RequestConfig{
			FlashDutyRequestConfig: &models.FlashDutyRequestConfig{
				IntegrationUrl: data["FlashDutyIntegrationUrl"],
			},
		},
	}

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// 调用SendFlashDuty方法
	flashDutyChannelID := int64(4344322009498)
	resp, err := SendFlashDuty(notifyChannel.RequestConfig.FlashDutyRequestConfig, events, flashDutyChannelID, client)

	// 验证结果
	if err != nil {
		t.Errorf("SendFlashDuty返回错误: %v", err)
	}

	// 验证响应内容
	if !strings.Contains(resp, "success") {
		t.Errorf("响应内容不包含预期的'success'字符串, 得到: %s", resp)
	}

	// 测试无效的客户端情况
	_, err = SendFlashDuty(notifyChannel.RequestConfig.FlashDutyRequestConfig, events, flashDutyChannelID, nil)
	if err == nil || !strings.Contains(err.Error(), "http client not found") {
		t.Errorf("预期错误'http client not found'，但得到: %v", err)
	}

	// 测试请求失败的情况
	invalidNotifyChannel := &models.NotifyChannelConfig{
		RequestType: "flashduty",
		RequestConfig: &models.RequestConfig{
			FlashDutyRequestConfig: &models.FlashDutyRequestConfig{
				IntegrationUrl: "http://invalid-url-that-does-not-exist",
			},
		},
	}

	_, err = SendFlashDuty(invalidNotifyChannel.RequestConfig.FlashDutyRequestConfig, events, flashDutyChannelID, client)
	if err == nil {
		t.Errorf("预期请求失败，但未返回错误")
	}
}

// senderDotEnvPath 返回 alert/sender/.env.json 路径（在 provider 目录跑测时为 ../.env.json）。
func senderDotEnvPath() string {
	for _, p := range []string{
		"../.env.json",
		filepath.Join("alert", "sender", ".env.json"),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "../.env.json"
}

// readSenderDotEnv 读取 .env.json，支持值为字符串或数字（与 encoding/json 一致）。
func readSenderDotEnv(t *testing.T) map[string]interface{} {
	t.Helper()
	path := senderDotEnvPath()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("未读取到 %s: %v", path, err)
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("解析 %s: %v", path, err)
	}
	return m
}

func senderEnvString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case json.Number:
		return strings.TrimSpace(x.String())
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

// read key value from json file
func readKeyValueFromJsonFile(filePath string) (map[string]string, error) {
	jsonFile, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer jsonFile.Close()

	var data map[string]string
	err = json.NewDecoder(jsonFile).Decode(&data)
	return data, err
}
