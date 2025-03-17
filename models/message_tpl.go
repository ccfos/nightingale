package models

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

// MessageTemplate 消息模板结构
type MessageTemplate struct {
	ID                 int64             `json:"id" gorm:"primarykey"`
	Name               string            `json:"name"`                           // 模板名称
	Ident              string            `json:"ident"`                          // 模板标识
	Content            map[string]string `json:"content" gorm:"serializer:json"` // 模板内容
	UserGroupIds       []int64           `json:"user_group_ids" gorm:"serializer:json"`
	NotifyChannelIdent string            `json:"notify_channel_ident"` // 通知媒介 Ident
	Private            int               `json:"private"`              // 0-公开 1-私有
	Weight             int               `json:"weight"`               // 权重，根据此字段对内置模板进行排序
	CreateAt           int64             `json:"create_at"`
	CreateBy           string            `json:"create_by"`
	UpdateAt           int64             `json:"update_at"`
	UpdateBy           string            `json:"update_by"`
}

func MessageTemplateStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=message_template")
		return s, err
	}

	session := DB(ctx).Model(&MessageTemplate{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func MessageTemplateGetsAll(ctx *ctx.Context) ([]*MessageTemplate, error) {
	if !ctx.IsCenter {
		templates, err := poster.GetByUrls[[]*MessageTemplate](ctx, "/v1/n9e/message-templates")
		return templates, err
	}

	var templates []*MessageTemplate
	err := DB(ctx).Find(&templates).Error
	if err != nil {
		return nil, err
	}

	return templates, nil
}

func MessageTemplateGets(ctx *ctx.Context, id int64, name, ident string) ([]*MessageTemplate, error) {
	session := DB(ctx)

	if id != 0 {
		session = session.Where("id = ?", id)
	}

	if name != "" {
		session = session.Where("name = ?", name)
	}

	if ident != "" {
		session = session.Where("ident = ?", ident)
	}

	var templates []*MessageTemplate
	err := session.Find(&templates).Error

	return templates, err
}

func (t *MessageTemplate) TableName() string {
	return "message_template"
}

func (t *MessageTemplate) Verify() error {
	if t.Name == "" {
		return errors.New("template name cannot be empty")
	}

	if t.Ident == "" {
		return errors.New("template identifier cannot be empty")
	}

	if !regexp.MustCompile("^[a-zA-Z0-9_-]+$").MatchString(t.Ident) {
		return fmt.Errorf("template identifier must be ^[a-zA-Z0-9_-]+$, current: %s", t.Ident)
	}

	for key := range t.Content {
		if key == "" {
			return errors.New("template content cannot have empty keys")
		}
	}

	if t.Private == 1 && len(t.UserGroupIds) == 0 {
		return errors.New("user group IDs of private msg tpl cannot be empty")
	}

	if t.Private != 0 && t.Private != 1 {
		return errors.New("private flag must be 0 or 1")
	}

	return nil
}

func (t *MessageTemplate) Update(ctx *ctx.Context, ref MessageTemplate) error {
	// ref.FE2DB()
	if t.Ident != ref.Ident {
		return errors.New("cannot update ident")
	}

	ref.ID = t.ID
	ref.CreateAt = t.CreateAt
	ref.CreateBy = t.CreateBy
	ref.UpdateAt = time.Now().Unix()

	err := ref.Verify()
	if err != nil {
		return err
	}
	return DB(ctx).Model(t).Select("*").Updates(ref).Error
}

func (t *MessageTemplate) DB2FE() {
	if t.UserGroupIds == nil {
		t.UserGroupIds = make([]int64, 0)
	}
}

func MessageTemplateGet(ctx *ctx.Context, where string, args ...interface{}) (*MessageTemplate, error) {
	lst, err := MessageTemplatesGet(ctx, where, args...)
	if err != nil || len(lst) == 0 {
		return nil, err
	}
	return lst[0], err
}

func MessageTemplatesGet(ctx *ctx.Context, where string, args ...interface{}) ([]*MessageTemplate, error) {
	lst := make([]*MessageTemplate, 0)
	session := DB(ctx)
	if where != "" && len(args) > 0 {
		session = session.Where(where, args...)
	}

	err := session.Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for _, t := range lst {
		t.DB2FE()
	}
	return lst, nil
}

func MessageTemplatesGetBy(ctx *ctx.Context, notifyChannelIdents []string) ([]*MessageTemplate, error) {
	lst := make([]*MessageTemplate, 0)
	session := DB(ctx)
	if len(notifyChannelIdents) > 0 {
		session = session.Where("notify_channel_ident IN (?)", notifyChannelIdents)
	}

	err := session.Order("weight asc").Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for _, t := range lst {
		t.DB2FE()
	}
	return lst, nil
}

type MsgTplList []*MessageTemplate

func (t MsgTplList) GetIdentSet() map[int64]struct{} {
	idents := make(map[int64]struct{}, len(t))
	for _, tpl := range t {
		idents[tpl.ID] = struct{}{}
	}
	return idents
}

func (t MsgTplList) IfUsed(nr *NotifyRule) bool {
	identSet := t.GetIdentSet()
	for _, nc := range nr.NotifyConfigs {
		if _, ok := identSet[nc.TemplateID]; ok {
			return true
		}
	}
	return false
}

const (
	DingtalkTitle   = `{{if $event.IsRecovered}} Recovered {{else}}Triggered{{end}}: {{$event.RuleName}}`
	FeishuCardTitle = `🔔 {{$event.RuleName}}`
	FeishuAppTitle  = `{{- if $event.IsRecovered }}🔔 ﹝恢复﹞ {{$event.RuleName}}{{- else }}🔔 ﹝告警﹞ {{$event.RuleName}}{{- end -}}`
	LarkCardTitle   = `🔔 {{$event.RuleName}}`
)

var NewTplMap = map[string]string{
	"ali-voice": `{{$event.RuleName}}`,
	"ali-sms":   `{{$event.RuleName}}`,
	"tx-voice":  `S{{$event.Severity}}{{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}{{$event.RuleName}}`,
	"tx-sms":    `级别状态: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}规则名称: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **告警级别**: {{$event.Severity}}级
{{- if $event.RuleNote}}
	- **规则备注**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **当次触发时值**: {{$event.TriggerValue}}
- **当次触发时间**: {{timeformat $event.TriggerTime}}
- **告警持续时长**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **恢复时值**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **恢复时间**: {{timeformat $event.LastEvalTime}}
- **告警持续时长**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **告警事件标签**:
{{- range $key, $val := $event.TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}

{{$domain := "http://127.0.0.1:17000" }}
{{$mutelink := print $domain "/alert-mutes/add?busiGroup=" $event.GroupId "&cate=" $event.Cate "&datasource_ids=" $event.DatasourceId "&prod=" $event.RuleProd}}
{{- range $key, $value := $event.TagsMap}}
{{- $encodedValue := $value | urlquery }}
{{- $mutelink = print $mutelink "&tags=" $key "%3D" $encodedValue}}
{{- end}}
[事件详情]({{$domain}}/alert-his-events/{{$event.Id}}) | [屏蔽1小时]({{$mutelink}}) | [查看曲线]({{$domain}}/metric/explorer?data_source_id={{$event.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{$event.PromQl|urlquery}})`,
	Email: `<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>夜莺告警通知</title>
		<style type="text/css">
			.wrapper {
				background-color: #f8f8f8;
				padding: 15px;
				height: 100%;
			}
			.main {
				width: 600px;
				padding: 30px;
				margin: 0 auto;
				background-color: #fff;
				font-size: 12px;
				font-family: verdana,'Microsoft YaHei',Consolas,'Deja Vu Sans Mono','Bitstream Vera Sans Mono';
			}
			header {
				border-radius: 2px 2px 0 0;
			}
			header .title {
				font-size: 14px;
				color: #333333;
				margin: 0;
			}
			header .sub-desc {
				color: #333;
				font-size: 14px;
				margin-top: 6px;
				margin-bottom: 0;
			}
			hr {
				margin: 20px 0;
				height: 0;
				border: none;
				border-top: 1px solid #e5e5e5;
			}
			em {
				font-weight: 600;
			}
			table {
				margin: 20px 0;
				width: 100%;
			}
	
			table tbody tr{
				font-weight: 200;
				font-size: 12px;
				color: #666;
				height: 32px;
			}
	
			.succ {
				background-color: green;
				color: #fff;
			}
	
			.fail {
				background-color: red;
				color: #fff;
			}
	
			.succ th, .succ td, .fail th, .fail td {
				color: #fff;
			}
	
			table tbody tr th {
				width: 80px;
				text-align: right;
			}
			.text-right {
				text-align: right;
			}
			.body {
				margin-top: 24px;
			}
			.body-text {
				color: #666666;
				-webkit-font-smoothing: antialiased;
			}
			.body-extra {
				-webkit-font-smoothing: antialiased;
			}
			.body-extra.text-right a {
				text-decoration: none;
				color: #333;
			}
			.body-extra.text-right a:hover {
				color: #666;
			}
			.button {
				width: 200px;
				height: 50px;
				margin-top: 20px;
				text-align: center;
				border-radius: 2px;
				background: #2D77EE;
				line-height: 50px;
				font-size: 20px;
				color: #FFFFFF;
				cursor: pointer;
			}
			.button:hover {
				background: rgb(25, 115, 255);
				border-color: rgb(25, 115, 255);
				color: #fff;
			}
			footer {
				margin-top: 10px;
				text-align: right;
			}
			.footer-logo {
				text-align: right;
			}
			.footer-logo-image {
				width: 108px;
				height: 27px;
				margin-right: 10px;
			}
			.copyright {
				margin-top: 10px;
				font-size: 12px;
				text-align: right;
				color: #999;
				-webkit-font-smoothing: antialiased;
			}
		</style>
	</head>
	<body>
	<div class="wrapper">
		<div class="main">
			<header>
				<h3 class="title">{{$event.RuleName}}</h3>
				<p class="sub-desc"></p>
			</header>
	
			<hr>
	
			<div class="body">
				<table cellspacing="0" cellpadding="0" border="0">
					<tbody>
					{{if $event.IsRecovered}}
					<tr class="succ">
						<th>级别状态：</th>
						<td>S{{$event.Severity}} Recovered</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>级别状态：</th>
						<td>S{{$event.Severity}} Triggered</td>
					</tr>
					{{end}}
	
					<tr>
						<th>策略备注：</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>设备备注：</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>触发时值：</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>监控对象：</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>监控指标：</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>恢复时间：</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>触发时间：</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>发送时间：</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						报警太多？使用 <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a> 做告警聚合降噪、排班OnCall！
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `级别状态: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}   
规则名称: {{$event.RuleName}}{{if $event.RuleNote}}   
规则备注: {{$event.RuleNote}}{{end}}   
监控指标: {{$event.TagsJSON}}
{{if $event.IsRecovered}}恢复时间：{{timeformat $event.LastEvalTime}}{{else}}触发时间: {{timeformat $event.TriggerTime}}
触发时值: {{$event.TriggerValue}}{{end}}
发送时间: {{timestamp}}{{$domain := "http://127.0.0.1:17000" }}   
事件详情: {{$domain}}/alert-his-events/{{$event.Id}}{{$muteUrl := print $domain "/alert-mutes/add?busiGroup=" $event.GroupId "&cate=" $event.Cate "&datasource_ids=" $event.DatasourceId "&prod=" $event.RuleProd}}{{range $key, $value := $event.TagsMap}}{{$muteUrl = print $muteUrl "&tags=" $key "%3D" $value}}{{end}}   
屏蔽1小时: {{ unescaped $muteUrl }}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**告警集群:** {{$event.Cluster}}{{end}}   
**级别状态:** S{{$event.Severity}} Recovered   
**告警名称:** {{$event.RuleName}}  
**事件标签:** {{$event.TagsJSON}}   
**恢复时间:** {{timeformat $event.LastEvalTime}}   
**告警描述:** **服务已恢复**   
{{- else }}
{{- if ne $event.Cate "host"}}   
**告警集群:** {{$event.Cluster}}{{end}}   
**级别状态:** S{{$event.Severity}} Triggered   
**告警名称:** {{$event.RuleName}}  
**事件标签:** {{$event.TagsJSON}}   
**触发时间:** {{timeformat $event.TriggerTime}}   
**发送时间:** {{timestamp}}   
**触发时值:** {{$event.TriggerValue}}   
{{if $event.RuleNote }}**告警描述:** **{{$event.RuleNote}}**{{end}}   
{{- end -}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
[事件详情]({{$domain}}/alert-his-events/{{$event.Id}})|[屏蔽1小时]({{$domain}}/alert-mutes/add?busiGroup={{$event.GroupId}}&cate={{$event.Cate}}&datasource_ids={{$event.DatasourceId}}&prod={{$event.RuleProd}}{{range $key, $value := $event.TagsMap}}&tags={{$key}}%3D{{$value}}{{end}})|[查看曲线]({{$domain}}/metric/explorer?data_source_id={{$event.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{$event.PromQl|escape}})`,
	EmailSubject: `{{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
	Mm: `级别状态: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}   
规则名称: {{$event.RuleName}}{{if $event.RuleNote}}   
规则备注: {{$event.RuleNote}}{{end}}   
监控指标: {{$event.TagsJSON}}   
{{if $event.IsRecovered}}恢复时间：{{timeformat $event.LastEvalTime}}{{else}}触发时间: {{timeformat $event.TriggerTime}}   
触发时值: {{$event.TriggerValue}}{{end}}   
发送时间: {{timestamp}}`,
	Telegram: `**级别状态**: {{if $event.IsRecovered}}<font color="info">S{{$event.Severity}} Recovered</font>{{else}}<font color="warning">S{{$event.Severity}} Triggered</font>{{end}}   
**规则标题**: {{$event.RuleName}}{{if $event.RuleNote}}   
**规则备注**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}   
**监控对象**: {{$event.TargetIdent}}{{end}}   
**监控指标**: {{$event.TagsJSON}}{{if not $event.IsRecovered}}   
**触发时值**: {{$event.TriggerValue}}{{end}}   
{{if $event.IsRecovered}}**恢复时间**: {{timeformat $event.LastEvalTime}}{{else}}**首次触发时间**: {{timeformat $event.FirstTriggerTime}}{{end}}   
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**距离首次告警**: {{humanizeDurationInterface $time_duration}}
**发送时间**: {{timestamp}}`,
	Wecom: `**级别状态**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} Recovered</font>{{else}}<font color="warning">💔S{{$event.Severity}} Triggered</font>{{end}}       
**规则标题**: {{$event.RuleName}}{{if $event.RuleNote}}   
**规则备注**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}   
**监控对象**: {{$event.TargetIdent}}{{end}}   
**监控指标**: {{$event.TagsJSON}}{{if not $event.IsRecovered}}   
**触发时值**: {{$event.TriggerValue}}{{end}}   
{{if $event.IsRecovered}}**恢复时间**: {{timeformat $event.LastEvalTime}}{{else}}**首次触发时间**: {{timeformat $event.FirstTriggerTime}}{{end}}   
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**距离首次告警**: {{humanizeDurationInterface $time_duration}}
**发送时间**: {{timestamp}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
[事件详情]({{$domain}}/alert-his-events/{{$event.Id}})|[屏蔽1小时]({{$domain}}/alert-mutes/add?busiGroup={{$event.GroupId}}&cate={{$event.Cate}}&datasource_ids={{$event.DatasourceId}}&prod={{$event.RuleProd}}{{range $key, $value := $event.TagsMap}}&tags={{$key}}%3D{{$value}}{{end}})|[查看曲线]({{$domain}}/metric/explorer?data_source_id={{$event.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{$event.PromQl|escape}})`,
	Lark: `级别状态: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}   
规则名称: {{$event.RuleName}}{{if $event.RuleNote}}   
规则备注: {{$event.RuleNote}}{{end}}   
监控指标: {{$event.TagsJSON}}
{{if $event.IsRecovered}}恢复时间：{{timeformat $event.LastEvalTime}}{{else}}触发时间: {{timeformat $event.TriggerTime}}
触发时值: {{$event.TriggerValue}}{{end}}
发送时间: {{timestamp}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
事件详情: {{$domain}}/alert-his-events/{{$event.Id}}
屏蔽1小时: {{$domain}}/alert-mutes/add?busiGroup={{$event.GroupId}}&cate={{$event.Cate}}&datasource_ids={{$event.DatasourceId}}&prod={{$event.RuleProd}}{{range $key, $value := $event.TagsMap}}&tags={{$key}}%3D{{$value}}{{end}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**告警集群:** {{$event.Cluster}}{{end}}   
**级别状态:** S{{$event.Severity}} Recovered   
**告警名称:** {{$event.RuleName}}   
**事件标签:** {{$event.TagsJSON}}   
**恢复时间:** {{timeformat $event.LastEvalTime}}   
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**持续时长**: {{humanizeDurationInterface $time_duration}}   
**告警描述:** **服务已恢复**   
{{- else }}
{{- if ne $event.Cate "host"}}   
**告警集群:** {{$event.Cluster}}{{end}}   
**级别状态:** S{{$event.Severity}} Triggered   
**告警名称:** {{$event.RuleName}}   
**事件标签:** {{$event.TagsJSON}}   
**触发时间:** {{timeformat $event.TriggerTime}}   
**发送时间:** {{timestamp}}   
**触发时值:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**持续时长**: {{humanizeDurationInterface $time_duration}}   
{{if $event.RuleNote }}**告警描述:** **{{$event.RuleNote}}**{{end}}   
{{- end -}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
[事件详情]({{$domain}}/alert-his-events/{{$event.Id}})|[屏蔽1小时]({{$domain}}/alert-mutes/add?busiGroup={{$event.GroupId}}&cate={{$event.Cate}}&datasource_ids={{$event.DatasourceId}}&prod={{$event.RuleProd}}{{range $key, $value := $event.TagsMap}}&tags={{$key}}%3D{{$value}}{{end}})|[查看曲线]({{$domain}}/metric/explorer?data_source_id={{$event.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{$event.PromQl|escape}})`,
	SlackWebhook: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
*Alarm cluster:* {{$event.Cluster}}{{end}}
*Level Status:* S{{$event.Severity}} Recovered
*Alarm name:* {{$event.RuleName}}
*Recovery time:* {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}
{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
*Duration*: {{humanizeDurationInterface $time_duration}}
*Alarm description:* *Service has been restored*
{{- else }}
{{- if ne $event.Cate "host"}}
*Alarm cluster:* {{$event.Cluster}}{{end}}
*Level Status:* S{{$event.Severity}} Triggered
*Alarm name:* {{$event.RuleName}}
*Trigger time:* {{timeformat $event.TriggerTime}}
*Sending time:* {{timestamp}}
*Trigger time value:* {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}
{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
*Duration*: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}*Alarm description:* *{{$event.RuleNote}}*{{end}}
{{- end -}}

{{$domain := "http://127.0.0.1:17000" }}   
<{{$domain}}/alert-his-events/{{$event.Id}}|Event Details> 
<{{$domain}}/alert-mutes/add?busiGroup={{$event.GroupId}}&cate={{$event.Cate}}&datasource_ids={{$event.DatasourceId}}&prod={{$event.RuleProd}}{{range $key, $value := $event.TagsMap}}&tags={{$key}}%3D{{$value}}{{end}}|Block for 1 hour> 
<{{$domain}}/metric/explorer?data_source_id={{$event.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{$event.PromQl|escape}}|View Curve>`,
	Discord: `**Level Status**: {{if $event.IsRecovered}}S{{$event.Severity}} Recovered{{else}}S{{$event.Severity}} Triggered{{end}}   
**Rule Title**: {{$event.RuleName}}{{if $event.RuleNote}}   
**Rule Note**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}   
**Monitor Target**: {{$event.TargetIdent}}{{end}}   
**Metrics**: {{$event.TagsJSON}}{{if not $event.IsRecovered}}   
**Trigger Value**: {{$event.TriggerValue}}{{end}}   
{{if $event.IsRecovered}}**Recovery Time**: {{timeformat $event.LastEvalTime}}{{else}}**First Trigger Time**: {{timeformat $event.FirstTriggerTime}}{{end}}   
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Time Since First Alert**: {{humanizeDurationInterface $time_duration}}
**Send Time**: {{timestamp}}

{{$domain := "http://127.0.0.1:17000" }}
{{$mutelink := print $domain "/alert-mutes/add?busiGroup=" $event.GroupId "&cate=" $event.Cate "&datasource_ids=" $event.DatasourceId "&prod=" $event.RuleProd}}
{{- range $key, $value := $event.TagsMap}}
{{- $encodedValue := $value | urlquery }}
{{- $mutelink = print $mutelink "&tags=" $key "%3D" $encodedValue}}
{{- end}}
[Event Details]({{$domain}}/alert-his-events/{{$event.Id}}) | [Silence 1h]({{$mutelink}}) | [View Graph]({{$domain}}/metric/explorer?data_source_id={{$event.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{$event.PromQl|urlquery}})`,

	MattermostWebhook: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**Alarm cluster:** {{$event.Cluster}}{{end}}   
**Level Status:** S{{$event.Severity}} Recovered   
**Alarm name:** {{$event.RuleName}}   
**Recovery time:** {{timeformat $event.LastEvalTime}}   
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Duration**: {{humanizeDurationInterface $time_duration}}   
**Alarm description:** **Service has been restored**   
{{- else }}
{{- if ne $event.Cate "host"}}   
**Alarm cluster:** {{$event.Cluster}}{{end}}   
**Level Status:** S{{$event.Severity}} Triggered   
**Alarm name:** {{$event.RuleName}}   
**Trigger time:** {{timeformat $event.TriggerTime}}   
**Sending time:** {{timestamp}}   
**Trigger time value:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Duration**: {{humanizeDurationInterface $time_duration}}   
{{if $event.RuleNote }}**Alarm description:** **{{$event.RuleNote}}**{{end}}   
{{- end -}}
{{$domain := "http://127.0.0.1:17000" }}   
[Event Details]({{$domain}}/alert-his-events/{{$event.Id}})|[Block for 1 hour]({{$domain}}/alert-mutes/add?busiGroup={{$event.GroupId}}&cate={{$event.Cate}}&datasource_ids={{$event.DatasourceId}}&prod={{$event.RuleProd}}{{range $key, $value := $event.TagsMap}}&tags={{$key}}%3D{{$value}}{{end}})|[View Curve]({{$domain}}/metric/explorer?data_source_id={{$event.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{$event.PromQl|escape}})`,
	FeishuApp: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**告警集群:** {{$event.Cluster}}{{end}}   
**级别状态:** S{{$event.Severity}} Recovered   
**告警名称:** {{$event.RuleName}}  
**事件标签:** {{$event.TagsJSON}}   
**恢复时间:** {{timeformat $event.LastEvalTime}}   
**告警描述:** **服务已恢复**   
{{- else }}
{{- if ne $event.Cate "host"}}   
**告警集群:** {{$event.Cluster}}{{end}}   
**级别状态:** S{{$event.Severity}} Triggered   
**告警名称:** {{$event.RuleName}}  
**事件标签:** {{$event.TagsJSON}}   
**触发时间:** {{timeformat $event.TriggerTime}}   
**发送时间:** {{timestamp}}   
**触发时值:** {{$event.TriggerValue}}   
{{if $event.RuleNote }}**告警描述:** **{{$event.RuleNote}}**{{end}}   
{{- end -}}`,
}

var MsgTplMap = []MessageTemplate{
	{Name: "Callback", Ident: "callback", Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice", Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms", Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice", Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms", Weight: 7, Content: map[string]string{"content": NewTplMap["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram, Weight: 6, Content: map[string]string{"content": NewTplMap[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMap[LarkCard]}},
	{Name: "Lark", Ident: Lark, Weight: 5, Content: map[string]string{"content": NewTplMap[Lark]}},
	{Name: "Feishu", Ident: Feishu, Weight: 4, Content: map[string]string{"content": NewTplMap[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMap[FeishuCard]}},
	{Name: "FeishuApp", Ident: FeishuApp, Weight: 4, Content: map[string]string{"title": FeishuAppTitle, "content": NewTplMap[FeishuApp]}},
	{Name: "Wecom", Ident: Wecom, Weight: 3, Content: map[string]string{"content": NewTplMap[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk, Weight: 2, Content: map[string]string{"title": NewTplMap[EmailSubject], "content": NewTplMap[Dingtalk]}},
	{Name: "Email", Ident: Email, Weight: 1, Content: map[string]string{"subject": NewTplMap[EmailSubject], "content": NewTplMap[Email]}},
}

func InitMessageTemplate(ctx *ctx.Context) {
	if !ctx.IsCenter {
		return
	}

	for _, tpl := range MsgTplMap {
		msgTpl := MessageTemplate{
			Name:               tpl.Name,
			Ident:              tpl.Ident,
			Content:            tpl.Content,
			NotifyChannelIdent: tpl.Ident,
			CreateBy:           "system",
			CreateAt:           time.Now().Unix(),
			UpdateBy:           "system",
			UpdateAt:           time.Now().Unix(),
			Weight:             tpl.Weight,
		}

		err := msgTpl.Upsert(ctx, msgTpl.Ident)
		if err != nil {
			logger.Warningf("failed to upsert msg tpls %v", err)
		}
	}
}

func (t *MessageTemplate) Upsert(ctx *ctx.Context, ident string) error {
	tpl, err := MessageTemplateGet(ctx, "ident = ?", ident)
	if err != nil {
		return errors.WithMessage(err, "failed to get message tpl")
	}
	if tpl == nil {
		return Insert(ctx, t)
	}

	if tpl.UpdateBy != "" && tpl.UpdateBy != "system" {
		return nil
	}
	return tpl.Update(ctx, *t)
}

func (t *MessageTemplate) RenderEvent(events []*AlertCurEvent) map[string]interface{} {
	if t == nil {
		return nil
	}
	// event 内容渲染到 messageTemplate
	tplContent := make(map[string]interface{})
	for key, msgTpl := range t.Content {
		var defs = []string{
			"{{ $events := . }}",
			"{{ $event := index $events 0 }}",
			"{{ $labels := $event.TagsMap }}",
			"{{ $value := $event.TriggerValue }}",
		}

		var body bytes.Buffer
		if t.NotifyChannelIdent == "email" {
			text := strings.Join(append(defs, msgTpl), "")
			tpl, err := texttemplate.New(key).Funcs(tplx.TemplateFuncMap).Parse(text)
			if err != nil {
				logger.Errorf("failed to parse template: %v", err)
				tplContent[key] = fmt.Sprintf("failed to parse template: %v", err)
				continue
			}

			var body bytes.Buffer
			if err = tpl.Execute(&body, events); err != nil {
				logger.Errorf("failed to execute template: %v", err)
				tplContent[key] = fmt.Sprintf("failed to execute template: %v", err)
				continue
			}
			tplContent[key] = body.String()
			continue
		} else if t.NotifyChannelIdent == "slackwebhook" || t.NotifyChannelIdent == "slackbot" {
			text := strings.Join(append(defs, msgTpl), "")
			tpl, err := template.New(key).Funcs(tplx.TemplateFuncMap).Parse(text)
			if err != nil {
				logger.Errorf("failed to parse template: %v events: %v", err, events)
				continue
			}

			if err = tpl.Execute(&body, events); err != nil {
				logger.Errorf("failed to execute template: %v events: %v", err, events)
				continue
			}

			escaped := strings.ReplaceAll(body.String(), `"`, `\"`)
			escaped = strings.ReplaceAll(escaped, "\n", "\\n")
			escaped = strings.ReplaceAll(escaped, "\r", "\\r")
			escaped = strings.ReplaceAll(escaped, "&lt;", "<")
			tplContent[key] = template.HTML(escaped)
			continue
		}

		text := strings.Join(append(defs, msgTpl), "")
		tpl, err := template.New(key).Funcs(tplx.TemplateFuncMap).Parse(text)
		if err != nil {
			logger.Errorf("failed to parse template: %v events: %v", err, events)
			tplContent[key] = fmt.Sprintf("failed to parse template: %v", err)
			continue
		}

		if err = tpl.Execute(&body, events); err != nil {
			logger.Errorf("failed to execute template: %v events: %v", err, events)
			tplContent[key] = fmt.Sprintf("failed to execute template: %v", err)
			continue
		}

		escaped := strings.ReplaceAll(body.String(), `"`, `\"`)
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		escaped = strings.ReplaceAll(escaped, "\r", "\\r")
		tplContent[key] = template.HTML(escaped)
	}
	return tplContent
}
