#### {{if .IsRecovered}}<font color="#008800">S{{.Severity}} - Recovered - {{.RuleName}}</font>{{else}}<font color="#FF0000">S{{.Severity}} - Triggered - {{.RuleName}}</font>{{end}}

---

- **规则标题**: {{.RuleName}}{{if .RuleNote}}
- **规则备注**: {{.RuleNote}}{{end}}
- **监控指标**: {{.TagsJSON}}
- {{if .IsRecovered}}**恢复时间**：{{timeformat .LastEvalTime}}{{else}}**触发时间**: {{timeformat .TriggerTime}}
- **触发时值**: {{.TriggerValue}}{{end}}
- **发送时间**: {{timestamp}}

