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
	// 非英语的语言码必须与 NormalizeMsgTplLang 对该语言请求头的返回值逐字相同，
	// 否则 FilterMsgTplsByLang 永远筛不出这套模板
	MsgTplLangJa = "ja_JP"
	MsgTplLangRu = "ru_RU"
	MsgTplLangPt = "pt_BR"
	MsgTplLangEs = "es_ES"
	MsgTplLangId = "id_ID"
	MsgTplLangKo = "ko_KR"
	MsgTplLangFr = "fr_FR"
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

// msgTplChannelKey 取模板所属渠道，用于按渠道分组做语言回退。
// NotifyChannelIdent 为空是存量数据的形态（当年中文内置模板不显式设置渠道，
// 由 InitMessageTemplate 落库时补上），此时退回 ident：ident 带语言后缀，
// 各语言变体会各自成组、全部保留，宁可多显示也不让模板凭空消失
func msgTplChannelKey(t *MessageTemplate) string {
	if t.NotifyChannelIdent != "" {
		return t.NotifyChannelIdent
	}
	return t.Ident
}

// FilterMsgTplsByLang 按请求语言过滤内置模板（CreateBy=="system"），
// 请求语言没有对应内置模板时先回退英文，英文也没有则回退中文。
// 用户自建模板与语言无关，始终保留：其 lang 仅记录创建时的界面语言，若参与过滤，
// 存量自建模板（迁移后 lang 默认为空）和跨语言团队互建的模板会对对方隐藏，
// 导致配置通知规则时无法从列表选中。
//
// 回退按「渠道」而不是按整个列表做。若取一个全局语言，只要该语言存在任意一条内置
// 模板，缺这门语言的渠道就会被整个筛掉而不是回退——下游追加的内置模板（n9e-plus
// 的北极星/灭火图等渠道只有中英两套）最先踩到，表现为模板列表里整批渠道消失。
func FilterMsgTplsByLang(lst []*MessageTemplate, reqLang string) []*MessageTemplate {
	want := NormalizeMsgTplLang(reqLang)

	// 渠道 -> 该渠道的内置模板覆盖了哪些语言
	langsByChannel := make(map[string]map[string]bool)
	for _, t := range lst {
		if t.CreateBy != SYSTEM {
			continue
		}
		ch := msgTplChannelKey(t)
		if langsByChannel[ch] == nil {
			langsByChannel[ch] = make(map[string]bool)
		}
		langsByChannel[ch][NormalizeMsgTplLang(t.Lang)] = true
	}

	// 每个渠道各自定语言：请求语言 → 英文 → 中文（中文即空串，存量模板的取值）
	pick := func(ch string) string {
		langs := langsByChannel[ch]
		switch {
		case langs[want]:
			return want
		case langs[MsgTplLangEn]:
			return MsgTplLangEn
		default:
			return ""
		}
	}

	res := make([]*MessageTemplate, 0, len(lst))
	for _, t := range lst {
		if t.CreateBy != SYSTEM {
			res = append(res, t)
			continue
		}
		if NormalizeMsgTplLang(t.Lang) == pick(msgTplChannelKey(t)) {
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

// NewTplMapRu 内置模板的俄文文案，仅收录与 NewTplMap 中文内容不同的模板；
// 本身即为英文或语言无关的模板（Jira、Slack、Discord、Mattermost、语音/短信等）直接复用 NewTplMap
var NewTplMapRu = map[string]string{
	"tx-sms": `Уровень и статус: S{{$event.Severity}} {{if $event.IsRecovered}}Восстановлено{{else}}Сработало{{end}} Название правила: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **Уровень важности**: S{{$event.Severity}}
{{- if $event.RuleNote}}
	- **Примечание к правилу**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **Значение при срабатывании**: {{$event.TriggerValue}}
- **Время срабатывания**: {{timeformat $event.TriggerTime}}
- **Длительность**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **Значение при восстановлении**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **Время восстановления**: {{timeformat $event.LastEvalTime}}
- **Длительность**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **Метки события**:
{{- range $key, $val := $event.TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}
{{if $event.AnnotationsJSON}}
- **Аннотации**:
{{- range $key, $val := $event.AnnotationsJSON}}
	- {{$key}}: {{$val}}
{{- end}}
{{end}}
[Детали события]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [Заглушить на 1 час]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [Открыть график]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Email: `<!DOCTYPE html>
	<html lang="ru">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>Уведомление об оповещении Nightingale</title>
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
						<th>Уровень и статус:</th>
						<td>S{{$event.Severity}} Восстановлено</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>Уровень и статус:</th>
						<td>S{{$event.Severity}} Сработало</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Примечание к правилу:</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>Примечание к объекту:</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>Значение при срабатывании:</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>Объект мониторинга:</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>Метрики:</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>Время восстановления:</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>Время срабатывания:</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Время отправки:</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						Слишком много оповещений? Попробуйте <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a> — агрегация оповещений, снижение шума и дежурные расписания.
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `Уровень и статус: S{{$event.Severity}} {{if $event.IsRecovered}}Восстановлено{{else}}Сработало{{end}}
Название правила: {{$event.RuleName}}{{if $event.RuleNote}}
Примечание к правилу: {{$event.RuleNote}}{{end}}
Метрики: {{$event.TagsJSON}}
Аннотации:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{if $event.IsRecovered}}Время восстановления: {{timeformat $event.LastEvalTime}}{{else}}Время срабатывания: {{timeformat $event.TriggerTime}}
Значение при срабатывании: {{$event.TriggerValue}}{{end}}
Время отправки: {{timestamp}}
Детали события: {{.domain}}/share/alert-his-events/{{$event.Id}}
Заглушить на 1 час: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**Кластер:** {{$event.Cluster}}{{end}}
**Уровень и статус:** S{{$event.Severity}} Восстановлено
**Название правила:** {{$event.RuleName}}
**Метки события:** {{$event.TagsJSON}}
**Время восстановления:** {{timeformat $event.LastEvalTime}}
**Описание:** **Сервис восстановлен**
{{- else }}
{{- if ne $event.Cate "host"}}
**Кластер:** {{$event.Cluster}}{{end}}
**Уровень и статус:** S{{$event.Severity}} Сработало
**Название правила:** {{$event.RuleName}}
**Метки события:** {{$event.TagsJSON}}
**Время срабатывания:** {{timeformat $event.TriggerTime}}
**Время отправки:** {{timestamp}}
**Значение при срабатывании:** {{$event.TriggerValue}}
{{if $event.RuleNote }}**Описание:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
{{if $event.AnnotationsJSON}}
**Аннотации**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{- end}}
[Детали события]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Заглушить на 1 час]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Открыть график]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Telegram: `<b>Уровень и статус: {{if $event.IsRecovered}}💚 S{{$event.Severity}} Восстановлено{{else}}⚠️ S{{$event.Severity}} Сработало{{end}}</b>
<b>Название правила</b>: {{$event.RuleName}}{{if $event.RuleNote}}
<b>Примечание к правилу</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
<b>Объект мониторинга</b>: {{$event.TargetIdent}}{{end}}
<b>Метрики</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}
<b>Значение при срабатывании</b>: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}<b>Время восстановления</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>Время первого срабатывания</b>: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>Время с первого оповещения</b>: {{humanizeDurationInterface $time_duration}}
<b>Время отправки</b>: {{timestamp}}`,
	Wecom: `**Уровень и статус**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} Восстановлено</font>{{else}}<font color="warning">💔S{{$event.Severity}} Сработало</font>{{end}}
**Название правила**: {{$event.RuleName}}{{if $event.RuleNote}}
**Примечание к правилу**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
**Объект мониторинга**: {{$event.TargetIdent}}{{end}}
**Метрики**: {{$event.TagsJSON}}
{{if $event.AnnotationsJSON}}**Аннотации**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**Значение при срабатывании**: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}**Время восстановления**: {{timeformat $event.LastEvalTime}}{{else}}**Время первого срабатывания**: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Время с первого оповещения**: {{humanizeDurationInterface $time_duration}}
**Время отправки**: {{timestamp}}
[Детали события]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Заглушить на 1 час]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Открыть график]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `Уровень и статус: S{{$event.Severity}} {{if $event.IsRecovered}}Восстановлено{{else}}Сработало{{end}}
Название правила: {{$event.RuleName}}{{if $event.RuleNote}}
Примечание к правилу: {{$event.RuleNote}}{{end}}
Метрики: {{$event.TagsJSON}}
{{if $event.IsRecovered}}Время восстановления: {{timeformat $event.LastEvalTime}}{{else}}Время срабатывания: {{timeformat $event.TriggerTime}}
Значение при срабатывании: {{$event.TriggerValue}}{{end}}
Время отправки: {{timestamp}}
Детали события: {{.domain}}/share/alert-his-events/{{$event.Id}}
Заглушить на 1 час: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**Кластер:** {{$event.Cluster}}{{end}}
**Уровень и статус:** S{{$event.Severity}} Восстановлено
**Название правила:** {{$event.RuleName}}
**Метки события:** {{$event.TagsJSON}}
**Время восстановления:** {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Длительность**: {{humanizeDurationInterface $time_duration}}
**Описание:** **Сервис восстановлен**
{{- else }}
{{- if ne $event.Cate "host"}}
**Кластер:** {{$event.Cluster}}{{end}}
**Уровень и статус:** S{{$event.Severity}} Сработало
**Название правила:** {{$event.RuleName}}
**Метки события:** {{$event.TagsJSON}}
**Время срабатывания:** {{timeformat $event.TriggerTime}}
**Время отправки:** {{timestamp}}
**Значение при срабатывании:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Длительность**: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}**Описание:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
[Детали события]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Заглушить на 1 час]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Открыть график]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	EmailSubject: `{{if $event.IsRecovered}}Восстановлено{{else}}Сработало{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
}

// NewTplMapFr 内置模板的fr_FR文案，仅收录与 NewTplMap 中文内容不同的模板；
// 本身即为英文或语言无关的模板（Jira、Slack、Discord、Mattermost、语音/短信等）直接复用 NewTplMap
var NewTplMapFr = map[string]string{
	"tx-sms": `Gravité et état: S{{$event.Severity}} {{if $event.IsRecovered}}Résolu{{else}}Déclenché{{end}} Nom de la règle: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **Gravité**: S{{$event.Severity}}
{{- if $event.RuleNote}}
	- **Remarque sur la règle**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **Valeur au déclenchement**: {{$event.TriggerValue}}
- **Heure de déclenchement**: {{timeformat $event.TriggerTime}}
- **Durée**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **Valeur à la résolution**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **Heure de résolution**: {{timeformat $event.LastEvalTime}}
- **Durée**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **Étiquettes de l'événement**:
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
[Détail de l'événement]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [Sourdine 1 h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [Voir le graphique]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Email: `<!DOCTYPE html>
	<html lang="fr">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>Notification d'alerte Nightingale</title>
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
						<th>Gravité et état:</th>
						<td>S{{$event.Severity}} Résolu</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>Gravité et état:</th>
						<td>S{{$event.Severity}} Déclenché</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Remarque sur la règle:</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>Remarque sur l'objet:</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>Valeur au déclenchement:</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>Objet supervisé:</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>Métriques:</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>Heure de résolution:</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>Heure de déclenchement:</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Heure d'envoi:</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						Trop d'alertes ? Essayez <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a> pour les regrouper, réduire le bruit et organiser les astreintes.
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `Gravité et état: S{{$event.Severity}} {{if $event.IsRecovered}}Résolu{{else}}Déclenché{{end}}
Nom de la règle: {{$event.RuleName}}{{if $event.RuleNote}}
Remarque sur la règle: {{$event.RuleNote}}{{end}}
Métriques: {{$event.TagsJSON}}
Annotations:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{if $event.IsRecovered}}Heure de résolution: {{timeformat $event.LastEvalTime}}{{else}}Heure de déclenchement: {{timeformat $event.TriggerTime}}
Valeur au déclenchement: {{$event.TriggerValue}}{{end}}
Heure d'envoi: {{timestamp}}
Détail de l'événement: {{.domain}}/share/alert-his-events/{{$event.Id}}
Mettre en sourdine 1 heure: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**Cluster:** {{$event.Cluster}}{{end}}
**Gravité et état:** S{{$event.Severity}} Résolu
**Nom de la règle:** {{$event.RuleName}}
**Étiquettes de l'événement:** {{$event.TagsJSON}}
**Heure de résolution:** {{timeformat $event.LastEvalTime}}
**Description:** **Service rétabli**
{{- else }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Gravité et état:** S{{$event.Severity}} Déclenché
**Nom de la règle:** {{$event.RuleName}}
**Étiquettes de l'événement:** {{$event.TagsJSON}}
**Heure de déclenchement:** {{timeformat $event.TriggerTime}}
**Heure d'envoi:** {{timestamp}}
**Valeur au déclenchement:** {{$event.TriggerValue}}
{{if $event.RuleNote }}**Description:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
{{if $event.AnnotationsJSON}}
**Annotations**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{- end}}
[Détail de l'événement]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Sourdine 1 h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Voir le graphique]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Telegram: `<b>Gravité et état: {{if $event.IsRecovered}}💚 S{{$event.Severity}} Résolu{{else}}⚠️ S{{$event.Severity}} Déclenché{{end}}</b>
<b>Nom de la règle</b>: {{$event.RuleName}}{{if $event.RuleNote}}
<b>Remarque sur la règle</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
<b>Objet supervisé</b>: {{$event.TargetIdent}}{{end}}
<b>Métriques</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}
<b>Valeur au déclenchement</b>: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}<b>Heure de résolution</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>Premier déclenchement</b>: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>Temps depuis la première alerte</b>: {{humanizeDurationInterface $time_duration}}
<b>Heure d'envoi</b>: {{timestamp}}`,
	Wecom: `**Gravité et état**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} Résolu</font>{{else}}<font color="warning">💔S{{$event.Severity}} Déclenché</font>{{end}}
**Nom de la règle**: {{$event.RuleName}}{{if $event.RuleNote}}
**Remarque sur la règle**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
**Objet supervisé**: {{$event.TargetIdent}}{{end}}
**Métriques**: {{$event.TagsJSON}}
{{if $event.AnnotationsJSON}}**Annotations**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**Valeur au déclenchement**: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}**Heure de résolution**: {{timeformat $event.LastEvalTime}}{{else}}**Premier déclenchement**: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Temps depuis la première alerte**: {{humanizeDurationInterface $time_duration}}
**Heure d'envoi**: {{timestamp}}
[Détail de l'événement]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Sourdine 1 h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Voir le graphique]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `Gravité et état: S{{$event.Severity}} {{if $event.IsRecovered}}Résolu{{else}}Déclenché{{end}}
Nom de la règle: {{$event.RuleName}}{{if $event.RuleNote}}
Remarque sur la règle: {{$event.RuleNote}}{{end}}
Métriques: {{$event.TagsJSON}}
{{if $event.IsRecovered}}Heure de résolution: {{timeformat $event.LastEvalTime}}{{else}}Heure de déclenchement: {{timeformat $event.TriggerTime}}
Valeur au déclenchement: {{$event.TriggerValue}}{{end}}
Heure d'envoi: {{timestamp}}
Détail de l'événement: {{.domain}}/share/alert-his-events/{{$event.Id}}
Mettre en sourdine 1 heure: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Gravité et état:** S{{$event.Severity}} Résolu
**Nom de la règle:** {{$event.RuleName}}
**Étiquettes de l'événement:** {{$event.TagsJSON}}
**Heure de résolution:** {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Durée**: {{humanizeDurationInterface $time_duration}}
**Description:** **Service rétabli**
{{- else }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Gravité et état:** S{{$event.Severity}} Déclenché
**Nom de la règle:** {{$event.RuleName}}
**Étiquettes de l'événement:** {{$event.TagsJSON}}
**Heure de déclenchement:** {{timeformat $event.TriggerTime}}
**Heure d'envoi:** {{timestamp}}
**Valeur au déclenchement:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Durée**: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}**Description:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
[Détail de l'événement]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Sourdine 1 h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Voir le graphique]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	EmailSubject: `{{if $event.IsRecovered}}Résolu{{else}}Déclenché{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
}

// NewTplMapKo 内置模板的ko_KR文案，仅收录与 NewTplMap 中文内容不同的模板；
// 本身即为英文或语言无关的模板（Jira、Slack、Discord、Mattermost、语音/短信等）直接复用 NewTplMap
var NewTplMapKo = map[string]string{
	"tx-sms": `등급과 상태: S{{$event.Severity}} {{if $event.IsRecovered}}복구됨{{else}}발생함{{end}} 규칙 이름: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **등급**: S{{$event.Severity}}
{{- if $event.RuleNote}}
	- **규칙 메모**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **발생 당시 값**: {{$event.TriggerValue}}
- **발생 시각**: {{timeformat $event.TriggerTime}}
- **지속 시간**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **복구 당시 값**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **복구 시각**: {{timeformat $event.LastEvalTime}}
- **지속 시간**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **이벤트 레이블**:
{{- range $key, $val := $event.TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}
{{if $event.AnnotationsJSON}}
- **주석**:
{{- range $key, $val := $event.AnnotationsJSON}}
	- {{$key}}: {{$val}}
{{- end}}
{{end}}
[이벤트 상세]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [1시간 차단]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [그래프 보기]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Email: `<!DOCTYPE html>
	<html lang="ko">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>Nightingale 알림</title>
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
						<th>등급과 상태:</th>
						<td>S{{$event.Severity}} 복구됨</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>등급과 상태:</th>
						<td>S{{$event.Severity}} 발생함</td>
					</tr>
					{{end}}
	
					<tr>
						<th>규칙 메모:</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>대상 메모:</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>발생 당시 값:</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>모니터링 대상:</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>지표:</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>복구 시각:</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>발생 시각:</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>전송 시각:</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						알림이 너무 많나요? <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a>로 알림을 묶고 잡음을 줄이고 당직 일정을 관리해 보세요.
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `등급과 상태: S{{$event.Severity}} {{if $event.IsRecovered}}복구됨{{else}}발생함{{end}}
규칙 이름: {{$event.RuleName}}{{if $event.RuleNote}}
규칙 메모: {{$event.RuleNote}}{{end}}
지표: {{$event.TagsJSON}}
주석:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{if $event.IsRecovered}}복구 시각: {{timeformat $event.LastEvalTime}}{{else}}발생 시각: {{timeformat $event.TriggerTime}}
발생 당시 값: {{$event.TriggerValue}}{{end}}
전송 시각: {{timestamp}}
이벤트 상세: {{.domain}}/share/alert-his-events/{{$event.Id}}
1시간 차단: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**클러스터:** {{$event.Cluster}}{{end}}
**등급과 상태:** S{{$event.Severity}} 복구됨
**규칙 이름:** {{$event.RuleName}}
**이벤트 레이블:** {{$event.TagsJSON}}
**복구 시각:** {{timeformat $event.LastEvalTime}}
**설명:** **서비스 복구됨**
{{- else }}
{{- if ne $event.Cate "host"}}
**클러스터:** {{$event.Cluster}}{{end}}
**등급과 상태:** S{{$event.Severity}} 발생함
**규칙 이름:** {{$event.RuleName}}
**이벤트 레이블:** {{$event.TagsJSON}}
**발생 시각:** {{timeformat $event.TriggerTime}}
**전송 시각:** {{timestamp}}
**발생 당시 값:** {{$event.TriggerValue}}
{{if $event.RuleNote }}**설명:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
{{if $event.AnnotationsJSON}}
**주석**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{- end}}
[이벤트 상세]({{.domain}}/share/alert-his-events/{{$event.Id}})|[1시간 차단]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[그래프 보기]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Telegram: `<b>등급과 상태: {{if $event.IsRecovered}}💚 S{{$event.Severity}} 복구됨{{else}}⚠️ S{{$event.Severity}} 발생함{{end}}</b>
<b>규칙 이름</b>: {{$event.RuleName}}{{if $event.RuleNote}}
<b>규칙 메모</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
<b>모니터링 대상</b>: {{$event.TargetIdent}}{{end}}
<b>지표</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}
<b>발생 당시 값</b>: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}<b>복구 시각</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>최초 발생 시각</b>: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>최초 알림 이후 경과 시간</b>: {{humanizeDurationInterface $time_duration}}
<b>전송 시각</b>: {{timestamp}}`,
	Wecom: `**등급과 상태**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} 복구됨</font>{{else}}<font color="warning">💔S{{$event.Severity}} 발생함</font>{{end}}
**규칙 이름**: {{$event.RuleName}}{{if $event.RuleNote}}
**규칙 메모**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
**모니터링 대상**: {{$event.TargetIdent}}{{end}}
**지표**: {{$event.TagsJSON}}
{{if $event.AnnotationsJSON}}**주석**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**발생 당시 값**: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}**복구 시각**: {{timeformat $event.LastEvalTime}}{{else}}**최초 발생 시각**: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**최초 알림 이후 경과 시간**: {{humanizeDurationInterface $time_duration}}
**전송 시각**: {{timestamp}}
[이벤트 상세]({{.domain}}/share/alert-his-events/{{$event.Id}})|[1시간 차단]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[그래프 보기]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `등급과 상태: S{{$event.Severity}} {{if $event.IsRecovered}}복구됨{{else}}발생함{{end}}
규칙 이름: {{$event.RuleName}}{{if $event.RuleNote}}
규칙 메모: {{$event.RuleNote}}{{end}}
지표: {{$event.TagsJSON}}
{{if $event.IsRecovered}}복구 시각: {{timeformat $event.LastEvalTime}}{{else}}발생 시각: {{timeformat $event.TriggerTime}}
발생 당시 값: {{$event.TriggerValue}}{{end}}
전송 시각: {{timestamp}}
이벤트 상세: {{.domain}}/share/alert-his-events/{{$event.Id}}
1시간 차단: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**클러스터:** {{$event.Cluster}}{{end}}
**등급과 상태:** S{{$event.Severity}} 복구됨
**규칙 이름:** {{$event.RuleName}}
**이벤트 레이블:** {{$event.TagsJSON}}
**복구 시각:** {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**지속 시간**: {{humanizeDurationInterface $time_duration}}
**설명:** **서비스 복구됨**
{{- else }}
{{- if ne $event.Cate "host"}}
**클러스터:** {{$event.Cluster}}{{end}}
**등급과 상태:** S{{$event.Severity}} 발생함
**규칙 이름:** {{$event.RuleName}}
**이벤트 레이블:** {{$event.TagsJSON}}
**발생 시각:** {{timeformat $event.TriggerTime}}
**전송 시각:** {{timestamp}}
**발생 당시 값:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**지속 시간**: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}**설명:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
[이벤트 상세]({{.domain}}/share/alert-his-events/{{$event.Id}})|[1시간 차단]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[그래프 보기]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	EmailSubject: `{{if $event.IsRecovered}}복구됨{{else}}발생함{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
}

// NewTplMapId 内置模板的id_ID文案，仅收录与 NewTplMap 中文内容不同的模板；
// 本身即为英文或语言无关的模板（Jira、Slack、Discord、Mattermost、语音/短信等）直接复用 NewTplMap
var NewTplMapId = map[string]string{
	"tx-sms": `Tingkat dan status: S{{$event.Severity}} {{if $event.IsRecovered}}Pulih{{else}}Terpicu{{end}} Nama aturan: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **Tingkat keparahan**: S{{$event.Severity}}
{{- if $event.RuleNote}}
	- **Catatan aturan**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **Nilai saat terpicu**: {{$event.TriggerValue}}
- **Waktu terpicu**: {{timeformat $event.TriggerTime}}
- **Durasi**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **Nilai saat pulih**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **Waktu pemulihan**: {{timeformat $event.LastEvalTime}}
- **Durasi**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **Label event**:
{{- range $key, $val := $event.TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}
{{if $event.AnnotationsJSON}}
- **Anotasi**:
{{- range $key, $val := $event.AnnotationsJSON}}
	- {{$key}}: {{$val}}
{{- end}}
{{end}}
[Detail event]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [Redam 1 jam]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [Lihat grafik]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Email: `<!DOCTYPE html>
	<html lang="id">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>Notifikasi alert Nightingale</title>
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
						<th>Tingkat dan status:</th>
						<td>S{{$event.Severity}} Pulih</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>Tingkat dan status:</th>
						<td>S{{$event.Severity}} Terpicu</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Catatan aturan:</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>Catatan objek:</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>Nilai saat terpicu:</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>Objek yang dipantau:</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>Metrik:</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>Waktu pemulihan:</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>Waktu terpicu:</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Waktu pengiriman:</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						Terlalu banyak alert? Coba <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a> untuk menggabungkan alert, mengurangi noise, dan mengatur jadwal jaga.
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `Tingkat dan status: S{{$event.Severity}} {{if $event.IsRecovered}}Pulih{{else}}Terpicu{{end}}
Nama aturan: {{$event.RuleName}}{{if $event.RuleNote}}
Catatan aturan: {{$event.RuleNote}}{{end}}
Metrik: {{$event.TagsJSON}}
Anotasi:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{if $event.IsRecovered}}Waktu pemulihan: {{timeformat $event.LastEvalTime}}{{else}}Waktu terpicu: {{timeformat $event.TriggerTime}}
Nilai saat terpicu: {{$event.TriggerValue}}{{end}}
Waktu pengiriman: {{timestamp}}
Detail event: {{.domain}}/share/alert-his-events/{{$event.Id}}
Redam 1 jam: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**Kluster:** {{$event.Cluster}}{{end}}
**Tingkat dan status:** S{{$event.Severity}} Pulih
**Nama aturan:** {{$event.RuleName}}
**Label event:** {{$event.TagsJSON}}
**Waktu pemulihan:** {{timeformat $event.LastEvalTime}}
**Deskripsi:** **Layanan pulih**
{{- else }}
{{- if ne $event.Cate "host"}}
**Kluster:** {{$event.Cluster}}{{end}}
**Tingkat dan status:** S{{$event.Severity}} Terpicu
**Nama aturan:** {{$event.RuleName}}
**Label event:** {{$event.TagsJSON}}
**Waktu terpicu:** {{timeformat $event.TriggerTime}}
**Waktu pengiriman:** {{timestamp}}
**Nilai saat terpicu:** {{$event.TriggerValue}}
{{if $event.RuleNote }}**Deskripsi:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
{{if $event.AnnotationsJSON}}
**Anotasi**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{- end}}
[Detail event]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Redam 1 jam]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Lihat grafik]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Telegram: `<b>Tingkat dan status: {{if $event.IsRecovered}}💚 S{{$event.Severity}} Pulih{{else}}⚠️ S{{$event.Severity}} Terpicu{{end}}</b>
<b>Nama aturan</b>: {{$event.RuleName}}{{if $event.RuleNote}}
<b>Catatan aturan</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
<b>Objek yang dipantau</b>: {{$event.TargetIdent}}{{end}}
<b>Metrik</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}
<b>Nilai saat terpicu</b>: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}<b>Waktu pemulihan</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>Pertama kali terpicu</b>: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>Waktu sejak alert pertama</b>: {{humanizeDurationInterface $time_duration}}
<b>Waktu pengiriman</b>: {{timestamp}}`,
	Wecom: `**Tingkat dan status**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} Pulih</font>{{else}}<font color="warning">💔S{{$event.Severity}} Terpicu</font>{{end}}
**Nama aturan**: {{$event.RuleName}}{{if $event.RuleNote}}
**Catatan aturan**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
**Objek yang dipantau**: {{$event.TargetIdent}}{{end}}
**Metrik**: {{$event.TagsJSON}}
{{if $event.AnnotationsJSON}}**Anotasi**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**Nilai saat terpicu**: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}**Waktu pemulihan**: {{timeformat $event.LastEvalTime}}{{else}}**Pertama kali terpicu**: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Waktu sejak alert pertama**: {{humanizeDurationInterface $time_duration}}
**Waktu pengiriman**: {{timestamp}}
[Detail event]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Redam 1 jam]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Lihat grafik]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `Tingkat dan status: S{{$event.Severity}} {{if $event.IsRecovered}}Pulih{{else}}Terpicu{{end}}
Nama aturan: {{$event.RuleName}}{{if $event.RuleNote}}
Catatan aturan: {{$event.RuleNote}}{{end}}
Metrik: {{$event.TagsJSON}}
{{if $event.IsRecovered}}Waktu pemulihan: {{timeformat $event.LastEvalTime}}{{else}}Waktu terpicu: {{timeformat $event.TriggerTime}}
Nilai saat terpicu: {{$event.TriggerValue}}{{end}}
Waktu pengiriman: {{timestamp}}
Detail event: {{.domain}}/share/alert-his-events/{{$event.Id}}
Redam 1 jam: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**Kluster:** {{$event.Cluster}}{{end}}
**Tingkat dan status:** S{{$event.Severity}} Pulih
**Nama aturan:** {{$event.RuleName}}
**Label event:** {{$event.TagsJSON}}
**Waktu pemulihan:** {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Durasi**: {{humanizeDurationInterface $time_duration}}
**Deskripsi:** **Layanan pulih**
{{- else }}
{{- if ne $event.Cate "host"}}
**Kluster:** {{$event.Cluster}}{{end}}
**Tingkat dan status:** S{{$event.Severity}} Terpicu
**Nama aturan:** {{$event.RuleName}}
**Label event:** {{$event.TagsJSON}}
**Waktu terpicu:** {{timeformat $event.TriggerTime}}
**Waktu pengiriman:** {{timestamp}}
**Nilai saat terpicu:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Durasi**: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}**Deskripsi:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
[Detail event]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Redam 1 jam]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Lihat grafik]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	EmailSubject: `{{if $event.IsRecovered}}Pulih{{else}}Terpicu{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
}

// NewTplMapEs 内置模板的es_ES文案，仅收录与 NewTplMap 中文内容不同的模板；
// 本身即为英文或语言无关的模板（Jira、Slack、Discord、Mattermost、语音/短信等）直接复用 NewTplMap
var NewTplMapEs = map[string]string{
	"tx-sms": `Severidad y estado: S{{$event.Severity}} {{if $event.IsRecovered}}Recuperado{{else}}Disparado{{end}} Nombre de la regla: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **Severidad**: S{{$event.Severity}}
{{- if $event.RuleNote}}
	- **Observación de la regla**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **Valor en el disparo**: {{$event.TriggerValue}}
- **Hora del disparo**: {{timeformat $event.TriggerTime}}
- **Duración**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **Valor en la recuperación**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **Momento de la recuperación**: {{timeformat $event.LastEvalTime}}
- **Duración**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **Etiquetas del evento**:
{{- range $key, $val := $event.TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}
{{if $event.AnnotationsJSON}}
- **Anotaciones**:
{{- range $key, $val := $event.AnnotationsJSON}}
	- {{$key}}: {{$val}}
{{- end}}
{{end}}
[Detalles del evento]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [Silenciar 1 h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [Ver el gráfico]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Email: `<!DOCTYPE html>
	<html lang="pt">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>Notificación de alerta de Nightingale</title>
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
						<th>Severidad y estado:</th>
						<td>S{{$event.Severity}} Recuperado</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>Severidad y estado:</th>
						<td>S{{$event.Severity}} Disparado</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Observación de la regla:</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>Observación del objeto:</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>Valor en el disparo:</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>Objeto monitorizado:</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>Métricas:</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>Momento de la recuperación:</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>Hora del disparo:</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Hora de envío:</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						¿Demasiadas alertas? Prueba <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a> para agregar alertas, reducir el ruido y organizar las guardias.
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `Severidad y estado: S{{$event.Severity}} {{if $event.IsRecovered}}Recuperado{{else}}Disparado{{end}}
Nombre de la regla: {{$event.RuleName}}{{if $event.RuleNote}}
Observación de la regla: {{$event.RuleNote}}{{end}}
Métricas: {{$event.TagsJSON}}
Anotaciones:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{if $event.IsRecovered}}Momento de la recuperación: {{timeformat $event.LastEvalTime}}{{else}}Hora del disparo: {{timeformat $event.TriggerTime}}
Valor en el disparo: {{$event.TriggerValue}}{{end}}
Hora de envío: {{timestamp}}
Detalles del evento: {{.domain}}/share/alert-his-events/{{$event.Id}}
Silenciar 1 hora: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**Clúster:** {{$event.Cluster}}{{end}}
**Severidad y estado:** S{{$event.Severity}} Recuperado
**Nombre de la regla:** {{$event.RuleName}}
**Etiquetas del evento:** {{$event.TagsJSON}}
**Momento de la recuperación:** {{timeformat $event.LastEvalTime}}
**Descripción:** **Servicio recuperado**
{{- else }}
{{- if ne $event.Cate "host"}}
**Clúster:** {{$event.Cluster}}{{end}}
**Severidad y estado:** S{{$event.Severity}} Disparado
**Nombre de la regla:** {{$event.RuleName}}
**Etiquetas del evento:** {{$event.TagsJSON}}
**Hora del disparo:** {{timeformat $event.TriggerTime}}
**Hora de envío:** {{timestamp}}
**Valor en el disparo:** {{$event.TriggerValue}}
{{if $event.RuleNote }}**Descripción:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
{{if $event.AnnotationsJSON}}
**Anotaciones**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{- end}}
[Detalles del evento]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Silenciar 1 h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Ver el gráfico]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Telegram: `<b>Severidad y estado: {{if $event.IsRecovered}}💚 S{{$event.Severity}} Recuperado{{else}}⚠️ S{{$event.Severity}} Disparado{{end}}</b>
<b>Nombre de la regla</b>: {{$event.RuleName}}{{if $event.RuleNote}}
<b>Observación de la regla</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
<b>Objeto monitorizado</b>: {{$event.TargetIdent}}{{end}}
<b>Métricas</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}
<b>Valor en el disparo</b>: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}<b>Momento de la recuperación</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>Primer disparo</b>: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>Tiempo desde la primera alerta</b>: {{humanizeDurationInterface $time_duration}}
<b>Hora de envío</b>: {{timestamp}}`,
	Wecom: `**Severidad y estado**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} Recuperado</font>{{else}}<font color="warning">💔S{{$event.Severity}} Disparado</font>{{end}}
**Nombre de la regla**: {{$event.RuleName}}{{if $event.RuleNote}}
**Observación de la regla**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
**Objeto monitorizado**: {{$event.TargetIdent}}{{end}}
**Métricas**: {{$event.TagsJSON}}
{{if $event.AnnotationsJSON}}**Anotaciones**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**Valor en el disparo**: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}**Momento de la recuperación**: {{timeformat $event.LastEvalTime}}{{else}}**Primer disparo**: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Tiempo desde la primera alerta**: {{humanizeDurationInterface $time_duration}}
**Hora de envío**: {{timestamp}}
[Detalles del evento]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Silenciar 1 h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Ver el gráfico]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `Severidad y estado: S{{$event.Severity}} {{if $event.IsRecovered}}Recuperado{{else}}Disparado{{end}}
Nombre de la regla: {{$event.RuleName}}{{if $event.RuleNote}}
Observación de la regla: {{$event.RuleNote}}{{end}}
Métricas: {{$event.TagsJSON}}
{{if $event.IsRecovered}}Momento de la recuperación: {{timeformat $event.LastEvalTime}}{{else}}Hora del disparo: {{timeformat $event.TriggerTime}}
Valor en el disparo: {{$event.TriggerValue}}{{end}}
Hora de envío: {{timestamp}}
Detalles del evento: {{.domain}}/share/alert-his-events/{{$event.Id}}
Silenciar 1 hora: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**Clúster:** {{$event.Cluster}}{{end}}
**Severidad y estado:** S{{$event.Severity}} Recuperado
**Nombre de la regla:** {{$event.RuleName}}
**Etiquetas del evento:** {{$event.TagsJSON}}
**Momento de la recuperación:** {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Duración**: {{humanizeDurationInterface $time_duration}}
**Descripción:** **Servicio recuperado**
{{- else }}
{{- if ne $event.Cate "host"}}
**Clúster:** {{$event.Cluster}}{{end}}
**Severidad y estado:** S{{$event.Severity}} Disparado
**Nombre de la regla:** {{$event.RuleName}}
**Etiquetas del evento:** {{$event.TagsJSON}}
**Hora del disparo:** {{timeformat $event.TriggerTime}}
**Hora de envío:** {{timestamp}}
**Valor en el disparo:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Duración**: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}**Descripción:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
[Detalles del evento]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Silenciar 1 h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Ver el gráfico]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	EmailSubject: `{{if $event.IsRecovered}}Recuperado{{else}}Disparado{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
}

// NewTplMapPt 内置模板的葡萄牙文文案，仅收录与 NewTplMap 中文内容不同的模板；
// 本身即为英文或语言无关的模板（Jira、Slack、Discord、Mattermost、语音/短信等）直接复用 NewTplMap
var NewTplMapPt = map[string]string{
	"tx-sms": `Severidade e situação: S{{$event.Severity}} {{if $event.IsRecovered}}Recuperado{{else}}Disparado{{end}} Nome da regra: {{$event.RuleName}}`,
	Dingtalk: `#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}
- **Severidade**: S{{$event.Severity}}
{{- if $event.RuleNote}}
	- **Observação da regra**: {{$event.RuleNote}}
{{- end}}
{{- if not $event.IsRecovered}}
- **Valor no disparo**: {{$event.TriggerValue}}
- **Horário do disparo**: {{timeformat $event.TriggerTime}}
- **Duração**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **Valor na recuperação**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **Momento da recuperação**: {{timeformat $event.LastEvalTime}}
- **Duração**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **Rótulos do evento**:
{{- range $key, $val := $event.TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}
{{if $event.AnnotationsJSON}}
- **Anotações**:
{{- range $key, $val := $event.AnnotationsJSON}}
	- {{$key}}: {{$val}}
{{- end}}
{{end}}
[Detalhes do evento]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [Silenciar 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}} | [Ver gráfico]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Email: `<!DOCTYPE html>
	<html lang="es">
	<head>
		<meta charset="UTF-8">
		<meta http-equiv="X-UA-Compatible" content="ie=edge">
		<title>Notificação de alerta do Nightingale</title>
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
						<th>Severidade e situação:</th>
						<td>S{{$event.Severity}} Recuperado</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>Severidade e situação:</th>
						<td>S{{$event.Severity}} Disparado</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Observação da regra:</th>
						<td>{{$event.RuleNote}}</td>
					</tr>
					<tr>
						<th>Observação do objeto:</th>
						<td>{{$event.TargetNote}}</td>
					</tr>
					{{if not $event.IsRecovered}}
					<tr>
						<th>Valor no disparo:</th>
						<td>{{$event.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if $event.TargetIdent}}
					<tr>
						<th>Objeto monitorado:</th>
						<td>{{$event.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>Métricas:</th>
						<td>{{$event.TagsJSON}}</td>
					</tr>
	
					{{if $event.IsRecovered}}
					<tr>
						<th>Momento da recuperação:</th>
						<td>{{timeformat $event.LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>Horário do disparo:</th>
						<td>
							{{timeformat $event.TriggerTime}}
						</td>
					</tr>
					{{end}}
	
					<tr>
						<th>Horário do envio:</th>
						<td>
							{{timestamp}}
						</td>
					</tr>
					</tbody>
				</table>
	
				<hr>
	
				<footer>
					<div class="copyright" style="font-style: italic">
						Alertas demais? Experimente o <a href="https://flashcat.cloud/product/flashduty/" target="_blank">FlashDuty</a> para agregar alertas, reduzir ruído e organizar plantões.
					</div>
				</footer>
			</div>
		</div>
	</div>
	</body>
	</html>`,
	Feishu: `Severidade e situação: S{{$event.Severity}} {{if $event.IsRecovered}}Recuperado{{else}}Disparado{{end}}
Nome da regra: {{$event.RuleName}}{{if $event.RuleNote}}
Observação da regra: {{$event.RuleNote}}{{end}}
Métricas: {{$event.TagsJSON}}
Anotações:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{if $event.IsRecovered}}Momento da recuperação: {{timeformat $event.LastEvalTime}}{{else}}Horário do disparo: {{timeformat $event.TriggerTime}}
Valor no disparo: {{$event.TriggerValue}}{{end}}
Horário do envio: {{timestamp}}
Detalhes do evento: {{.domain}}/share/alert-his-events/{{$event.Id}}
Silenciar por 1 hora: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	FeishuCard: `{{- if $event.IsRecovered -}}
{{- if ne $event.Cate "host" -}}
**Cluster:** {{$event.Cluster}}{{end}}
**Severidade e situação:** S{{$event.Severity}} Recuperado
**Nome da regra:** {{$event.RuleName}}
**Rótulos do evento:** {{$event.TagsJSON}}
**Momento da recuperação:** {{timeformat $event.LastEvalTime}}
**Descrição:** **Serviço recuperado**
{{- else }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Severidade e situação:** S{{$event.Severity}} Disparado
**Nome da regra:** {{$event.RuleName}}
**Rótulos do evento:** {{$event.TagsJSON}}
**Horário do disparo:** {{timeformat $event.TriggerTime}}
**Horário do envio:** {{timestamp}}
**Valor no disparo:** {{$event.TriggerValue}}
{{if $event.RuleNote }}**Descrição:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
{{if $event.AnnotationsJSON}}
**Anotações**:
{{- range $key, $val := $event.AnnotationsJSON}}
{{$key}}: {{$val}}
{{- end}}
{{- end}}
[Detalhes do evento]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Silenciar 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Ver gráfico]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Telegram: `<b>Severidade e situação: {{if $event.IsRecovered}}💚 S{{$event.Severity}} Recuperado{{else}}⚠️ S{{$event.Severity}} Disparado{{end}}</b>
<b>Nome da regra</b>: {{$event.RuleName}}{{if $event.RuleNote}}
<b>Observação da regra</b>: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
<b>Objeto monitorado</b>: {{$event.TargetIdent}}{{end}}
<b>Métricas</b>: {{$event.TagsJSON}}{{if not $event.IsRecovered}}
<b>Valor no disparo</b>: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}<b>Momento da recuperação</b>: {{timeformat $event.LastEvalTime}}{{else}}<b>Primeiro disparo</b>: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}<b>Tempo desde o primeiro alerta</b>: {{humanizeDurationInterface $time_duration}}
<b>Horário do envio</b>: {{timestamp}}`,
	Wecom: `**Severidade e situação**: {{if $event.IsRecovered}}<font color="info">💚S{{$event.Severity}} Recuperado</font>{{else}}<font color="warning">💔S{{$event.Severity}} Disparado</font>{{end}}
**Nome da regra**: {{$event.RuleName}}{{if $event.RuleNote}}
**Observação da regra**: {{$event.RuleNote}}{{end}}{{if $event.TargetIdent}}
**Objeto monitorado**: {{$event.TargetIdent}}{{end}}
**Métricas**: {{$event.TagsJSON}}
{{if $event.AnnotationsJSON}}**Anotações**:{{range $key, $val := $event.AnnotationsJSON}}{{$key}}:{{$val}}  {{end}}   {{end}}{{if not $event.IsRecovered}}
**Valor no disparo**: {{$event.TriggerValue}}{{end}}
{{if $event.IsRecovered}}**Momento da recuperação**: {{timeformat $event.LastEvalTime}}{{else}}**Primeiro disparo**: {{timeformat $event.FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Tempo desde o primeiro alerta**: {{humanizeDurationInterface $time_duration}}
**Horário do envio**: {{timestamp}}
[Detalhes do evento]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Silenciar 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Ver gráfico]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	Lark: `Severidade e situação: S{{$event.Severity}} {{if $event.IsRecovered}}Recuperado{{else}}Disparado{{end}}
Nome da regra: {{$event.RuleName}}{{if $event.RuleNote}}
Observação da regra: {{$event.RuleNote}}{{end}}
Métricas: {{$event.TagsJSON}}
{{if $event.IsRecovered}}Momento da recuperação: {{timeformat $event.LastEvalTime}}{{else}}Horário do disparo: {{timeformat $event.TriggerTime}}
Valor no disparo: {{$event.TriggerValue}}{{end}}
Horário do envio: {{timestamp}}
Detalhes do evento: {{.domain}}/share/alert-his-events/{{$event.Id}}
Silenciar por 1 hora: {{.domain}}/alert-mutes/add?__event_id={{$event.Id}}`,
	LarkCard: `{{ if $event.IsRecovered }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Severidade e situação:** S{{$event.Severity}} Recuperado
**Nome da regra:** {{$event.RuleName}}
**Rótulos do evento:** {{$event.TagsJSON}}
**Momento da recuperação:** {{timeformat $event.LastEvalTime}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Duração**: {{humanizeDurationInterface $time_duration}}
**Descrição:** **Serviço recuperado**
{{- else }}
{{- if ne $event.Cate "host"}}
**Cluster:** {{$event.Cluster}}{{end}}
**Severidade e situação:** S{{$event.Severity}} Disparado
**Nome da regra:** {{$event.RuleName}}
**Rótulos do evento:** {{$event.TagsJSON}}
**Horário do disparo:** {{timeformat $event.TriggerTime}}
**Horário do envio:** {{timestamp}}
**Valor no disparo:** {{$event.TriggerValue}}
{{$time_duration := sub now.Unix $event.FirstTriggerTime }}{{if $event.IsRecovered}}{{$time_duration = sub $event.LastEvalTime $event.FirstTriggerTime }}{{end}}**Duração**: {{humanizeDurationInterface $time_duration}}
{{if $event.RuleNote }}**Descrição:** **{{$event.RuleNote}}**{{end}}
{{- end -}}
[Detalhes do evento]({{.domain}}/share/alert-his-events/{{$event.Id}})|[Silenciar 1h]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}}){{if eq $event.Cate "prometheus"}}|[Ver gráfico]({{.domain}}/metric/explorer?__event_id={{$event.Id}}&mode=graph){{end}}`,
	EmailSubject: `{{if $event.IsRecovered}}Recuperado{{else}}Disparado{{end}}: {{$event.RuleName}} {{$event.TagsJSON}}`,
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

// MsgTplMapRu 内置模板的俄文版本，与 MsgTplMap 一一对应；
// ident 追加 -ru 后缀与其他语言版本在 message_template 表中共存，NotifyChannelIdent 仍为渠道 ident
var MsgTplMapRu = []MessageTemplate{
	{Name: "Jira", Ident: Jira + "-ru", NotifyChannelIdent: Jira, Lang: MsgTplLangRu, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert + "-ru", NotifyChannelIdent: JSMAlert, Lang: MsgTplLangRu, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "Callback", Ident: "callback-ru", NotifyChannelIdent: "callback", Lang: MsgTplLangRu, Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook + "-ru", NotifyChannelIdent: MattermostWebhook, Lang: MsgTplLangRu, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot + "-ru", NotifyChannelIdent: MattermostBot, Lang: MsgTplLangRu, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook + "-ru", NotifyChannelIdent: SlackWebhook, Lang: MsgTplLangRu, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot + "-ru", NotifyChannelIdent: SlackBot, Lang: MsgTplLangRu, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord + "-ru", NotifyChannelIdent: Discord, Lang: MsgTplLangRu, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice-ru", NotifyChannelIdent: "ali-voice", Lang: MsgTplLangRu, Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms-ru", NotifyChannelIdent: "ali-sms", Lang: MsgTplLangRu, Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice-ru", NotifyChannelIdent: "tx-voice", Lang: MsgTplLangRu, Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms-ru", NotifyChannelIdent: "tx-sms", Lang: MsgTplLangRu, Weight: 7, Content: map[string]string{"content": NewTplMapRu["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram + "-ru", NotifyChannelIdent: Telegram, Lang: MsgTplLangRu, Weight: 6, Content: map[string]string{"content": NewTplMapRu[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard + "-ru", NotifyChannelIdent: LarkCard, Lang: MsgTplLangRu, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMapRu[LarkCard]}},
	{Name: "Lark", Ident: Lark + "-ru", NotifyChannelIdent: Lark, Lang: MsgTplLangRu, Weight: 5, Content: map[string]string{"content": NewTplMapRu[Lark]}},
	{Name: "Feishu", Ident: Feishu + "-ru", NotifyChannelIdent: Feishu, Lang: MsgTplLangRu, Weight: 4, Content: map[string]string{"content": NewTplMapRu[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard + "-ru", NotifyChannelIdent: FeishuCard, Lang: MsgTplLangRu, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMapRu[FeishuCard]}},
	{Name: "Wecom", Ident: Wecom + "-ru", NotifyChannelIdent: Wecom, Lang: MsgTplLangRu, Weight: 3, Content: map[string]string{"content": NewTplMapRu[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk + "-ru", NotifyChannelIdent: Dingtalk, Lang: MsgTplLangRu, Weight: 2, Content: map[string]string{"title": NewTplMapRu[EmailSubject], "content": NewTplMapRu[Dingtalk]}},
	{Name: "Email", Ident: Email + "-ru", NotifyChannelIdent: Email, Lang: MsgTplLangRu, Weight: 1, Content: map[string]string{"subject": NewTplMapRu[EmailSubject], "content": NewTplMapRu[Email]}},
}

// MsgTplMapFr 内置模板的fr_FR版本，与 MsgTplMap 一一对应；
// ident 追加 -fr 后缀与其他语言版本在 message_template 表中共存，NotifyChannelIdent 仍为渠道 ident
var MsgTplMapFr = []MessageTemplate{
	{Name: "Jira", Ident: Jira + "-fr", NotifyChannelIdent: Jira, Lang: MsgTplLangFr, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert + "-fr", NotifyChannelIdent: JSMAlert, Lang: MsgTplLangFr, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "Callback", Ident: "callback-fr", NotifyChannelIdent: "callback", Lang: MsgTplLangFr, Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook + "-fr", NotifyChannelIdent: MattermostWebhook, Lang: MsgTplLangFr, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot + "-fr", NotifyChannelIdent: MattermostBot, Lang: MsgTplLangFr, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook + "-fr", NotifyChannelIdent: SlackWebhook, Lang: MsgTplLangFr, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot + "-fr", NotifyChannelIdent: SlackBot, Lang: MsgTplLangFr, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord + "-fr", NotifyChannelIdent: Discord, Lang: MsgTplLangFr, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice-fr", NotifyChannelIdent: "ali-voice", Lang: MsgTplLangFr, Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms-fr", NotifyChannelIdent: "ali-sms", Lang: MsgTplLangFr, Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice-fr", NotifyChannelIdent: "tx-voice", Lang: MsgTplLangFr, Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms-fr", NotifyChannelIdent: "tx-sms", Lang: MsgTplLangFr, Weight: 7, Content: map[string]string{"content": NewTplMapFr["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram + "-fr", NotifyChannelIdent: Telegram, Lang: MsgTplLangFr, Weight: 6, Content: map[string]string{"content": NewTplMapFr[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard + "-fr", NotifyChannelIdent: LarkCard, Lang: MsgTplLangFr, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMapFr[LarkCard]}},
	{Name: "Lark", Ident: Lark + "-fr", NotifyChannelIdent: Lark, Lang: MsgTplLangFr, Weight: 5, Content: map[string]string{"content": NewTplMapFr[Lark]}},
	{Name: "Feishu", Ident: Feishu + "-fr", NotifyChannelIdent: Feishu, Lang: MsgTplLangFr, Weight: 4, Content: map[string]string{"content": NewTplMapFr[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard + "-fr", NotifyChannelIdent: FeishuCard, Lang: MsgTplLangFr, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMapFr[FeishuCard]}},
	{Name: "Wecom", Ident: Wecom + "-fr", NotifyChannelIdent: Wecom, Lang: MsgTplLangFr, Weight: 3, Content: map[string]string{"content": NewTplMapFr[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk + "-fr", NotifyChannelIdent: Dingtalk, Lang: MsgTplLangFr, Weight: 2, Content: map[string]string{"title": NewTplMapFr[EmailSubject], "content": NewTplMapFr[Dingtalk]}},
	{Name: "Email", Ident: Email + "-fr", NotifyChannelIdent: Email, Lang: MsgTplLangFr, Weight: 1, Content: map[string]string{"subject": NewTplMapFr[EmailSubject], "content": NewTplMapFr[Email]}},
}

// MsgTplMapKo 内置模板的ko_KR版本，与 MsgTplMap 一一对应；
// ident 追加 -ko 后缀与其他语言版本在 message_template 表中共存，NotifyChannelIdent 仍为渠道 ident
var MsgTplMapKo = []MessageTemplate{
	{Name: "Jira", Ident: Jira + "-ko", NotifyChannelIdent: Jira, Lang: MsgTplLangKo, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert + "-ko", NotifyChannelIdent: JSMAlert, Lang: MsgTplLangKo, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "Callback", Ident: "callback-ko", NotifyChannelIdent: "callback", Lang: MsgTplLangKo, Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook + "-ko", NotifyChannelIdent: MattermostWebhook, Lang: MsgTplLangKo, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot + "-ko", NotifyChannelIdent: MattermostBot, Lang: MsgTplLangKo, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook + "-ko", NotifyChannelIdent: SlackWebhook, Lang: MsgTplLangKo, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot + "-ko", NotifyChannelIdent: SlackBot, Lang: MsgTplLangKo, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord + "-ko", NotifyChannelIdent: Discord, Lang: MsgTplLangKo, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice-ko", NotifyChannelIdent: "ali-voice", Lang: MsgTplLangKo, Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms-ko", NotifyChannelIdent: "ali-sms", Lang: MsgTplLangKo, Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice-ko", NotifyChannelIdent: "tx-voice", Lang: MsgTplLangKo, Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms-ko", NotifyChannelIdent: "tx-sms", Lang: MsgTplLangKo, Weight: 7, Content: map[string]string{"content": NewTplMapKo["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram + "-ko", NotifyChannelIdent: Telegram, Lang: MsgTplLangKo, Weight: 6, Content: map[string]string{"content": NewTplMapKo[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard + "-ko", NotifyChannelIdent: LarkCard, Lang: MsgTplLangKo, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMapKo[LarkCard]}},
	{Name: "Lark", Ident: Lark + "-ko", NotifyChannelIdent: Lark, Lang: MsgTplLangKo, Weight: 5, Content: map[string]string{"content": NewTplMapKo[Lark]}},
	{Name: "Feishu", Ident: Feishu + "-ko", NotifyChannelIdent: Feishu, Lang: MsgTplLangKo, Weight: 4, Content: map[string]string{"content": NewTplMapKo[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard + "-ko", NotifyChannelIdent: FeishuCard, Lang: MsgTplLangKo, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMapKo[FeishuCard]}},
	{Name: "Wecom", Ident: Wecom + "-ko", NotifyChannelIdent: Wecom, Lang: MsgTplLangKo, Weight: 3, Content: map[string]string{"content": NewTplMapKo[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk + "-ko", NotifyChannelIdent: Dingtalk, Lang: MsgTplLangKo, Weight: 2, Content: map[string]string{"title": NewTplMapKo[EmailSubject], "content": NewTplMapKo[Dingtalk]}},
	{Name: "Email", Ident: Email + "-ko", NotifyChannelIdent: Email, Lang: MsgTplLangKo, Weight: 1, Content: map[string]string{"subject": NewTplMapKo[EmailSubject], "content": NewTplMapKo[Email]}},
}

// MsgTplMapId 内置模板的id_ID版本，与 MsgTplMap 一一对应；
// ident 追加 -id 后缀与其他语言版本在 message_template 表中共存，NotifyChannelIdent 仍为渠道 ident
var MsgTplMapId = []MessageTemplate{
	{Name: "Jira", Ident: Jira + "-id", NotifyChannelIdent: Jira, Lang: MsgTplLangId, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert + "-id", NotifyChannelIdent: JSMAlert, Lang: MsgTplLangId, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "Callback", Ident: "callback-id", NotifyChannelIdent: "callback", Lang: MsgTplLangId, Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook + "-id", NotifyChannelIdent: MattermostWebhook, Lang: MsgTplLangId, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot + "-id", NotifyChannelIdent: MattermostBot, Lang: MsgTplLangId, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook + "-id", NotifyChannelIdent: SlackWebhook, Lang: MsgTplLangId, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot + "-id", NotifyChannelIdent: SlackBot, Lang: MsgTplLangId, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord + "-id", NotifyChannelIdent: Discord, Lang: MsgTplLangId, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice-id", NotifyChannelIdent: "ali-voice", Lang: MsgTplLangId, Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms-id", NotifyChannelIdent: "ali-sms", Lang: MsgTplLangId, Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice-id", NotifyChannelIdent: "tx-voice", Lang: MsgTplLangId, Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms-id", NotifyChannelIdent: "tx-sms", Lang: MsgTplLangId, Weight: 7, Content: map[string]string{"content": NewTplMapId["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram + "-id", NotifyChannelIdent: Telegram, Lang: MsgTplLangId, Weight: 6, Content: map[string]string{"content": NewTplMapId[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard + "-id", NotifyChannelIdent: LarkCard, Lang: MsgTplLangId, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMapId[LarkCard]}},
	{Name: "Lark", Ident: Lark + "-id", NotifyChannelIdent: Lark, Lang: MsgTplLangId, Weight: 5, Content: map[string]string{"content": NewTplMapId[Lark]}},
	{Name: "Feishu", Ident: Feishu + "-id", NotifyChannelIdent: Feishu, Lang: MsgTplLangId, Weight: 4, Content: map[string]string{"content": NewTplMapId[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard + "-id", NotifyChannelIdent: FeishuCard, Lang: MsgTplLangId, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMapId[FeishuCard]}},
	{Name: "Wecom", Ident: Wecom + "-id", NotifyChannelIdent: Wecom, Lang: MsgTplLangId, Weight: 3, Content: map[string]string{"content": NewTplMapId[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk + "-id", NotifyChannelIdent: Dingtalk, Lang: MsgTplLangId, Weight: 2, Content: map[string]string{"title": NewTplMapId[EmailSubject], "content": NewTplMapId[Dingtalk]}},
	{Name: "Email", Ident: Email + "-id", NotifyChannelIdent: Email, Lang: MsgTplLangId, Weight: 1, Content: map[string]string{"subject": NewTplMapId[EmailSubject], "content": NewTplMapId[Email]}},
}

// MsgTplMapEs 内置模板的es_ES版本，与 MsgTplMap 一一对应；
// ident 追加 -es 后缀与其他语言版本在 message_template 表中共存，NotifyChannelIdent 仍为渠道 ident
var MsgTplMapEs = []MessageTemplate{
	{Name: "Jira", Ident: Jira + "-es", NotifyChannelIdent: Jira, Lang: MsgTplLangEs, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert + "-es", NotifyChannelIdent: JSMAlert, Lang: MsgTplLangEs, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "Callback", Ident: "callback-es", NotifyChannelIdent: "callback", Lang: MsgTplLangEs, Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook + "-es", NotifyChannelIdent: MattermostWebhook, Lang: MsgTplLangEs, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot + "-es", NotifyChannelIdent: MattermostBot, Lang: MsgTplLangEs, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook + "-es", NotifyChannelIdent: SlackWebhook, Lang: MsgTplLangEs, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot + "-es", NotifyChannelIdent: SlackBot, Lang: MsgTplLangEs, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord + "-es", NotifyChannelIdent: Discord, Lang: MsgTplLangEs, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice-es", NotifyChannelIdent: "ali-voice", Lang: MsgTplLangEs, Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms-es", NotifyChannelIdent: "ali-sms", Lang: MsgTplLangEs, Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice-es", NotifyChannelIdent: "tx-voice", Lang: MsgTplLangEs, Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms-es", NotifyChannelIdent: "tx-sms", Lang: MsgTplLangEs, Weight: 7, Content: map[string]string{"content": NewTplMapEs["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram + "-es", NotifyChannelIdent: Telegram, Lang: MsgTplLangEs, Weight: 6, Content: map[string]string{"content": NewTplMapEs[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard + "-es", NotifyChannelIdent: LarkCard, Lang: MsgTplLangEs, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMapEs[LarkCard]}},
	{Name: "Lark", Ident: Lark + "-es", NotifyChannelIdent: Lark, Lang: MsgTplLangEs, Weight: 5, Content: map[string]string{"content": NewTplMapEs[Lark]}},
	{Name: "Feishu", Ident: Feishu + "-es", NotifyChannelIdent: Feishu, Lang: MsgTplLangEs, Weight: 4, Content: map[string]string{"content": NewTplMapEs[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard + "-es", NotifyChannelIdent: FeishuCard, Lang: MsgTplLangEs, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMapEs[FeishuCard]}},
	{Name: "Wecom", Ident: Wecom + "-es", NotifyChannelIdent: Wecom, Lang: MsgTplLangEs, Weight: 3, Content: map[string]string{"content": NewTplMapEs[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk + "-es", NotifyChannelIdent: Dingtalk, Lang: MsgTplLangEs, Weight: 2, Content: map[string]string{"title": NewTplMapEs[EmailSubject], "content": NewTplMapEs[Dingtalk]}},
	{Name: "Email", Ident: Email + "-es", NotifyChannelIdent: Email, Lang: MsgTplLangEs, Weight: 1, Content: map[string]string{"subject": NewTplMapEs[EmailSubject], "content": NewTplMapEs[Email]}},
}

// MsgTplMapPt 内置模板的葡萄牙文版本，与 MsgTplMap 一一对应；
// ident 追加 -pt 后缀与其他语言版本在 message_template 表中共存，NotifyChannelIdent 仍为渠道 ident
var MsgTplMapPt = []MessageTemplate{
	{Name: "Jira", Ident: Jira + "-pt", NotifyChannelIdent: Jira, Lang: MsgTplLangPt, Weight: 18, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "JSMAlert", Ident: JSMAlert + "-pt", NotifyChannelIdent: JSMAlert, Lang: MsgTplLangPt, Weight: 17, Content: map[string]string{"content": NewTplMap[Jira]}},
	{Name: "Callback", Ident: "callback-pt", NotifyChannelIdent: "callback", Lang: MsgTplLangPt, Weight: 16, Content: map[string]string{"content": ""}},
	{Name: "MattermostWebhook", Ident: MattermostWebhook + "-pt", NotifyChannelIdent: MattermostWebhook, Lang: MsgTplLangPt, Weight: 15, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "MattermostBot", Ident: MattermostBot + "-pt", NotifyChannelIdent: MattermostBot, Lang: MsgTplLangPt, Weight: 14, Content: map[string]string{"content": NewTplMap[MattermostWebhook]}},
	{Name: "SlackWebhook", Ident: SlackWebhook + "-pt", NotifyChannelIdent: SlackWebhook, Lang: MsgTplLangPt, Weight: 13, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "SlackBot", Ident: SlackBot + "-pt", NotifyChannelIdent: SlackBot, Lang: MsgTplLangPt, Weight: 12, Content: map[string]string{"content": NewTplMap[SlackWebhook]}},
	{Name: "Discord", Ident: Discord + "-pt", NotifyChannelIdent: Discord, Lang: MsgTplLangPt, Weight: 11, Content: map[string]string{"content": NewTplMap[Discord]}},
	{Name: "Aliyun Voice", Ident: "ali-voice-pt", NotifyChannelIdent: "ali-voice", Lang: MsgTplLangPt, Weight: 10, Content: map[string]string{"incident": NewTplMap["ali-voice"]}},
	{Name: "Aliyun SMS", Ident: "ali-sms-pt", NotifyChannelIdent: "ali-sms", Lang: MsgTplLangPt, Weight: 9, Content: map[string]string{"incident": NewTplMap["ali-sms"]}},
	{Name: "Tencent Voice", Ident: "tx-voice-pt", NotifyChannelIdent: "tx-voice", Lang: MsgTplLangPt, Weight: 8, Content: map[string]string{"content": NewTplMap["tx-voice"]}},
	{Name: "Tencent SMS", Ident: "tx-sms-pt", NotifyChannelIdent: "tx-sms", Lang: MsgTplLangPt, Weight: 7, Content: map[string]string{"content": NewTplMapPt["tx-sms"]}},
	{Name: "Telegram", Ident: Telegram + "-pt", NotifyChannelIdent: Telegram, Lang: MsgTplLangPt, Weight: 6, Content: map[string]string{"content": NewTplMapPt[Telegram]}},
	{Name: "LarkCard", Ident: LarkCard + "-pt", NotifyChannelIdent: LarkCard, Lang: MsgTplLangPt, Weight: 5, Content: map[string]string{"title": LarkCardTitle, "content": NewTplMapPt[LarkCard]}},
	{Name: "Lark", Ident: Lark + "-pt", NotifyChannelIdent: Lark, Lang: MsgTplLangPt, Weight: 5, Content: map[string]string{"content": NewTplMapPt[Lark]}},
	{Name: "Feishu", Ident: Feishu + "-pt", NotifyChannelIdent: Feishu, Lang: MsgTplLangPt, Weight: 4, Content: map[string]string{"content": NewTplMapPt[Feishu]}},
	{Name: "FeishuCard", Ident: FeishuCard + "-pt", NotifyChannelIdent: FeishuCard, Lang: MsgTplLangPt, Weight: 4, Content: map[string]string{"title": FeishuCardTitle, "content": NewTplMapPt[FeishuCard]}},
	{Name: "Wecom", Ident: Wecom + "-pt", NotifyChannelIdent: Wecom, Lang: MsgTplLangPt, Weight: 3, Content: map[string]string{"content": NewTplMapPt[Wecom]}},
	{Name: "Dingtalk", Ident: Dingtalk + "-pt", NotifyChannelIdent: Dingtalk, Lang: MsgTplLangPt, Weight: 2, Content: map[string]string{"title": NewTplMapPt[EmailSubject], "content": NewTplMapPt[Dingtalk]}},
	{Name: "Email", Ident: Email + "-pt", NotifyChannelIdent: Email, Lang: MsgTplLangPt, Weight: 1, Content: map[string]string{"subject": NewTplMapPt[EmailSubject], "content": NewTplMapPt[Email]}},
}

// BuiltinMsgTplLangVariants 内置模板的非中文语言版本登记表。
// 新增一门语言：加好 NewTplMapXx / MsgTplMapXx 后在这里追加一行即可，
// 落库与一致性测试都从这里取，不必再逐处补代码
var BuiltinMsgTplLangVariants = []struct {
	Lang      string
	Suffix    string
	Bodies    map[string]string
	Templates []MessageTemplate
}{
	{MsgTplLangEn, "-en", NewTplMapEn, MsgTplMapEn},
	{MsgTplLangJa, "-ja", NewTplMapJa, MsgTplMapJa},
	{MsgTplLangRu, "-ru", NewTplMapRu, MsgTplMapRu},
	{MsgTplLangPt, "-pt", NewTplMapPt, MsgTplMapPt},
	{MsgTplLangEs, "-es", NewTplMapEs, MsgTplMapEs},
	{MsgTplLangFr, "-fr", NewTplMapFr, MsgTplMapFr},
	{MsgTplLangId, "-id", NewTplMapId, MsgTplMapId},
	{MsgTplLangKo, "-ko", NewTplMapKo, MsgTplMapKo},
}

func InitMessageTemplate(ctx *ctx.Context) {
	if !ctx.IsCenter {
		return
	}

	tpls := make([]MessageTemplate, 0, len(MsgTplMap)*(1+len(BuiltinMsgTplLangVariants)))
	tpls = append(tpls, MsgTplMap...)
	for _, v := range BuiltinMsgTplLangVariants {
		tpls = append(tpls, v.Templates...)
	}

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
