{{ if .IsRecovered }}
**告警集群:** {{.Cluster}}
**级别状态:** S{{.Severity}} Recovered
**告警名称:** {{.RuleName}}
**恢复时间:** {{timeformat .LastEvalTime}}
**告警描述:** **服务已恢复**
{{- else }}
**告警集群:** {{.Cluster}}
**级别状态:** S{{.Severity}} Triggered
**告警名称:** {{.RuleName}}
**触发时间:** {{timeformat .TriggerTime}}
**发送时间:** {{timestamp}}
**触发时值:** {{.TriggerValue}}
{{if .RuleNote }}**告警描述:** **{{.RuleNote}}**{{end}}
{{- end -}}