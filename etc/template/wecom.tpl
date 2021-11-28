**级别状态**: {{if .IsRecovered}}<font color="info">S{{.Severity}} Recovered</font>{{else}}<font color="warning">S{{.Severity}} Triggered</font>{{end}}
**规则标题**: {{.RuleName}}{{if .RuleNote}}
**规则备注**: {{.RuleNote}}{{end}}
**监控指标**: {{.TagsJSON}}
**触发时间**: {{timeformat .TriggerTime}}
**触发时值**: {{.TriggerValue}}