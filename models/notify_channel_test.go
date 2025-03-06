package models

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSendDingTalkNotification(t *testing.T) {
	// 创建一个测试服务器来模拟钉钉API响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法和内容类型
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type: application/json, got %s", contentType)
		}

		// 检查URL中的access_token参数
		token := r.URL.Query().Get("access_token")
		if token != "test-token" {
			t.Errorf("Expected access_token=test-token, got %s", token)
		}

		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
		}

		// 检查请求体是否包含预期的内容
		if !strings.Contains(string(body), "测试告警消息") {
			t.Errorf("Request body does not contain expected content, got: %s", string(body))
		}

		// 返回成功响应
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	// 创建钉钉通知配置
	notifyChannel := &NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     server.URL, // 使用测试服务器的URL
				Timeout: 5,
				Request: RequestDetail{
					Body: `{"msgtype":"text","text":{"content":"{{ $tpl.content }}"},"at":{"isAtAll":false,"atMobiles":["{{ $params.ats }}"]}}`,
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
					{
						Key: "ats",
					},
				},
			},
		},
	}

	// 创建测试事件
	events := []*AlertCurEvent{
		{
			Hash:     "test-hash",
			RuleName: "测试规则",
			Severity: 3,
			TagsMap: map[string]string{
				"host": "test-host",
				"app":  "test-app",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]string{
		"content": "测试告警消息",
	}

	// 创建通知参数
	params := map[string]string{
		"access_token": "test-token",
		"ats":          "13800138000",
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("Failed to create HTTP client: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, params, &User{Phone: "+8618021015257"}, client)
	if err != nil {
		t.Fatalf("SendHTTP failed: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "errmsg") {
		t.Errorf("Response does not contain expected content, got: %s", resp)
	}
}

func TestSendTencentVoiceNotification(t *testing.T) {
	// 创建一个测试服务器来模拟腾讯云语音API响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法和内容类型
		if r.Method != "POST" {
			t.Errorf("预期POST请求，得到 %s", r.Method)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("预期 Content-Type: application/json，得到 %s", contentType)
		}

		// 验证请求头
		action := r.Header.Get("X-TC-Action")
		if action != "SendTtsVoice" {
			t.Errorf("预期 X-TC-Action: SendTtsVoice，得到 %s", action)
		}

		version := r.Header.Get("X-TC-Version")
		if version != "2020-09-02" {
			t.Errorf("预期 X-TC-Version: 2020-09-02，得到 %s", version)
		}

		region := r.Header.Get("X-TC-Region")
		if region == "" {
			t.Errorf("缺少 X-TC-Region 请求头")
		}

		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("读取请求体失败: %v", err)
		}

		// 解析JSON请求体
		var requestBody map[string]interface{}
		if err := json.Unmarshal(body, &requestBody); err != nil {
			t.Errorf("解析JSON请求体失败: %v", err)
		}

		// 根据腾讯云API文档验证必要参数
		requiredFields := []string{"TemplateId", "CalledNumber", "VoiceSdkAppid"}
		for _, field := range requiredFields {
			if _, exists := requestBody[field]; !exists {
				t.Errorf("请求体缺少必要字段 %s", field)
			}
		}

		// 验证手机号格式符合E.164标准
		calledNumber, _ := requestBody["CalledNumber"].(string)
		if calledNumber == "" || !strings.HasPrefix(calledNumber, "+") {
			fmt.Println(calledNumber)
			t.Errorf("CalledNumber 格式不符合E.164标准: %s", calledNumber)
		}

		// 验证可选参数
		if templateParamSet, exists := requestBody["TemplateParamSet"].([]interface{}); exists {
			// 确保模板参数是字符串数组
			for i, param := range templateParamSet {
				if _, ok := param.(string); !ok {
					t.Errorf("TemplateParamSet[%d] 不是字符串类型", i)
				}
			}
		}

		// 验证播放次数
		if playTimes, exists := requestBody["PlayTimes"].(float64); exists {
			if playTimes < 1 || playTimes > 3 {
				t.Errorf("PlayTimes 值 %v 超出范围(1-3)", playTimes)
			}
		}

		// 返回成功响应
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Response":{"RequestId":"test-request-id","SendStatus":{"CallId":"test-call-id","SessionContext":"","Code":"Ok","Message":"success"},"SessionContext":""}}`))
	}))
	defer server.Close()

	// 创建腾讯云语音通知配置
	notifyChannel := &NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     server.URL, // 使用测试服务器的URL
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

	// 创建测试事件
	events := []*AlertCurEvent{
		{
			Hash:     "test-hash",
			RuleName: "测试规则",
			Severity: 3,
			TagsMap: map[string]string{
				"host": "test-host",
				"app":  "test-app",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]string{
		"code": "123456",
	}

	// 创建用户信息
	userInfos := []*User{
		{
			Phone: "+8613788888888",
		},
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, map[string]string{}, userInfos[0], client)
	if err != nil {
		t.Fatalf("SendHTTP失败: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "RequestId") || !strings.Contains(resp, "SendStatus") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendTencentSMSNotification(t *testing.T) {
	// 创建一个测试服务器来模拟腾讯云短信API响应
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法和内容类型
		if r.Method != "POST" {
			t.Errorf("预期POST请求，得到 %s", r.Method)
		}

		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("预期 Content-Type: application/json，得到 %s", contentType)
		}

		// 验证请求头
		action := r.Header.Get("X-TC-Action")
		if action != "SendSms" {
			t.Errorf("预期 X-TC-Action: SendSms，得到 %s", action)
		}

		version := r.Header.Get("X-TC-Version")
		if version != "2021-01-11" {
			t.Errorf("预期 X-TC-Version: 2021-01-11，得到 %s", version)
		}

		region := r.Header.Get("X-TC-Region")
		if region != "ap-guangzhou" {
			t.Errorf("预期 X-TC-Region: ap-guangzhou，得到 %s", region)
		}

		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("读取请求体失败: %v", err)
		}

		// 检查请求体是否包含预期的内容和格式
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "PhoneNumberSet") || !strings.Contains(bodyStr, "SmsSdkAppId") {
			t.Errorf("请求体不包含预期内容，得到: %s", bodyStr)
		}

		// 新增：验证更多腾讯云短信API必要字段
		expectedFields := []string{
			"TemplateId",
			"SignName",
			"TemplateParamSet",
		}

		for _, field := range expectedFields {
			if !strings.Contains(bodyStr, field) {
				t.Errorf("请求体缺少必要字段 %s，得到: %s", field, bodyStr)
			}
		}

		// 新增：解析JSON并验证字段值
		var requestBody map[string]interface{}
		if err := json.Unmarshal(body, &requestBody); err != nil {
			t.Errorf("解析请求体JSON失败: %v", err)
		} else {
			// 验证PhoneNumberSet正确性
			phoneNumbers, ok := requestBody["PhoneNumberSet"].([]interface{})
			if !ok || len(phoneNumbers) == 0 {
				t.Errorf("PhoneNumberSet格式不正确或为空")
			} else {
				// 验证手机号格式是否符合E.164标准 (+国家码手机号)
				phoneStr, _ := phoneNumbers[0].(string)
				fmt.Println(phoneStr)
			}

			// 验证SmsSdkAppId不为空
			if appId, ok := requestBody["SmsSdkAppId"].(string); !ok || appId == "" {
				t.Errorf("SmsSdkAppId不存在或为空")
			}

			// 验证TemplateId不为空
			if templateId, ok := requestBody["TemplateId"].(string); !ok || templateId == "" {
				t.Errorf("TemplateId不存在或为空")
			}
		}

		// 返回成功响应
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"Response":{"RequestId":"test-request-id","SendStatusSet":[{"SerialNo":"2023011100001111","PhoneNumber":"+8618021015257","Fee":1,"SessionContext":"","Code":"Ok","Message":"send success","IsoCode":"CN"}]}}`))
	}))
	defer server.Close()

	// 创建腾讯云短信通知配置
	notifyChannel := &NotifyChannelConfig{
		RequestType: "http",
		RequestConfig: &RequestConfig{
			HTTPRequestConfig: &HTTPRequestConfig{
				Method:  "POST",
				URL:     server.URL, // 使用测试服务器的URL
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

	// 创建测试事件
	events := []*AlertCurEvent{
		{
			Hash:     "test-hash",
			RuleName: "测试规则",
			Severity: 3,
			TagsMap: map[string]string{
				"host": "test-host",
				"app":  "test-app",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]string{
		"code": "123456",
	}

	// 创建用户信息
	userInfos := []*User{
		{
			Phone: "+8618021015257",
		},
		{
			Phone: "+8618021015258",
		},
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, map[string]string{}, userInfos[0], client)
	if err != nil {
		t.Fatalf("SendHTTP失败: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "RequestId") || !strings.Contains(resp, "SendStatusSet") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendAliYunVoiceNotification(t *testing.T) {
	data, err := readKeyValueFromJsonFile("/tmp/aliyun.json")
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

	// 创建测试事件
	events := []*AlertCurEvent{
		{
			Hash:     "test-hash",
			RuleName: "测试规则",
			Severity: 3,
			TagsMap: map[string]string{
				"host": "test-host",
				"app":  "test-app",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]string{
		"code": "123456",
	}

	// 创建用户信息
	user := &User{
		Phone: data["Phone"],
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, map[string]string{}, user, client)
	if err != nil {
		t.Fatalf("SendHTTP失败: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "RequestId") || !strings.Contains(resp, "CallId") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendAliYunSMSNotification(t *testing.T) {
	data, err := readKeyValueFromJsonFile("/tmp/aliyun.json")
	if err != nil {
		t.Fatalf("读取JSON文件失败: %v", err)
	}

	fmt.Println(data)

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

	// 创建测试事件
	events := []*AlertCurEvent{
		{
			Hash:     "test-hash",
			RuleName: "测试规则",
			Severity: 3,
			TagsMap: map[string]string{
				"host": "test-host",
				"app":  "test-app",
			},
		},
	}

	// 创建通知模板
	tpl := map[string]string{
		"code": "123456",
	}

	// 创建用户信息
	user := &User{
		Phone: data["Phone"],
	}

	// 创建HTTP客户端
	client, err := GetHTTPClient(notifyChannel)
	if err != nil {
		t.Fatalf("创建HTTP客户端失败: %v", err)
	}

	// 调用SendHTTP方法
	resp, err := notifyChannel.SendHTTP(events, tpl, map[string]string{}, user, client)
	if err != nil {
		t.Fatalf("SendHTTP失败: %v", err)
	}

	// 验证响应
	if !strings.Contains(resp, "BizId") || !strings.Contains(resp, "RequestId") {
		t.Errorf("响应不包含预期内容，得到: %s", resp)
	}
}

func TestSendFlashDuty(t *testing.T) {
	// 创建一个模拟HTTP服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != "POST" {
			t.Errorf("预期POST请求，得到 %s", r.Method)
		}

		// 验证请求URL参数
		channelID := r.URL.Query().Get("channel_id")
		if channelID != "4344322009498" {
			t.Errorf("预期channel_id=4344322009498，得到 %s", channelID)
		}

		// 读取并验证请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("读取请求体失败: %v", err)
		}

		// 尝试解析事件数据
		var events []*AlertCurEvent
		err = json.Unmarshal(body, &events)
		if err != nil {
			t.Errorf("解析事件数据失败: %v", err)
		}

		if len(events) == 0 {
			t.Errorf("请求体不包含事件数据")
		} else if events[0].RuleName != "测试告警规则" {
			t.Errorf("事件规则名称不匹配，预期'测试告警规则'，得到'%s'", events[0].RuleName)
		}

		// 返回成功响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code": 200, "message": "success", "data": {"id": "123456"}}`))
	}))
	defer server.Close()

	// 创建NotifyChannelConfig对象
	notifyChannel := &NotifyChannelConfig{
		ID:          1,
		Name:        "FlashDuty测试",
		Ident:       "flashduty-test",
		RequestType: "flashduty",
		RequestConfig: &RequestConfig{
			FlashDutyRequestConfig: &FlashDutyRequestConfig{
				IntegrationUrl: server.URL,
			},
		},
	}

	// 创建测试事件
	events := []*AlertCurEvent{
		{
			Hash:         "test-hash-123",
			RuleId:       123,
			RuleName:     "测试告警规则",
			Severity:     2,
			GroupId:      1,
			GroupName:    "测试团队",
			TriggerTime:  time.Now().Unix(),
			TriggerValue: "90.5",
			LastEvalTime: time.Now().Unix(),
			Status:       1,
			TagsMap: map[string]string{
				"host":    "test-host",
				"service": "test-service",
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
