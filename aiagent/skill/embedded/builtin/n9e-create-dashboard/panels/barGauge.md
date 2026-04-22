# barGauge — 柱状仪表

水平条形图，适合排序对比多个实例的同一指标值。

## custom 配置

```json
{
  "version": "3.4.0",
  "calc": "lastNotNull",
  "valueField": "Value",
  "baseColor": "#9470FF",
  "displayMode": "basic",
  "sortOrder": "desc",
  "otherPosition": "none",
  "valueMode": "color",
  "serieWidth": null,
  "nameField": null,
  "topn": null,
  "combine_other": false,
  "detailUrl": ""
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `calc` | string | 见 units.md 聚合函数 | 聚合计算方式 |
| `valueField` | string | | 取值字段 |
| `baseColor` | string | hex颜色 | 基础颜色（无阈值时） |
| `displayMode` | string | `"basic"` `"lcd"` | 显示模式：基础条或LCD段 |
| `sortOrder` | string | `"none"` `"asc"` `"desc"` | 排序方式 |
| `otherPosition` | string | `"none"` `"top"` `"bottom"` | 合并其他项的位置 |
| `valueMode` | string | `"color"` `"hidden"` | 数值颜色显示 |
| `serieWidth` | number/null | | 条形宽度百分比(null自动) |
| `nameField` | string/null | | 名称字段（覆盖默认） |
| `topn` | number/null | | 只显示前N项 |
| `combine_other` | boolean | | 其余合并为"其他" |
| `detailUrl` | string | | 详情跳转URL(支持模板变量) |

## 推荐布局

`h=8, w=8`（一行放 3 个）

## 适用场景

各磁盘使用率排名、各主机 CPU 排名、Top N 慢查询
