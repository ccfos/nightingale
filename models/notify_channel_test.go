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
					UserInfo: UserInfoParam{
						ContactKey: "phone",
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
			name: "flash duty",
			notifyParam: map[string]string{
				"integration_key": "-",
				"title_rule":      "test",
			},
			notifyChannel: &NotifyChannelConfig{
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "https://api.flashcat.cloud/event/push/alert/standard",
					Timeout: 10,
					Request: RequestDetail{
						Parameters: map[string]string{
							"integration_key": "{{ .integration_key }}",
							"channel_id":      "{{ .flash_duty_channel_id }}",
						},
						Body: `{"event_status": "Warning","alert_key": "1","description": "{{ .tpl.description }}","title_rule": "{{ .title_rule }}","event_time": 1706614721,"labels": {"name":"guguji5","env":"prod"}}`,
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
								Key: "title_rule",
							},
						},
					},
				},
			},
			flashDutyIDs: []int64{4344322009498},
			notifyTemplate: map[string]string{
				"description": "here is a test msg",
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
			name: "ali-sms",
			notifyParam: map[string]string{
				"access_key_id":     "-",
				"access_key_secret": "-",
			},
			flashDutyIDs: []int64{0},
			notifyChannel: &NotifyChannelConfig{
				Ident: "ali-sms",
				HTTPRequestConfig: &HTTPRequestConfig{
					Method:  "POST",
					URL:     "http://dysmsapi.aliyuncs.com",
					Timeout: 10,
					Request: RequestDetail{
						Parameters: map[string]string{
							"access_key_id":     "{{ .access_key_id }}",
							"access_key_secret": "{{ .access_key_secret }}",
							"sign_name":         "n9e",
							"template_code":     "SMS_478575599",
							"phone_numbers":     `{{ join .user_info.phone "," }}`,
							"template_param":    `{"code":"{{ .tpl.code }}"}`,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := GetHTTPClient(tt.notifyChannel)
			if err := tt.notifyChannel.SendHTTP(tt.events, tt.notifyTemplate, tt.notifyParam, tt.userInfos, tt.flashDutyIDs[0], client); (err != nil) != tt.wantErr {
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
			if err := tt.notifyChannel.SendScript([]*AlertCurEvent{}, tt.notifyTemplate, tt.notifyParam[0]); (err != nil) != tt.wantErr {
				t.Errorf("SendHTTP() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
