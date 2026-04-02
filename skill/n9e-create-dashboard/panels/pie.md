# pie — 饼图

展示各部分占比，支持甜甜圈样式。

## custom 配置

```json
{
  "version": "3.4.0",
  "textMode": "valueAndName",
  "colorMode": "value",
  "calc": "lastNotNull",
  "valueField": "Value",
  "countOfValueField": true,
  "legengPosition": "right",
  "max": null,
  "donut": false,
  "labelWithName": false,
  "labelWithValue": false,
  "detailName": "详情",
  "detailUrl": ""
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `textMode` | string | `"valueAndName"` `"name"` `"value"` | 图例文本 |
| `colorMode` | string | `"value"` `"background"` | 颜色模式 |
| `calc` | string | 见 units.md 聚合函数 | 聚合计算方式 |
| `valueField` | string | | 取值字段 |
| `countOfValueField` | boolean | | 是否统计值字段 |
| `legengPosition` | string | `"right"` `"left"` `"top"` `"bottom"` `"hidden"` | 图例位置 |
| `max` | number/null | | 最大分片数 |
| `donut` | boolean | | 甜甜圈样式 |
| `labelWithName` | boolean | | 标签显示名称 |
| `labelWithValue` | boolean | | 标签显示数值 |
| `detailName` | string | | 详情链接名称 |
| `detailUrl` | string | | 详情跳转URL |

**detailUrl 模板变量：** `${__field.name}` `${__field.value}` `${__field.labels.xxx}`

## 推荐布局

`h=6, w=6`（一行放 4 个）

## 适用场景

流量来源占比、磁盘空间分布、告警级别分布
