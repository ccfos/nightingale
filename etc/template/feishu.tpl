级别状态: S{{.Severity}} {{if .IsRecovered}}Recovered{{else}}Triggered{{end}}
规则名称: {{.RuleName}}{{if .RuleNote}}
规则备注: {{.RuleNote}}{{end}}
监控指标: {{.TagsJSON}}
触发时间: {{timeformat .TriggerTime}}
触发时值: {{.TriggerValue}}