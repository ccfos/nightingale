package models

import (
	"net/http"
	"testing"
)

func TestNotifyChannelConfig_SendHTTP(t *testing.T) {
	type args struct {
		content map[string]string
		client  *http.Client
	}
	tests := []struct {
		name           string
		events         []*AlertCurEvent
		notifyParam    map[string]string
		notifyChannel  *NotifyChannelConfig
		notifyTemplate map[string]string
		userInfos      []*User
		flashDutyIDs   []int64
		wantErr        bool
	}{
		{
			name:        "test",
			notifyParam: map[string]string{},
			notifyChannel: &NotifyChannelConfig{
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "http://localhost:8080",
					Timeout: 10,
					Request: RequestDetail{
						Body: `{"msgType":"text","test":{"content":"{{ .tpl.test }}"}}`,
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
			},
			flashDutyIDs: []int64{0},
			notifyTemplate: map[string]string{
				"test": "here is a test msg",
			},
		},
		{
			name: "dingTalk",
			notifyParam: map[string]string{
				"access_token": "-",
			},

			notifyChannel: &NotifyChannelConfig{
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://oapi.dingtalk.com/robot/send",
					Timeout: 10,
					Request: RequestDetail{
						Body: `{"text":{"content":"{{ .tpl.test }}"}, "msgtype":"text", "at":{"isAtAll":"false", "atMobiles":{{ .user_info.phone }}} }`,
						Parameters: map[string]string{
							"access_token": "{{ .access_token }}",
						},
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "custom",
					Custom: CustomParam{
						Params: []ParamItem{
							{
								Key: "access_token",
							},
						},
					},
				},
			},
			flashDutyIDs: []int64{0},
			userInfos: []*User{
				{
					Phone: "18021015257",
				},
			},
			notifyTemplate: map[string]string{
				"test": "here is a test message",
			},
		},
		{
			name: "feishu",
			notifyParam: map[string]string{
				"hook": "-",
			},

			notifyChannel: &NotifyChannelConfig{
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://open.feishu.cn/open-apis/bot/v2/hook/{{ .hook }}",
					Timeout: 3,
					Request: RequestDetail{
						Body: `{"msg_type":"text","content":{"text":"{{ .tpl.test }}"}}`,
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
					RetryTimes: 0,
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "custom",
					Custom: CustomParam{
						Params: []ParamItem{
							{
								Key: "hook",
							},
						},
					},
				},
			},
			flashDutyIDs: []int64{0},
			notifyTemplate: map[string]string{
				"test": "here is a test msg",
			},
		},
		{
			name: "feishucard",
			notifyParam: map[string]string{
				"name": "xub",
				"msg":  "here is a test msg",
				"hook": "-",
			},
			flashDutyIDs: []int64{0},
			notifyChannel: &NotifyChannelConfig{
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://open.feishu.cn/open-apis/bot/v2/hook/{{ .hook }}",
					Timeout: 10,
					Request: RequestDetail{
						Body: `{"msg_type":"interactive","card":{"type":"template","data":{"template_id":"AAqFiKNkewv7V","template_version_name":"1.0.2", "template_variable": {"name": "{{ .name }}", "msg": "{{ .msg }}"}}}}`,
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "custom",
					Custom: CustomParam{
						Params: []ParamItem{
							{
								Key: "name",
							},
							{
								Key: "msg",
							},
							{
								Key: "hook",
							},
						},
					},
				},
			},
		},
		{
			name: "wecom",
			notifyParam: map[string]string{
				"key": "-",
			},
			notifyChannel: &NotifyChannelConfig{
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://qyapi.weixin.qq.com/cgi-bin/webhook/send",
					Timeout: 10,
					Request: RequestDetail{
						Body: `{"msgtype":"text","text":{"content":"{{ .tpl.test }}"}}`,
						Parameters: map[string]string{
							"key": "{{ .key }}",
						},
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "custom",
					Custom: CustomParam{
						Params: []ParamItem{
							{
								Key: "key",
							},
						},
					},
				},
			},
			flashDutyIDs: []int64{0},
			notifyTemplate: map[string]string{
				"test": "here is a test msg",
			},
		},
		{
			name:         "ali-sms",
			flashDutyIDs: []int64{0},
			notifyChannel: &NotifyChannelConfig{
				Ident: "ali-sms",
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "http://dysmsapi.aliyuncs.com",
					Timeout: 10,
					Request: RequestDetail{
						Parameters: map[string]string{
							"AccessKeyId":     "-",
							"AccessKeySecret": "-",

							"Action":           "SendSms",
							"Format":           "JSON",
							"OutId":            "123",
							"PhoneNumbers":     `{{ join .user_info.phone "," }}`,
							"RegionId":         "cn-hangzhou",
							"SignName":         "n9e",
							"SignatureMethod":  "HMAC-SHA1",
							"SignatureVersion": "1.0",
							"TemplateCode":     "SMS_478575599",
							"TemplateParam":    `{"code":"{{ .tpl.code }}"}`,
							"Version":          "2017-05-25",
						},
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "user_info",
					UserInfo: UserInfoParam{
						ContactKey: "phone",
					},
				},
			},
			userInfos: []*User{
				{
					Phone: "18021015257",
				},
			},
			notifyTemplate: map[string]string{
				"code": "123456",
			},
		},
		{
			name:         "ali-voice",
			flashDutyIDs: []int64{0},
			notifyChannel: &NotifyChannelConfig{
				Ident: "ali-voice",
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "http://dyvmsapi.aliyuncs.com",
					Timeout: 10,
					Request: RequestDetail{
						Parameters: map[string]string{
							"Action":   "SingleCallByTts",
							"Version":  "2017-05-25",
							"Format":   "JSON",
							"OutId":    "123",
							"RegionId": "cn-hangzhou",

							"SignatureMethod":  "HMAC-SHA1",
							"SignatureVersion": "1.0",

							"AccessKeyId":     "-",
							"AccessKeySecret": "-",
							"TtsCode":         "TTS_282205058",
							"CalledNumber":    `{{ index .user_info.phone 0 }}`,
							"TtsParam":        `{"alert_name":"test"}`,
						},
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "user_info",
					UserInfo: UserInfoParam{
						ContactKey: "phone",
					},
				},
			},
			userInfos: []*User{
				{
					Phone: "18021015257",
				},
			},
			notifyTemplate: map[string]string{
				"code": "123456",
			},
		},
		{
			name:         "tx-sms",
			notifyParam:  map[string]string{},
			flashDutyIDs: []int64{0},
			notifyChannel: &NotifyChannelConfig{
				Ident: "tx-sms",
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://sms.tencentcloudapi.com",
					Timeout: 10,
					Request: RequestDetail{
						Body: `{"PhoneNumberSet":[{{range $index, $element := .user_info.phone}}{{if $index}},{{end}}"{{$element}}"{{end}}],"SignName":"快猫星云","SmsSdkAppId":"1400682772","TemplateId":"1584300","TemplateParamSet":["测试"]}`,
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
						"Host":         "sms.tencentcloudapi.com",
						"X-TC-Action":  "SendSms",
						"X-TC-Version": "2021-01-11",
						"X-TC-Region":  "ap-guangzhou",
						"Service":      "sms",
						"Secret_ID":    "-",
						"Secret_Key":   "-",
					},
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "user_info",
					UserInfo: UserInfoParam{
						ContactKey: "phone",
					},
				},
			},
			userInfos: []*User{
				{
					Phone: "+8618021015257",
				},
			},
			notifyTemplate: map[string]string{
				"code": "123456",
			},
		},
		{
			name:         "tx-voice",
			notifyParam:  map[string]string{},
			flashDutyIDs: []int64{0},
			notifyChannel: &NotifyChannelConfig{
				Ident: "tx-voice",
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://vms.tencentcloudapi.com",
					Timeout: 10,
					Request: RequestDetail{
						Body: `{"CalledNumber":"{{ index .user_info.phone 0 }}","VoiceSdkAppid":"1400655317","TemplateId":"1475778","TemplateParamSet":["测试"]}`,
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
						"Host":         "vms.tencentcloudapi.com",
						"X-TC-Action":  "SendTtsVoice",
						"X-TC-Version": "2020-09-02",
						"X-TC-Region":  "ap-beijing",
						"Service":      "vms",
						"Secret_ID":    "-",
						"Secret_Key":   "-",
					},
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "user_info",
					UserInfo: UserInfoParam{
						ContactKey: "phone",
					},
				},
			},
			userInfos: []*User{
				{
					Phone: "+8618021015257",
				},
			},
			notifyTemplate: map[string]string{
				"code": "123456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := GetHTTPClient(tt.notifyChannel)
			if err := tt.notifyChannel.SendHTTP(tt.events, tt.notifyTemplate, tt.notifyParam, tt.userInfos, client); (err != nil) != tt.wantErr {
				t.Errorf("SendHTTP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNotifyChannelConfig_SendFlashDuty(t *testing.T) {
	type args struct {
		content map[string]string
		client  *http.Client
	}
	tests := []struct {
		name           string
		events         []*AlertCurEvent
		notifyParam    map[string]string
		notifyChannel  *NotifyChannelConfig
		notifyTemplate map[string]string
		userInfos      []*User
		flashDutyIDs   []int64
		wantErr        bool
	}{
		{
			name: "flash duty",
			notifyChannel: &NotifyChannelConfig{
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "",
					Timeout: 10,
					Request: RequestDetail{
						Parameters: map[string]string{
							"integration_key": "{{ .integration_key }}",
							"channel_id":      "{{ .flash_duty_channel_id }}",
						},
						Body: `{"event_status": "Warning","alert_key": "1","description": "{{ .tpl.description }}","title_rule": "test","event_time": 1706614721,"labels": {"name":"guguji5","env":"prod"}}`,
					},
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "flashduty",
					FlashDuty: FlashDutyParam{
						IntegrationUrl: "https://api.flashcat.cloud/event/push/alert/standard",
					},
				},
			},
			flashDutyIDs: []int64{4344322009498},
			notifyTemplate: map[string]string{
				"description": "here is a test msg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := GetHTTPClient(tt.notifyChannel)
			if err := tt.notifyChannel.SendFlashDuty(tt.events, tt.notifyTemplate, tt.flashDutyIDs[0], client); (err != nil) != tt.wantErr {
				t.Errorf("SendHTTP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

//func TestNotifyChannelConfig_SendEmail(t *testing.T) {
//	type args struct {
//		content map[string]string
//		client  *smtp.Client
//	}
//	tests := []struct {
//		name           string
//		notifyParam    []map[string]string
//		notifyChannel  *NotifyChannelConfig
//		notifyTemplate map[string]string
//		wantErr        bool
//	}{
//		{
//			name: "test",
//			notifyParam: []map[string]string{
//				{
//					"email": "erickbin@163.com",
//				},
//			},
//			notifyChannel: &NotifyChannelConfig{
//				SMTPRequestConfig: SMTPRequestConfig{
//					Host:               "smtp.163.com",
//					Port:               25,
//					Username:           "erickbin",
//					Password:           "WRURJ33L3hMTkMQt",
//					From:               "erickbin@163.com",
//					Message:            `{{ .tpl.test }}`,
//					InsecureSkipVerify: true,
//					Batch:              5,
//				},
//				ParamConfig: NotifyParamConfig{
//					ParamType: "user_info",
//					UserInfo: UserInfoParam{
//						ContactKey: "email",
//					},
//					BatchSend: true,
//				},
//			},
//			notifyTemplate: map[string]string{
//				"test": "here is a test msg",
//			},
//		},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			client, err := GetSMTPClient(tt.notifyChannel)
//			if err != nil {
//				t.Errorf("GetSMTPClient() error = %v", err)
//			}
//			if err := tt.notifyChannel.SendEmail(tt.notifyTemplate, tt.notifyParam, client); (err != nil) != tt.wantErr {
//				t.Errorf("SendHTTP() error = %v, wantErr %v", err, tt.wantErr)
//			}
//		})
//	}
//}

func TestNotifyChannelConfig_SendScript(t *testing.T) {
	type args struct {
		content map[string]string
		script  string
		path    string
	}
	tests := []struct {
		name           string
		notifyParam    []map[string]string
		notifyChannel  *NotifyChannelConfig
		notifyTemplate map[string]string
		wantErr        bool
		userInfos      []*User
	}{
		{
			name: "script",
			notifyParam: []map[string]string{
				{},
			},
			notifyChannel: &NotifyChannelConfig{
				ScriptRequestConfig: &ScriptRequestConfig{
					Timeout: 10,
					Script:  "#!/bin/bash \necho test",
					Path:    "",
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "custom",
					Custom: CustomParam{
						[]ParamItem{},
					},
				},
			},
			notifyTemplate: map[string]string{
				"test": "here is a test msg",
			},
		},
		{
			name: "timeout",
			notifyParam: []map[string]string{
				{},
			},
			notifyChannel: &NotifyChannelConfig{
				ScriptRequestConfig: &ScriptRequestConfig{
					Timeout: 10,
					Script:  "#!/bin/bash \nsleep 20",
					Path:    "",
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "custom",
					Custom: CustomParam{
						[]ParamItem{},
					},
				},
			},
			notifyTemplate: map[string]string{
				"test": "here is a test msg",
			},
		},

		{
			name: "path",
			notifyParam: []map[string]string{
				{},
			},
			notifyChannel: &NotifyChannelConfig{
				ScriptRequestConfig: &ScriptRequestConfig{
					Timeout: 10,
					Script:  "",
					Path:    "/Users/red/Desktop/myGo/work/ccfos/nightingale/models/.notify_scriptt",
				},
				ParamConfig: &NotifyParamConfig{
					ParamType: "custom",
					Custom: CustomParam{
						[]ParamItem{},
					},
				},
			},
			notifyTemplate: map[string]string{
				"test": "here is a test msg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.notifyChannel.SendScript([]*AlertCurEvent{}, tt.notifyTemplate, tt.notifyParam[0], tt.userInfos); (err != nil) != tt.wantErr {
				t.Errorf("SendHTTP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
