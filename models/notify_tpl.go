package models

import (
	"encoding/json"
	"fmt"
	"html/template"
	"path"
	"strings"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/pkg/tplx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

type NotifyTpl struct {
	Id       int64  `json:"id"`
	Name     string `json:"name"`
	Channel  string `json:"channel"`
	Content  string `json:"content"`
	BuiltIn  bool   `json:"built_in" gorm:"-"`
	CreateAt int64  `json:"create_at"`
	CreateBy string `json:"create_by"`
	UpdateAt int64  `json:"update_at"`
	UpdateBy string `json:"update_by"`
}

func (n *NotifyTpl) TableName() string {
	return "notify_tpl"
}

func (n *NotifyTpl) Create(c *ctx.Context) error {
	return Insert(c, n)
}

func (n *NotifyTpl) UpdateContent(c *ctx.Context) error {
	return DB(c).Model(n).Select("content", "update_at", "update_by").Updates(n).Error
}

func (n *NotifyTpl) Update(c *ctx.Context) error {
	return DB(c).Model(n).Select("name", "update_at", "update_by").Updates(n).Error
}

func (n *NotifyTpl) CreateIfNotExists(c *ctx.Context, channel string) error {
	count, err := NotifyTplCountByChannel(c, channel)
	if err != nil {
		return errors.WithMessage(err, "failed to count notify tpls")
	}

	if count != 0 {
		return nil
	}

	err = n.Create(c)
	return err
}

func (n *NotifyTpl) NotifyTplDelete(ctx *ctx.Context, id int64) error {
	return DB(ctx).Where("channel not in (?) and id =? ", DefaultChannels, id).Delete(new(NotifyTpl)).Error
}

func NotifyTplCountByChannel(c *ctx.Context, channel string) (int64, error) {
	var count int64
	err := DB(c).Model(&NotifyTpl{}).Where("channel=?", channel).Count(&count).Error
	return count, err
}

func NotifyTplGets(c *ctx.Context) ([]*NotifyTpl, error) {
	if !c.IsCenter {
		lst, err := poster.GetByUrls[[]*NotifyTpl](c, "/v1/n9e/notify-tpls")
		return lst, err
	}

	var lst []*NotifyTpl
	err := DB(c).Find(&lst).Error
	return lst, err
}

func ListTpls(c *ctx.Context) (map[string]*template.Template, error) {
	notifyTpls, err := NotifyTplGets(c)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get notify tpls")
	}

	tpls := make(map[string]*template.Template)
	for _, notifyTpl := range notifyTpls {
		var defs = []string{
			"{{$labels := .TagsMap}}",
			"{{$value := .TriggerValue}}",
		}
		text := strings.Join(append(defs, notifyTpl.Content), "")
		tpl, err := template.New(notifyTpl.Channel).Funcs(tplx.TemplateFuncMap).Parse(text)
		if err != nil {
			return nil, fmt.Errorf("failed to parse tpl:%v %v ", notifyTpl, err)
		}

		tpls[notifyTpl.Channel] = tpl
	}
	return tpls, nil
}

// get notify by id
func NotifyTplGet(c *ctx.Context, id int64) (*NotifyTpl, error) {
	var tpl NotifyTpl
	err := DB(c).Where("id=?", id).First(&tpl).Error
	return &tpl, err
}

func InitNotifyConfig(c *ctx.Context, tplDir string) {
	if !c.IsCenter {
		return
	}

	// init notify channel
	cval, err := ConfigsGet(c, NOTIFYCHANNEL)
	if err != nil {
		logger.Errorf("failed to get notify contact config: %v", err)
		return
	}

	if cval == "" {
		var notifyChannels []NotifyChannel
		for _, channel := range DefaultChannels {
			notifyChannels = append(notifyChannels, NotifyChannel{Ident: channel, Name: channel, BuiltIn: true})
		}

		data, _ := json.Marshal(notifyChannels)
		err = ConfigsSet(c, NOTIFYCHANNEL, string(data))
		if err != nil {
			logger.Errorf("failed to set notify contact config: %v", err)
			return
		}
	} else {
		var channels []NotifyChannel
		err = json.Unmarshal([]byte(cval), &channels)
		if err != nil {
			logger.Errorf("failed to unmarshal notify channel config: %v", err)
			return
		}
		channelMap := make(map[string]bool)
		for _, channel := range channels {
			channelMap[channel.Ident] = true
		}

		var newChannels []NotifyChannel
		for _, channel := range DefaultChannels {
			if _, ok := channelMap[channel]; !ok {
				newChannels = append(newChannels, NotifyChannel{Ident: channel, Name: channel, BuiltIn: true})
			}
		}
		if len(newChannels) > 0 {
			channels = append(channels, newChannels...)
			data, _ := json.Marshal(channels)
			err = ConfigsSet(c, NOTIFYCHANNEL, string(data))
			if err != nil {
				logger.Errorf("failed to set notify contact config: %v", err)
				return
			}
		}
	}

	// init notify contact
	cval, err = ConfigsGet(c, NOTIFYCONTACT)
	if err != nil {
		logger.Errorf("failed to get notify contact config: %v", err)
		return
	}

	if cval == "" {
		var notifyContacts []NotifyContact
		for _, contact := range DefaultContacts {
			notifyContacts = append(notifyContacts, NotifyContact{Ident: contact, Name: contact, BuiltIn: true})
		}

		data, _ := json.Marshal(notifyContacts)
		err = ConfigsSet(c, NOTIFYCONTACT, string(data))
		if err != nil {
			logger.Errorf("failed to set notify contact config: %v", err)
			return
		}
	} else {
		var contacts []NotifyContact
		if err = json.Unmarshal([]byte(cval), &contacts); err != nil {
			logger.Errorf("failed to unmarshal notify channel config: %v", err)
			return
		}
		contactMap := make(map[string]struct{})
		for _, contact := range contacts {
			contactMap[contact.Ident] = struct{}{}
		}

		var newContacts []NotifyContact
		for _, contact := range DefaultContacts {
			if _, ok := contactMap[contact]; !ok {
				newContacts = append(newContacts, NotifyContact{Ident: contact, Name: contact, BuiltIn: true})
			}
		}
		if len(newContacts) > 0 {
			contacts = append(contacts, newContacts...)
			data, err := json.Marshal(contacts)
			if err != nil {
				logger.Errorf("failed to marshal contacts: %v", err)
				return
			}
			if err = ConfigsSet(c, NOTIFYCONTACT, string(data)); err != nil {
				logger.Errorf("failed to set notify contact config: %v", err)
				return
			}
		}
	}

	// init notify tpl
	tplMap := getNotifyTpl(tplDir)
	for channel, content := range tplMap {
		notifyTpl := NotifyTpl{
			Name:    channel,
			Channel: channel,
			Content: content,
		}

		err := notifyTpl.CreateIfNotExists(c, channel)
		if err != nil {
			logger.Warningf("failed to create notify tpls %v", err)
		}
	}
}

func getNotifyTpl(tplDir string) map[string]string {
	filenames, err := file.FilesUnder(tplDir)
	if err != nil {
		logger.Errorf("failed to get tpl files under %s", tplDir)
		return nil
	}

	tplMap := make(map[string]string)
	if len(filenames) != 0 {
		for i := 0; i < len(filenames); i++ {
			if strings.HasSuffix(filenames[i], ".tpl") {
				name := strings.TrimSuffix(filenames[i], ".tpl")
				tplpath := path.Join(tplDir, filenames[i])
				content, err := file.ToString(tplpath)
				if err != nil {
					logger.Errorf("failed to read tpl file: %s", filenames[i])
					continue
				}
				tplMap[name] = content
			}
		}
		return tplMap
	}

	logger.Debugf("no tpl files under %s, use default tpl", tplDir)
	return TplMap
}

var TplMap = map[string]string{
	Dingtalk: `#### {{if .IsRecovered}}<font color="#008800">💚{{.RuleName}}</font>{{else}}<font color="#FF0000">💔{{.RuleName}}</font>{{end}}

---
{{$time_duration := sub now.Unix .FirstTriggerTime }}{{if .IsRecovered}}{{$time_duration = sub .LastEvalTime .FirstTriggerTime }}{{end}}
- **告警级别**: {{.Severity}}级
{{- if .RuleNote}}
- **规则备注**: {{.RuleNote}}
{{- end}}
{{- if not .IsRecovered}}
- **当次触发时值**: {{.TriggerValue}}
- **当次触发时间**: {{timeformat .TriggerTime}}
- **告警持续时长**: {{humanizeDurationInterface $time_duration}}
{{- else}}
{{- if .AnnotationsJSON.recovery_value}}
- **恢复时值**: {{formatDecimal .AnnotationsJSON.recovery_value 4}}
{{- end}}
- **恢复时间**: {{timeformat .LastEvalTime}}
- **告警持续时长**: {{humanizeDurationInterface $time_duration}}
{{- end}}
- **告警事件标签**:
{{- range $key, $val := .TagsMap}}
{{- if ne $key "rulename" }}
	- {{$key}}: {{$val}}
{{- end}}
{{- end}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
[事件详情]({{$domain}}/alert-his-events/{{.Id}})|[屏蔽1小时]({{$domain}}/alert-mutes/add?busiGroup={{.GroupId}}&cate={{.Cate}}&datasource_ids={{.DatasourceId}}&prod={{.RuleProd}}{{range $key, $value := .TagsMap}}&tags={{$key}}%3D{{$value}}{{end}})|[查看曲线]({{$domain}}/metric/explorer?data_source_id={{.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{.PromQl}})`,
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
				<h3 class="title">{{.RuleName}}</h3>
				<p class="sub-desc"></p>
			</header>
	
			<hr>
	
			<div class="body">
				<table cellspacing="0" cellpadding="0" border="0">
					<tbody>
					{{if .IsRecovered}}
					<tr class="succ">
						<th>级别状态：</th>
						<td>S{{.Severity}} Recovered</td>
					</tr>
					{{else}}
					<tr class="fail">
						<th>级别状态：</th>
						<td>S{{.Severity}} Triggered</td>
					</tr>
					{{end}}
	
					<tr>
						<th>策略备注：</th>
						<td>{{.RuleNote}}</td>
					</tr>
					<tr>
						<th>设备备注：</th>
						<td>{{.TargetNote}}</td>
					</tr>
					{{if not .IsRecovered}}
					<tr>
						<th>触发时值：</th>
						<td>{{.TriggerValue}}</td>
					</tr>
					{{end}}
	
					{{if .TargetIdent}}
					<tr>
						<th>监控对象：</th>
						<td>{{.TargetIdent}}</td>
					</tr>
					{{end}}
					<tr>
						<th>监控指标：</th>
						<td>{{.TagsJSON}}</td>
					</tr>
	
					{{if .IsRecovered}}
					<tr>
						<th>恢复时间：</th>
						<td>{{timeformat .LastEvalTime}}</td>
					</tr>
					{{else}}
					<tr>
						<th>触发时间：</th>
						<td>
							{{timeformat .TriggerTime}}
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
	Feishu: `级别状态: S{{.Severity}} {{if .IsRecovered}}Recovered{{else}}Triggered{{end}}   
规则名称: {{.RuleName}}{{if .RuleNote}}   
规则备注: {{.RuleNote}}{{end}}   
监控指标: {{.TagsJSON}}
{{if .IsRecovered}}恢复时间：{{timeformat .LastEvalTime}}{{else}}触发时间: {{timeformat .TriggerTime}}
触发时值: {{.TriggerValue}}{{end}}
发送时间: {{timestamp}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
事件详情: {{$domain}}/alert-his-events/{{.Id}}
屏蔽1小时: {{$domain}}/alert-mutes/add?busiGroup={{.GroupId}}&cate={{.Cate}}&datasource_ids={{.DatasourceId}}&prod={{.RuleProd}}{{range $key, $value := .TagsMap}}&tags={{$key}}%3D{{$value}}{{end}}`,
	FeishuCard: `{{ if .IsRecovered }}
{{- if ne .Cate "host"}}
**告警集群:** {{.Cluster}}{{end}}   
**级别状态:** S{{.Severity}} Recovered   
**告警名称:** {{.RuleName}}   
**恢复时间:** {{timeformat .LastEvalTime}}   
**告警描述:** **服务已恢复**   
{{- else }}
{{- if ne .Cate "host"}}   
**告警集群:** {{.Cluster}}{{end}}   
**级别状态:** S{{.Severity}} Triggered   
**告警名称:** {{.RuleName}}   
**触发时间:** {{timeformat .TriggerTime}}   
**发送时间:** {{timestamp}}   
**触发时值:** {{.TriggerValue}}   
{{if .RuleNote }}**告警描述:** **{{.RuleNote}}**{{end}}   
{{- end -}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
[事件详情]({{$domain}}/alert-his-events/{{.Id}})|[屏蔽1小时]({{$domain}}/alert-mutes/add?busiGroup={{.GroupId}}&cate={{.Cate}}&datasource_ids={{.DatasourceId}}&prod={{.RuleProd}}{{range $key, $value := .TagsMap}}&tags={{$key}}%3D{{$value}}{{end}})|[查看曲线]({{$domain}}/metric/explorer?data_source_id={{.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{.PromQl}})`,
	EmailSubject: `{{if .IsRecovered}}Recovered{{else}}Triggered{{end}}: {{.RuleName}} {{.TagsJSON}}`,
	Mm: `级别状态: S{{.Severity}} {{if .IsRecovered}}Recovered{{else}}Triggered{{end}}   
规则名称: {{.RuleName}}{{if .RuleNote}}   
规则备注: {{.RuleNote}}{{end}}   
监控指标: {{.TagsJSON}}   
{{if .IsRecovered}}恢复时间：{{timeformat .LastEvalTime}}{{else}}触发时间: {{timeformat .TriggerTime}}   
触发时值: {{.TriggerValue}}{{end}}   
发送时间: {{timestamp}}`,
	Telegram: `**级别状态**: {{if .IsRecovered}}<font color="info">S{{.Severity}} Recovered</font>{{else}}<font color="warning">S{{.Severity}} Triggered</font>{{end}}   
**规则标题**: {{.RuleName}}{{if .RuleNote}}   
**规则备注**: {{.RuleNote}}{{end}}{{if .TargetIdent}}   
**监控对象**: {{.TargetIdent}}{{end}}   
**监控指标**: {{.TagsJSON}}{{if not .IsRecovered}}   
**触发时值**: {{.TriggerValue}}{{end}}   
{{if .IsRecovered}}**恢复时间**: {{timeformat .LastEvalTime}}{{else}}**首次触发时间**: {{timeformat .FirstTriggerTime}}{{end}}   
{{$time_duration := sub now.Unix .FirstTriggerTime }}{{if .IsRecovered}}{{$time_duration = sub .LastEvalTime .FirstTriggerTime }}{{end}}**距离首次告警**: {{humanizeDurationInterface $time_duration}}
**发送时间**: {{timestamp}}`,
	Wecom: `**级别状态**: {{if .IsRecovered}}<font color="info">S{{.Severity}} Recovered</font>{{else}}<font color="warning">S{{.Severity}} Triggered</font>{{end}}   
**规则标题**: {{.RuleName}}{{if .RuleNote}}   
**规则备注**: {{.RuleNote}}{{end}}{{if .TargetIdent}}   
**监控对象**: {{.TargetIdent}}{{end}}   
**监控指标**: {{.TagsJSON}}{{if not .IsRecovered}}   
**触发时值**: {{.TriggerValue}}{{end}}   
{{if .IsRecovered}}**恢复时间**: {{timeformat .LastEvalTime}}{{else}}**首次触发时间**: {{timeformat .FirstTriggerTime}}{{end}}   
{{$time_duration := sub now.Unix .FirstTriggerTime }}{{if .IsRecovered}}{{$time_duration = sub .LastEvalTime .FirstTriggerTime }}{{end}}**距离首次告警**: {{humanizeDurationInterface $time_duration}}
**发送时间**: {{timestamp}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
[事件详情]({{$domain}}/alert-his-events/{{.Id}})|[屏蔽1小时]({{$domain}}/alert-mutes/add?busiGroup={{.GroupId}}&cate={{.Cate}}&datasource_ids={{.DatasourceId}}&prod={{.RuleProd}}{{range $key, $value := .TagsMap}}&tags={{$key}}%3D{{$value}}{{end}})|[查看曲线]({{$domain}}/metric/explorer?data_source_id={{.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{.PromQl}})`,
	Lark: `级别状态: S{{.Severity}} {{if .IsRecovered}}Recovered{{else}}Triggered{{end}}   
规则名称: {{.RuleName}}{{if .RuleNote}}   
规则备注: {{.RuleNote}}{{end}}   
监控指标: {{.TagsJSON}}
{{if .IsRecovered}}恢复时间：{{timeformat .LastEvalTime}}{{else}}触发时间: {{timeformat .TriggerTime}}
触发时值: {{.TriggerValue}}{{end}}
发送时间: {{timestamp}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
事件详情: {{$domain}}/alert-his-events/{{.Id}}
屏蔽1小时: {{$domain}}/alert-mutes/add?busiGroup={{.GroupId}}&cate={{.Cate}}&datasource_ids={{.DatasourceId}}&prod={{.RuleProd}}{{range $key, $value := .TagsMap}}&tags={{$key}}%3D{{$value}}{{end}}`,
	LarkCard: `{{ if .IsRecovered }}
{{- if ne .Cate "host"}}
**告警集群:** {{.Cluster}}{{end}}   
**级别状态:** S{{.Severity}} Recovered   
**告警名称:** {{.RuleName}}   
**恢复时间:** {{timeformat .LastEvalTime}}   
{{$time_duration := sub now.Unix .FirstTriggerTime }}{{if .IsRecovered}}{{$time_duration = sub .LastEvalTime .FirstTriggerTime }}{{end}}**持续时长**: {{humanizeDurationInterface $time_duration}}   
**告警描述:** **服务已恢复**   
{{- else }}
{{- if ne .Cate "host"}}   
**告警集群:** {{.Cluster}}{{end}}   
**级别状态:** S{{.Severity}} Triggered   
**告警名称:** {{.RuleName}}   
**触发时间:** {{timeformat .TriggerTime}}   
**发送时间:** {{timestamp}}   
**触发时值:** {{.TriggerValue}}
{{$time_duration := sub now.Unix .FirstTriggerTime }}{{if .IsRecovered}}{{$time_duration = sub .LastEvalTime .FirstTriggerTime }}{{end}}**持续时长**: {{humanizeDurationInterface $time_duration}}   
{{if .RuleNote }}**告警描述:** **{{.RuleNote}}**{{end}}   
{{- end -}}
{{$domain := "http://请联系管理员修改通知模板将域名替换为实际的域名" }}   
[事件详情]({{$domain}}/alert-his-events/{{.Id}})|[屏蔽1小时]({{$domain}}/alert-mutes/add?busiGroup={{.GroupId}}&cate={{.Cate}}&datasource_ids={{.DatasourceId}}&prod={{.RuleProd}}{{range $key, $value := .TagsMap}}&tags={{$key}}%3D{{$value}}{{end}})|[查看曲线]({{$domain}}/metric/explorer?data_source_id={{.DatasourceId}}&data_source_name=prometheus&mode=graph&prom_ql={{.PromQl}})`,
}
