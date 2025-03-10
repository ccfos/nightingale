package models

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// 创建测试事件
var events = []*AlertCurEvent{
	{
		Id:           1,
		Hash:         "test-hash",
		RuleName:     "测试规则",
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
		AnnotationsJSON: map[string]string{
			"summary":     "测试告警摘要",
			"description": "这是一个详细的告警描述",
		},
		Target: &Target{
			Ident: "test-target",
		},
		NotifyGroupsObj: []*UserGroup{
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
		t.Fatalf("读取JSON文件失败: %v", err)
	}
	// 创建钉钉通知配置
	notifyChannel := &NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://oapi.dingtalk.com/robot/send", // 使用测试服务器的URL
				Timeout: 10000,
				Request: RequestDetail{
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
		ParamConfig: &NotifyParamConfig{
			Custom: Params{
				Params: []ParamItem{
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
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, params, []string{data["Phone"]}, client)
	if err != nil {
		t.Fatalf("SendHTTP failed: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "errmsg") {
		t.Errorf("Response does not contain expected content, got: %s", resp)
	}
}

func TestSendTencentVoiceNotification(t *testing.T) {
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Fatalf("读取JSON文件失败: %v", err)
	}

	// 创建腾讯云语音通知配置
	notifyChannel := &NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     data["TencentVoiceUrl"], // 使用测试服务器的URL
				Timeout: 5,
				Request: RequestDetail{
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
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "phone",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"code": "123456",
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, map[string]string{}, []string{"+8618021015257"}, client)
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
		t.Fatalf("读取JSON文件失败: %v", err)
	}

	// 创建腾讯云短信通知配置
	notifyChannel := &NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     data["TencentSMSUrl"], // 使用测试服务器的URL
				Timeout: 5,
				Request: RequestDetail{
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
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "phone",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"code": "123456",
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, map[string]string{}, []string{"+8618021015257"}, client)
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
		t.Fatalf("读取JSON文件失败: %v", err)
	}

	// 创建阿里云语音通知配置
	notifyChannel := &NotifyChannelConfig{
		Ident:       "ali-voice",
		RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     "http://dyvmsapi.aliyuncs.com",
				Timeout: 10,
				Request: RequestDetail{
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
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "phone",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"code": data["TtsCode"],
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, map[string]string{}, []string{data["Phone"]}, client)
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
		t.Fatalf("读取JSON文件失败: %v", err)
	}

	notifyChannel := &NotifyChannelConfig{
		Ident:       "ali-sms",
		RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     "https://dysmsapi.aliyuncs.com",
				Timeout: 10000,
				Request: RequestDetail{
					Parameters: map[string]string{
						"PhoneNumbers":    "{{ $sendto }}",
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
		ParamConfig: &NotifyParamConfig{
			UserInfo: &UserInfo{
				ContactKey: "phone",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]interface{}{
		"code": data["TemplateCode"],
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, map[string]string{}, []string{data["Phone"]}, client)
	if err != nil {
		t.Fatalf("SendHTTP失败: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "BizId") || !strings.Contains(resp, "RequestId") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendFlashDuty(t *testing.T) {
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Fatalf("读取JSON文件失败: %v", err)
	}

	// 创建NotifyChannelConfig对象
	notifyChannel := &NotifyChannelConfig{
		ID:          1,
		Name:        "FlashDuty测试",
		Ident:       "flashduty-test",
		RequestType: "flashduty",
		RequestConfig: &RequestConfig{
			FlashDutyRequestConfig: &FlashDutyRequestConfig{
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
	resp, err := notifyChannel.SendFlashDuty(events, flashDutyChannelID, client)

	// 验证结果
	if err != nil {
		t.Errorf("SendFlashDuty返回错误: %v", err)
	}

	// 验证响应内容
	if !strings.Contains(resp, "success") {
		t.Errorf("响应内容不包含预期的'success'字符串, 得到: %s", resp)
	}

	// 测试无效的客户端情况
	_, err = notifyChannel.SendFlashDuty(events, flashDutyChannelID, nil)
	if err == nil || !strings.Contains(err.Error(), "http client not found") {
		t.Errorf("预期错误'http client not found'，但得到: %v", err)
	}

	// 测试请求失败的情况
	invalidNotifyChannel := &NotifyChannelConfig{
		RequestType: "flashduty",
		RequestConfig: &RequestConfig{
			FlashDutyRequestConfig: &FlashDutyRequestConfig{
				IntegrationUrl: "http://invalid-url-that-does-not-exist",
			},
		},
	}

	_, err = invalidNotifyChannel.SendFlashDuty(events, flashDutyChannelID, client)
	if err == nil {
		t.Errorf("预期请求失败，但未返回错误")
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
