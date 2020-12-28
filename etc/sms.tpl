{{if .IsUpgrade}}注意，告警已触发升级！{{end}}
级别状态：{{.Status}}
策略名称：{{.Sname}}
告警设备：{{.Endpoint}}
设备名称：{{.Name}}
设备备注：{{.Note}}
挂载节点：
{{range .Bindings}}{{.}}
{{end}}监控指标：{{.Metric}}
指标标签：{{.Tags}}
当前值：{{.Value}}
报警说明：{{.Info | unescaped}}
触发时间：{{.Etime}}
报警详情：{{.Elink | urlconvert}}
报警策略：{{.Slink | urlconvert}}
{{if .HasClaim}}认领报警：{{.Clink | urlconvert}}{{end}}