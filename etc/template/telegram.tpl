**级别状态**: {{if .IsRecovered}}<font color="info">S{{.Severity}} Recovered</font>{{else}}<font color="warning">S{{.Severity}} Triggered</font>{{end}}
**规则标题**: {{.RuleName}}{{if .RuleNote}}
**规则备注**: {{.RuleNote}}{{end}}{{if .TargetIdent}}
**监控对象**: {{.TargetIdent}}{{end}}
**监控指标**: {{.TagsJSON}}{{if not .IsRecovered}}
**触发时值**: {{.TriggerValue}}{{end}}
{{if .IsRecovered}}**恢复时间**: {{timeformat .LastEvalTime}}{{else}}**首次触发时间**: {{timeformat .FirstTriggerTime}}{{end}}
{{$time_duration := sub now.Unix .FirstTriggerTime }}{{if .IsRecovered}}{{$time_duration = sub .LastEvalTime .FirstTriggerTime }}{{end}}**持续时长**: {{humanizeDurationInterface $time_duration}}
**发送时间**: {{timestamp}}