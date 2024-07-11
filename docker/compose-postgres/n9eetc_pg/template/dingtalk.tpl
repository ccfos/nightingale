#### {{if .IsRecovered}}<font color="#008800">💚{{.RuleName}}</font>{{else}}<font color="#FF0000">💔{{.RuleName}}</font>{{end}}

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
  - `{{$key}}`: `{{$val}}`
{{- end}}
{{- end}}