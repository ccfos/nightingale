package models

import (
	"bytes"
	"fmt"
	"html/template"
	"regexp"
	"sort"
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
	Lang               string            `json:"lang"`                 // 模板语言，为空视为中文（兼容存量数据）
	CreateAt           int64             `json:"create_at"`
	CreateBy           string            `json:"create_by"`
	UpdateAt           int64             `json:"update_at"`
	UpdateBy           string            `json:"update_by"`
	UpdateByNickname   string            `json:"update_by_nickname" gorm:"-"`
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

	// 内置模板中英文版 weight 相同，按 id 兜底使排序确定（调用方取首个作为默认模板）
	err := session.Order("weight asc, id asc").Find(&lst).Error
	if err != nil {
		return nil, err
	}
	for _, t := range lst {
		t.DB2FE()
	}
	return lst, nil
}

const (
	MsgTplLangEn = "en"
	// MsgTplLangJa 必须与 NormalizeMsgTplLang 对日语请求头的返回值逐字相同
	MsgTplLangJa = "ja_JP"
)

// NormalizeMsgTplLang 归一化 X-Language 请求头或模板 lang 字段：
// 空值与 zh 前缀（zh_CN、zh_HK）均视为中文，返回空串（存量模板 lang 为空）；
// en 前缀（en、en_US）归一为 en；其他语言原样返回。
//
// 这里不接 i18nx.NormalizeLang：DB 里存量行的 lang 是 "" 和 "en"，改归一化结果
// 会让 FilterMsgTplsByLang 的英文兜底对不上存量数据。因此增补其他语言的内置模板时，
// 种子行的 Lang 必须与本函数对该语言请求头的返回值逐字相同——日语请求头到这里是
// "ja_JP"（原样返回），种子行写 "ja" 则永远匹配不上
func NormalizeMsgTplLang(lang string) string {
	switch {
	case lang == "" || strings.HasPrefix(lang, "zh"):
		return ""
	case strings.HasPrefix(lang, MsgTplLangEn):
		return MsgTplLangEn
	default:
		return lang
	}
}

// FilterMsgTplsByLang 按请求语言过滤内置模板（CreateBy=="system"，有中英两套），
// 请求语言没有对应的内置模板时先回退英文，英文也没有则回退中文。
// 用户自建模板与语言无关，始终保留：其 lang 仅记录创建时的界面语言，若参与过滤，
// 存量自建模板（迁移后 lang 默认为空）和跨语言团队互建的模板会对对方隐藏，
// 导致配置通知规则时无法从列表选中。
func FilterMsgTplsByLang(lst []*MessageTemplate, reqLang string) []*MessageTemplate {
	want := NormalizeMsgTplLang(reqLang)

	hasSysLang := func(lang string) bool {
		for _, t := range lst {
			if t.CreateBy == "system" && NormalizeMsgTplLang(t.Lang) == lang {
				return true
			}
		}
		return false
	}

	// 回退兜底，避免英文模板缺失（如手工执行 SQL 迁移滞后）时内置模板整体被过滤成空
	if !hasSysLang(want) {
		if want != MsgTplLangEn && hasSysLang(MsgTplLangEn) {
			want = MsgTplLangEn
		} else {
			want = ""
		}
	}

	res := make([]*MessageTemplate, 0, len(lst))
	for _, t := range lst {
		if t.CreateBy != "system" || NormalizeMsgTplLang(t.Lang) == want {
			res = append(res, t)
		}
	}
	return res
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
{{if $event.AnnotationsJSON}}
- **附加信息**:
{{- range $key, $val := $event.AnnotationsJSON}}
	- {{$key}}: {{$val}}
{{- end}}
{{end}}
[事件详情]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [屏蔽1小时]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [查看曲线]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
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
附加信息:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}  
{{if $event.IsRecovered}}恢复时间：{{timeformat $event.LastEvalTime}}{{else}}触发时间: {{timeformat $event.TriggerTime}}
触发时值: {{$event.TriggerValue}}{{end}}
发送时间: {{timestamp}}   
事件详情: {{.domain}}/share/alert-his-events/{{$event.Id}}   
屏蔽1小时: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
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
{{if $event.AnnotationsJSON}}
**附加信息**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}} 
{{- end}}
[事件详情]({{.domain}}/share/alert-his-events/{{$event.Id}})|[屏蔽1小时]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[查看曲线]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	EmailSubject: `{{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
	Mm: `级别状态: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}   
规则名称: {{$event.RuleName}}{{if $event.RuleNote}}   
规则备注: {{$event.RuleNote}}{{end}}   
监控指标: {{$event.TagsJSON}}   
{{if $event.IsRecovered}}恢复时间：{{timeformat $event.LastEvalTime}}{{else}}触发时间: {{timeformat $event.TriggerTime}}   
触发时值: {{$event.TriggerValue}}{{end}}   
发送时间: {{timestamp}}`,
	Telegram: `<b>级别状态: {{if $event.IsRecovered}}💚 S{{$event.Severity}} Recovered{{else}}⚠️ S{{$event.Severity}} Triggered{{end}}</b>
<b>规则标题</b>: {{$event.RuleName}}{{if $event.RuleNote}}   
<b>规则备注</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}   
<b>监控对象</b>: {{$event.TargetIdent}}{{end}}   
<b>监控指标</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}   
<b>触发时值</b>: {{$event.TriggerValue}}{{end}}   
{{if $event.IsRecovered}}<b>恢复时间</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>首次触发时间</b>: {{timeformat $event.FirstTriggerTime}}{{end}}   
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>距离首次告警</b>: {{humanizeDurationInterface $time_duration}}
<b>发送时间</b>: {{timestamp}}`,
	Wecom: `**级别状态**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} Recovered</font>{{else}}<font color="warning">💔S{{$event.Severity}} Triggered</font>{{end}}       
**规则标题**: {{$event.RuleName}}{{if $event.RuleNote}}   
**规则备注**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}   
**监控对象**: {{$event.TargetIdent}}{{end}}   
**监控指标**: {{$event.TagsJSON}}   
{{if $event.AnnotationsJSON}}**附加信息**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**触发时值**: {{$event.TriggerValue}}{{end}}   
{{if $event.IsRecovered}}**恢复时间**: {{timeformat $event.LastEvalTime}}{{else}}**首次触发时间**: {{timeformat $event.FirstTriggerTime}}{{end}}   
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**距离首次告警**: {{humanizeDurationInterface $time_duration}}
**发送时间**: {{timestamp}}   
[事件详情]({{.domain}}/share/alert-his-events/{{$event.Id}})|[屏蔽1小时]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[查看曲线]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `级别状态: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}   
规则名称: {{$event.RuleName}}{{if $event.RuleNote}}   
规则备注: {{$event.RuleNote}}{{end}}   
监控指标: {{$event.TagsJSON}}
{{if $event.IsRecovered}}恢复时间：{{timeformat $event.LastEvalTime}}{{else}}触发时间: {{timeformat $event.TriggerTime}}
触发时值: {{$event.TriggerValue}}{{end}}
发送时间: {{timestamp}}   
事件详情: {{.domain}}/share/alert-his-events/{{$event.Id}}
屏蔽1小时: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
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
[事件详情]({{.domain}}/share/alert-his-events/{{$event.Id}})|[屏蔽1小时]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[查看曲线]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
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

<{{.domain}}/share/alert-his-events/{{$event.Id}}|Event Details> 
<{{.domain}}/alert-mutes/add?__event_id={{$event.Id}}|Block for 1 hour> 
<{{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph|View Curve>`,
	Discord: `**Level Status**: {{if $event.IsRecovered}}S{{$event.Severity}} Recovered{{else}}S{{$event.Severity}} Triggered{{end}}   
**Rule Title**: {{$event.RuleName}}{{if $event.RuleNote}}   
**Rule Note**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}   
**Monitor Target**: {{$event.TargetIdent}}{{end}}   
**Metrics**: {{$event.TagsJSON}}{{if not $event.IsRecovered}}   
**Trigger Value**: {{$event.TriggerValue}}{{end}}   
{{if $event.IsRecovered}}**Recovery Time**: {{timeformat $event.LastEvalTime}}{{else}}**First Trigger Time**: {{timeformat $event.FirstTriggerTime}}{{end}}   
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Time Since First Alert**: {{humanizeDurationInterface $time_duration}}
**Send Time**: {{timestamp}}

[Event Details]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [Silence 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}) | [View Graph]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph)`,

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
[Event Details]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Block for 1 hour]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}})|[View Curve]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph)`,

	// Jira and JSMAlert share the same template format
	Jira: `Severity: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}
Rule Name: {{$event.RuleName}}{{if $event.RuleNote}}
Rule Notes: {{$event.RuleNote}}{{end}}
Metrics: {{$event.TagsJSON}}
Annotations:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}\n{{if $event.IsRecovered}}Recovery Time: {{timeformat $event.LastEvalTime}}{{else}}Trigger Time: {{timeformat $event.TriggerTime}}
Trigger Value: {{$event.TriggerValue}}{{end}}
Send Time: {{timestamp}}
Event Details: {{.domain}}/share/alert-his-events/{{$event.Id}}
Mute for 1 Hour: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
}

// Weight 用于页面元素排序，weight 越大 排序越靠后
var MsgTplMap = []MessageTemplate{
	{Name: "Jira", Ident: Jira, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
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
	{Name: "Wecom", Ident: Wecom, Weight: 3, Content: map[string]string{"content": NewTplMap[Wecom]}},
	//{Name: "WecomApp", Ident: "wecomapp", Weight: 3, Content: map[string]string{"title": NewTplMap[EmailSubject], "content": NewTplMap[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk, Weight: 2, Content: map[string]string{"title": NewTplMap[EmailSubject], "content": NewTplMap[Dingtalk]}},
	// TODO(dingtalkapp): 钉钉应用本次不上线，默认模板先注释；上线时恢复。
	// {Name: "DingtalkApp", Ident: "dingtalkapp", Weight: 2, Content: map[string]string{"title": NewTplMap[EmailSubject], "content": NewTplMap[Dingtalk]}},
	//{Name: "FeishuApp", Ident: "feishuapp", Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMap[FeishuCard]}},
	{Name: "Email", Ident: Email, Weight: 1, Content: map[string]string{"subject": NewTplMap[EmailSubject], "content": NewTplMap[Email]}},
}

// NewTplMapEn 内置模板的英文文案，仅收录与 NewTplMap 中文内容不同的模板；
// 本身即为英文或语言无关的模板（Jira、Slack、Discord、Mattermost、语音/短信等）直接复用 NewTplMap
var NewTplMapEn = map[string]string{
	"tx-sms": `Level Status: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}} Rule Name: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **Severity**: S{{$event.Severity}}
{{- if $event.RuleNote}}
	- **Rule Note**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **Trigger Value**: {{$event.TriggerValue}}
- **Trigger Time**: {{timeformat $event.TriggerTime}}
- **Duration**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **Recovery Value**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **Recovery Time**: {{timeformat $event.LastEvalTime}}
- **Duration**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **Event Tags**:
{{- range $key, $val := $event.TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}
{{if $event.AnnotationsJSON}}
- **Annotations**:
{{- range $key, $val := $event.AnnotationsJSON}}
	- {{$key}}: {{$val}}
{{- end}}
{{end}}
[Event Details]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [Mute 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [View Graph]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Email: `<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>Nightingale Alert Notification</title>
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
						<th>Level Status:</th>
						<td>S{{$event.Severity}} Recovered</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>Level Status:</th>
						<td>S{{$event.Severity}} Triggered</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Rule Note:</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>Target Note:</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>Trigger Value:</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>Target:</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>Metrics:</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>Recovery Time:</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>Trigger Time:</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Send Time:</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						Too many alerts? Try <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a> for alert aggregation, noise reduction and on-call scheduling!
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `Level Status: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}
Rule Name: {{$event.RuleName}}{{if $event.RuleNote}}
Rule Note: {{$event.RuleNote}}{{end}}
Metrics: {{$event.TagsJSON}}
Annotations:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{if $event.IsRecovered}}Recovery Time: {{timeformat $event.LastEvalTime}}{{else}}Trigger Time: {{timeformat $event.TriggerTime}}
Trigger Value: {{$event.TriggerValue}}{{end}}
Send Time: {{timestamp}}
Event Details: {{.domain}}/share/alert-his-events/{{$event.Id}}
Mute for 1 Hour: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**Cluster:** {{$event.Cluster}}{{end}}
**Level Status:** S{{$event.Severity}} Recovered
**Rule Name:** {{$event.RuleName}}
**Event Tags:** {{$event.TagsJSON}}
**Recovery Time:** {{timeformat $event.LastEvalTime}}
**Description:** **Service recovered**
{{- else }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Level Status:** S{{$event.Severity}} Triggered
**Rule Name:** {{$event.RuleName}}
**Event Tags:** {{$event.TagsJSON}}
**Trigger Time:** {{timeformat $event.TriggerTime}}
**Send Time:** {{timestamp}}
**Trigger Value:** {{$event.TriggerValue}}
{{if $event.RuleNote }}**Description:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
{{if $event.AnnotationsJSON}}
**Annotations**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{- end}}
[Event Details]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Mute 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[View Graph]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Telegram: `<b>Level Status: {{if $event.IsRecovered}}💚 S{{$event.Severity}} Recovered{{else}}⚠️ S{{$event.Severity}} Triggered{{end}}</b>
<b>Rule Title</b>: {{$event.RuleName}}{{if $event.RuleNote}}
<b>Rule Note</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
<b>Monitor Target</b>: {{$event.TargetIdent}}{{end}}
<b>Metrics</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}
<b>Trigger Value</b>: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}<b>Recovery Time</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>First Trigger Time</b>: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>Time Since First Alert</b>: {{humanizeDurationInterface $time_duration}}
<b>Send Time</b>: {{timestamp}}`,
	Wecom: `**Level Status**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} Recovered</font>{{else}}<font color="warning">💔S{{$event.Severity}} Triggered</font>{{end}}
**Rule Title**: {{$event.RuleName}}{{if $event.RuleNote}}
**Rule Note**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
**Monitor Target**: {{$event.TargetIdent}}{{end}}
**Metrics**: {{$event.TagsJSON}}
{{if $event.AnnotationsJSON}}**Annotations**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**Trigger Value**: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}**Recovery Time**: {{timeformat $event.LastEvalTime}}{{else}}**First Trigger Time**: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Time Since First Alert**: {{humanizeDurationInterface $time_duration}}
**Send Time**: {{timestamp}}
[Event Details]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Mute 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[View Graph]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `Level Status: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}}
Rule Name: {{$event.RuleName}}{{if $event.RuleNote}}
Rule Note: {{$event.RuleNote}}{{end}}
Metrics: {{$event.TagsJSON}}
{{if $event.IsRecovered}}Recovery Time: {{timeformat $event.LastEvalTime}}{{else}}Trigger Time: {{timeformat $event.TriggerTime}}
Trigger Value: {{$event.TriggerValue}}{{end}}
Send Time: {{timestamp}}
Event Details: {{.domain}}/share/alert-his-events/{{$event.Id}}
Mute for 1 Hour: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Level Status:** S{{$event.Severity}} Recovered
**Rule Name:** {{$event.RuleName}}
**Event Tags:** {{$event.TagsJSON}}
**Recovery Time:** {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Duration**: {{humanizeDurationInterface $time_duration}}
**Description:** **Service recovered**
{{- else }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Level Status:** S{{$event.Severity}} Triggered
**Rule Name:** {{$event.RuleName}}
**Event Tags:** {{$event.TagsJSON}}
**Trigger Time:** {{timeformat $event.TriggerTime}}
**Send Time:** {{timestamp}}
**Trigger Value:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Duration**: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}**Description:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
[Event Details]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Mute 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[View Graph]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
}

// NewTplMapJa 内置模板的日文文案，仅收录与 NewTplMap 中文内容不同的模板；
// 本身即为英文或语言无关的模板（Jira、Slack、Discord、Mattermost、语音/短信等）直接复用 NewTplMap
var NewTplMapJa = map[string]string{
	"tx-sms": `レベル・状態: S{{$event.Severity}} {{if $event.IsRecovered}}復旧{{else}}発生{{end}} ルール名: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **深刻度**: S{{$event.Severity}}
{{- if $event.RuleNote}}
	- **ルール備考**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **発生時の値**: {{$event.TriggerValue}}
- **発生時刻**: {{timeformat $event.TriggerTime}}
- **継続時間**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **復旧時の値**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **復旧時刻**: {{timeformat $event.LastEvalTime}}
- **継続時間**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **イベントラベル**:
{{- range $key, $val := $event.TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}
{{if $event.AnnotationsJSON}}
- **付加情報**:
{{- range $key, $val := $event.AnnotationsJSON}}
	- {{$key}}: {{$val}}
{{- end}}
{{end}}
[イベント詳細]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [1時間ミュート]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [グラフを表示]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Email: `<!DOCTYPE html>
	<html lang="ja">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>Nightingale アラート通知</title>
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
						<th>レベル・状態:</th>
						<td>S{{$event.Severity}} 復旧</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>レベル・状態:</th>
						<td>S{{$event.Severity}} 発生</td>
					</tr>
					{{end}}
	
					<tr>
						<th>ルール備考:</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>対象備考:</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>発生時の値:</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>監視対象:</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>監視メトリック:</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>復旧時刻:</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>発生時刻:</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>送信時刻:</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						アラートが多すぎませんか？アラートの集約・ノイズ削減・オンコール管理には <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a> をお試しください。
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `レベル・状態: S{{$event.Severity}} {{if $event.IsRecovered}}復旧{{else}}発生{{end}}
ルール名: {{$event.RuleName}}{{if $event.RuleNote}}
ルール備考: {{$event.RuleNote}}{{end}}
監視メトリック: {{$event.TagsJSON}}
付加情報:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{if $event.IsRecovered}}復旧時刻: {{timeformat $event.LastEvalTime}}{{else}}発生時刻: {{timeformat $event.TriggerTime}}
発生時の値: {{$event.TriggerValue}}{{end}}
送信時刻: {{timestamp}}
イベント詳細: {{.domain}}/share/alert-his-events/{{$event.Id}}
1時間ミュート: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**クラスタ:** {{$event.Cluster}}{{end}}
**レベル・状態:** S{{$event.Severity}} 復旧
**ルール名:** {{$event.RuleName}}
**イベントラベル:** {{$event.TagsJSON}}
**復旧時刻:** {{timeformat $event.LastEvalTime}}
**説明:** **サービスが復旧しました**
{{- else }}
{{- if ne $event.Cate "host"}}
**クラスタ:** {{$event.Cluster}}{{end}}
**レベル・状態:** S{{$event.Severity}} 発生
**ルール名:** {{$event.RuleName}}
**イベントラベル:** {{$event.TagsJSON}}
**発生時刻:** {{timeformat $event.TriggerTime}}
**送信時刻:** {{timestamp}}
**発生時の値:** {{$event.TriggerValue}}
{{if $event.RuleNote }}**説明:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
{{if $event.AnnotationsJSON}}
**付加情報**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{- end}}
[イベント詳細]({{.domain}}/share/alert-his-events/{{$event.Id}})|[1時間ミュート]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[グラフを表示]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Telegram: `<b>レベル・状態: {{if $event.IsRecovered}}💚 S{{$event.Severity}} 復旧{{else}}⚠️ S{{$event.Severity}} 発生{{end}}</b>
<b>ルール名</b>: {{$event.RuleName}}{{if $event.RuleNote}}
<b>ルール備考</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
<b>監視対象</b>: {{$event.TargetIdent}}{{end}}
<b>監視メトリック</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}
<b>発生時の値</b>: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}<b>復旧時刻</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>初回発生時刻</b>: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>初回アラートからの経過時間</b>: {{humanizeDurationInterface $time_duration}}
<b>送信時刻</b>: {{timestamp}}`,
	Wecom: `**レベル・状態**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} 復旧</font>{{else}}<font color="warning">💔S{{$event.Severity}} 発生</font>{{end}}
**ルール名**: {{$event.RuleName}}{{if $event.RuleNote}}
**ルール備考**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
**監視対象**: {{$event.TargetIdent}}{{end}}
**監視メトリック**: {{$event.TagsJSON}}
{{if $event.AnnotationsJSON}}**付加情報**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**発生時の値**: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}**復旧時刻**: {{timeformat $event.LastEvalTime}}{{else}}**初回発生時刻**: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**初回アラートからの経過時間**: {{humanizeDurationInterface $time_duration}}
**送信時刻**: {{timestamp}}
[イベント詳細]({{.domain}}/share/alert-his-events/{{$event.Id}})|[1時間ミュート]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[グラフを表示]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `レベル・状態: S{{$event.Severity}} {{if $event.IsRecovered}}復旧{{else}}発生{{end}}
ルール名: {{$event.RuleName}}{{if $event.RuleNote}}
ルール備考: {{$event.RuleNote}}{{end}}
監視メトリック: {{$event.TagsJSON}}
{{if $event.IsRecovered}}復旧時刻: {{timeformat $event.LastEvalTime}}{{else}}発生時刻: {{timeformat $event.TriggerTime}}
発生時の値: {{$event.TriggerValue}}{{end}}
送信時刻: {{timestamp}}
イベント詳細: {{.domain}}/share/alert-his-events/{{$event.Id}}
1時間ミュート: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**クラスタ:** {{$event.Cluster}}{{end}}
**レベル・状態:** S{{$event.Severity}} 復旧
**ルール名:** {{$event.RuleName}}
**イベントラベル:** {{$event.TagsJSON}}
**復旧時刻:** {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**継続時間**: {{humanizeDurationInterface $time_duration}}
**説明:** **サービスが復旧しました**
{{- else }}
{{- if ne $event.Cate "host"}}
**クラスタ:** {{$event.Cluster}}{{end}}
**レベル・状態:** S{{$event.Severity}} 発生
**ルール名:** {{$event.RuleName}}
**イベントラベル:** {{$event.TagsJSON}}
**発生時刻:** {{timeformat $event.TriggerTime}}
**送信時刻:** {{timestamp}}
**発生時の値:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**継続時間**: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}**説明:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
[イベント詳細]({{.domain}}/share/alert-his-events/{{$event.Id}})|[1時間ミュート]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[グラフを表示]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	EmailSubject: `{{if $event.IsRecovered}}復旧{{else}}発生{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
}

// MsgTplMapEn 内置模板的英文版本，与 MsgTplMap 一一对应；
// ident 追加 -en 后缀与中文版在 message_template 表中共存，NotifyChannelIdent 仍为渠道 ident
var MsgTplMapEn = []MessageTemplate{
	{Name: "Jira", Ident: Jira + "-en", NotifyChannelIdent: Jira, Lang: MsgTplLangEn, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert + "-en", NotifyChannelIdent: JSMAlert, Lang: MsgTplLangEn, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "Callback", Ident: "callback-en", NotifyChannelIdent: "callback", Lang: MsgTplLangEn, Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook + "-en", NotifyChannelIdent: MattermostWebhook, Lang: MsgTplLangEn, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot + "-en", NotifyChannelIdent: MattermostBot, Lang: MsgTplLangEn, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook + "-en", NotifyChannelIdent: SlackWebhook, Lang: MsgTplLangEn, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot + "-en", NotifyChannelIdent: SlackBot, Lang: MsgTplLangEn, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord + "-en", NotifyChannelIdent: Discord, Lang: MsgTplLangEn, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice-en", NotifyChannelIdent: "ali-voice", Lang: MsgTplLangEn, Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms-en", NotifyChannelIdent: "ali-sms", Lang: MsgTplLangEn, Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice-en", NotifyChannelIdent: "tx-voice", Lang: MsgTplLangEn, Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms-en", NotifyChannelIdent: "tx-sms", Lang: MsgTplLangEn, Weight: 7, Content: map[string]string{"content": NewTplMapEn["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram + "-en", NotifyChannelIdent: Telegram, Lang: MsgTplLangEn, Weight: 6, Content: map[string]string{"content": NewTplMapEn[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard + "-en", NotifyChannelIdent: LarkCard, Lang: MsgTplLangEn, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMapEn[LarkCard]}},
	{Name: "Lark", Ident: Lark + "-en", NotifyChannelIdent: Lark, Lang: MsgTplLangEn, Weight: 5, Content: map[string]string{"content": NewTplMapEn[Lark]}},
	{Name: "Feishu", Ident: Feishu + "-en", NotifyChannelIdent: Feishu, Lang: MsgTplLangEn, Weight: 4, Content: map[string]string{"content": NewTplMapEn[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard + "-en", NotifyChannelIdent: FeishuCard, Lang: MsgTplLangEn, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMapEn[FeishuCard]}},
	{Name: "Wecom", Ident: Wecom + "-en", NotifyChannelIdent: Wecom, Lang: MsgTplLangEn, Weight: 3, Content: map[string]string{"content": NewTplMapEn[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk + "-en", NotifyChannelIdent: Dingtalk, Lang: MsgTplLangEn, Weight: 2, Content: map[string]string{"title": NewTplMap[EmailSubject], "content": NewTplMapEn[Dingtalk]}},
	{Name: "Email", Ident: Email + "-en", NotifyChannelIdent: Email, Lang: MsgTplLangEn, Weight: 1, Content: map[string]string{"subject": NewTplMap[EmailSubject], "content": NewTplMapEn[Email]}},
}

// MsgTplMapJa 内置模板的日文版本，与 MsgTplMap 一一对应；
// ident 追加 -ja 后缀与中英文版在 message_template 表中共存，NotifyChannelIdent 仍为渠道 ident
var MsgTplMapJa = []MessageTemplate{
	{Name: "Jira", Ident: Jira + "-ja", NotifyChannelIdent: Jira, Lang: MsgTplLangJa, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert + "-ja", NotifyChannelIdent: JSMAlert, Lang: MsgTplLangJa, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "Callback", Ident: "callback-ja", NotifyChannelIdent: "callback", Lang: MsgTplLangJa, Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook + "-ja", NotifyChannelIdent: MattermostWebhook, Lang: MsgTplLangJa, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot + "-ja", NotifyChannelIdent: MattermostBot, Lang: MsgTplLangJa, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook + "-ja", NotifyChannelIdent: SlackWebhook, Lang: MsgTplLangJa, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot + "-ja", NotifyChannelIdent: SlackBot, Lang: MsgTplLangJa, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord + "-ja", NotifyChannelIdent: Discord, Lang: MsgTplLangJa, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice-ja", NotifyChannelIdent: "ali-voice", Lang: MsgTplLangJa, Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms-ja", NotifyChannelIdent: "ali-sms", Lang: MsgTplLangJa, Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice-ja", NotifyChannelIdent: "tx-voice", Lang: MsgTplLangJa, Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms-ja", NotifyChannelIdent: "tx-sms", Lang: MsgTplLangJa, Weight: 7, Content: map[string]string{"content": NewTplMapJa["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram + "-ja", NotifyChannelIdent: Telegram, Lang: MsgTplLangJa, Weight: 6, Content: map[string]string{"content": NewTplMapJa[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard + "-ja", NotifyChannelIdent: LarkCard, Lang: MsgTplLangJa, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMapJa[LarkCard]}},
	{Name: "Lark", Ident: Lark + "-ja", NotifyChannelIdent: Lark, Lang: MsgTplLangJa, Weight: 5, Content: map[string]string{"content": NewTplMapJa[Lark]}},
	{Name: "Feishu", Ident: Feishu + "-ja", NotifyChannelIdent: Feishu, Lang: MsgTplLangJa, Weight: 4, Content: map[string]string{"content": NewTplMapJa[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard + "-ja", NotifyChannelIdent: FeishuCard, Lang: MsgTplLangJa, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMapJa[FeishuCard]}},
	{Name: "Wecom", Ident: Wecom + "-ja", NotifyChannelIdent: Wecom, Lang: MsgTplLangJa, Weight: 3, Content: map[string]string{"content": NewTplMapJa[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk + "-ja", NotifyChannelIdent: Dingtalk, Lang: MsgTplLangJa, Weight: 2, Content: map[string]string{"title": NewTplMapJa[EmailSubject], "content": NewTplMapJa[Dingtalk]}},
	{Name: "Email", Ident: Email + "-ja", NotifyChannelIdent: Email, Lang: MsgTplLangJa, Weight: 1, Content: map[string]string{"subject": NewTplMapJa[EmailSubject], "content": NewTplMapJa[Email]}},
}

func InitMessageTemplate(ctx *ctx.Context) {
	if !ctx.IsCenter {
		return
	}

	tpls := make([]MessageTemplate, 0, len(MsgTplMap)+len(MsgTplMapEn)+len(MsgTplMapJa))
	tpls = append(tpls, MsgTplMap...)
	tpls = append(tpls, MsgTplMapEn...)
	tpls = append(tpls, MsgTplMapJa...)

	for _, tpl := range tpls {
		notifyChannelIdent := tpl.NotifyChannelIdent
		if notifyChannelIdent == "" {
			// 中文内置模板未显式设置渠道 ident，其 ident 即渠道 ident
			notifyChannelIdent = tpl.Ident
		}

		msgTpl := MessageTemplate{
			Name:               tpl.Name,
			Ident:              tpl.Ident,
			Content:            tpl.Content,
			NotifyChannelIdent: notifyChannelIdent,
			Lang:               tpl.Lang,
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

var GetDefs func(map[string]interface{}) []string

func getDefs(renderData map[string]interface{}) []string {
	return []string{
		"{{ $events := .events }}",
		"{{ $event := index $events 0 }}",
		"{{ $labels := $event.TagsMap }}",
		"{{ $value := $event.TriggerValue }}",
		// 站点地址不在这里声明成 $domain：它是渲染数据里的一个键（RenderEvent 填的
		// renderData["domain"]），模板里直接写 {{$.domain}} 即可，内置模板（本文件的
		// MsgTplMap/NewTplMap）用的就是这个写法。
		//
		// 之所以不额外补一个 $domain 变量：GetDefs 是可被下游覆盖的函数变量，覆盖方
		// 是照抄一份而非在基线上追加，这里每加一个变量就多一处会静默分叉的隐式契约——
		// 而 .domain 走的是数据查找，不经过 defs，两边天然一致。
		//
		// 注意 {{.domain}} 只在顶层可用，range/with 内部 dot 会被改写；对外公开的写法
		// 统一用 {{$.domain}}（$ 恒为根数据，任何位置都成立）。
	}
}

func init() {
	GetDefs = getDefs
}

func buildRenderData(events []*AlertCurEvent, siteUrl string) map[string]interface{} {
	renderData := make(map[string]interface{})
	renderData["events"] = events
	// 模板里用 {{$.domain}} 取站点地址，见 getDefs 上方的说明
	renderData["domain"] = siteUrl
	return renderData
}

// isSlackIdent 的两个 slack 媒介需要把 &lt; 还原回 <，否则 Slack 的 <url|text> 链接语法失效。
func isSlackIdent(ident string) bool {
	return ident == "slackwebhook" || ident == "slackbot"
}

// renderField 渲染单个模板字段，按 NotifyChannelIdent 选择渲染分支：
//   - email：text/template，且不做任何转义（邮件正文里的换行就是换行）
//   - slackwebhook / slackbot：html/template + JSON 转义，并把 &lt; 还原成 <
//   - 其余：html/template + JSON 转义
//
// 与 RenderEvent 的唯一区别是错误处理：这里把错误交给调用方，由调用方决定是继续
// 投递（RenderEvent：把错误文本当正文，保持既有行为）还是中止（RenderEventStrict）。
func (t *MessageTemplate) renderField(key, msgTpl string, renderData map[string]interface{}) (interface{}, error) {
	text := strings.Join(append(GetDefs(renderData), msgTpl), "")

	if t.NotifyChannelIdent == "email" {
		tpl, err := texttemplate.New(key).Funcs(tplx.TemplateFuncMap).Parse(text)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template: %v", err)
		}
		var body bytes.Buffer
		if err = tpl.Execute(&body, renderData); err != nil {
			return nil, fmt.Errorf("failed to execute template: %v", err)
		}
		return body.String(), nil
	}

	tpl, err := template.New(key).Funcs(tplx.TemplateFuncMap).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %v", err)
	}
	var body bytes.Buffer
	if err = tpl.Execute(&body, renderData); err != nil {
		return nil, fmt.Errorf("failed to execute template: %v", err)
	}

	escaped := strings.ReplaceAll(body.String(), `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	if isSlackIdent(t.NotifyChannelIdent) {
		escaped = strings.ReplaceAll(escaped, "&lt;", "<")
	}
	return template.HTML(escaped), nil
}

func (t *MessageTemplate) RenderEvent(events []*AlertCurEvent, siteUrl string) map[string]interface{} {
	if t == nil {
		return nil
	}

	renderData := buildRenderData(events, siteUrl)

	// event 内容渲染到 messageTemplate
	tplContent := make(map[string]interface{})
	for key, msgTpl := range t.Content {
		val, err := t.renderField(key, msgTpl, renderData)
		if err != nil {
			logger.Errorf("failed to render template field %s: %v events: %v", key, err, events)
			// slack 分支历来是把出错的字段整个丢掉（下游按缺字段处理），其余分支把错误
			// 文本当正文发出去。这个差异是既有行为，这里只是显式化，没有改变它。
			// 需要如实报错的场景请用 RenderEventStrict。
			if !isSlackIdent(t.NotifyChannelIdent) {
				tplContent[key] = err.Error()
			}
			continue
		}
		tplContent[key] = val
	}
	return tplContent
}

// RenderEventStrict 与 RenderEvent 渲染逻辑完全一致（共用 renderField），区别只在
// 任何字段解析/执行失败时立即返回 error，而不是把错误文本当正文继续往下发。
//
// 「保存前测试媒介配置」这类需要如实报告成败的场景必须用它：RenderEvent 的吞错语义
// 会让模板写错时接口仍报成功，而第三方群里收到的是一段 "failed to parse template: ..."，
// 错误只在真实消息里才看得见。
func (t *MessageTemplate) RenderEventStrict(events []*AlertCurEvent, siteUrl string) (map[string]interface{}, error) {
	if t == nil {
		return nil, nil
	}

	renderData := buildRenderData(events, siteUrl)

	// map 遍历顺序随机，多个字段同时写错时报哪个字段将不确定；按 key 排序保证
	// 同一份模板每次报的都是同一个字段，用户才能照着定位
	keys := make([]string, 0, len(t.Content))
	for key := range t.Content {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	tplContent := make(map[string]interface{}, len(keys))
	for _, key := range keys {
		val, err := t.renderField(key, t.Content[key], renderData)
		if err != nil {
			return nil, fmt.Errorf("template field %q: %v", key, err)
		}
		tplContent[key] = val
	}
	return tplContent, nil
}
