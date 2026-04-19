# barchart — 柱状图

分类柱状图，X 轴为分类，Y 轴为数值，支持颜色分组。

## custom 配置

```json
{
  "version": "3.4.0",
  "calc": "lastNotNull",
  "valueField": "Value",
  "xAxisField": null,
  "yAxisField": null,
  "colorField": null,
  "barMaxWidth": null
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `calc` | string | 见 units.md 聚合函数 | 聚合计算方式 |
| `valueField` | string | | 取值字段 |
| `xAxisField` | string | | X轴分类字段(必填) |
| `yAxisField` | string | | Y轴数值字段(必填) |
| `colorField` | string/null | | 颜色分组字段 |
| `barMaxWidth` | number/null | | 柱子最大宽度 |

## 推荐布局

`h=8, w=12`

## 适用场景

各主机资源对比柱状图、按时间段统计的事件数
