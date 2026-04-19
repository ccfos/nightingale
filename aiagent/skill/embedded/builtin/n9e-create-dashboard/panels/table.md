# table — 表格（经典版）

传统表格，支持多种数据转换模式。

## custom 配置

```json
{
  "version": "3.4.0",
  "showHeader": true,
  "colorMode": "value",
  "calc": "lastNotNull",
  "displayMode": "seriesToRows",
  "columns": [],
  "aggrDimension": [],
  "tableLayout": "auto",
  "nowrap": true,
  "sortColumn": null,
  "sortOrder": null,
  "pageLimit": 500,
  "linkMode": "appendLinkColumn",
  "links": []
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `showHeader` | boolean | | 显示表头 |
| `colorMode` | string | `"value"` `"background"` | 颜色模式 |
| `calc` | string | 见聚合函数 + `"origin"` | 聚合方式(origin为原始数据) |
| `displayMode` | string | `"seriesToRows"` `"labelsOfSeriesToRows"` `"labelValuesToRows"` | 数据转换模式 |
| `columns` | string[] | | labelsOfSeriesToRows模式下的列选择 |
| `aggrDimension` | string[] | | labelValuesToRows模式下的聚合维度 |
| `tableLayout` | string | `"auto"` `"fixed"` | 表格布局 |
| `nowrap` | boolean | | 文本不换行 |
| `sortColumn` | string/null | | 默认排序列 |
| `sortOrder` | string/null | `"ascend"` `"descend"` | 排序方向 |
| `pageLimit` | number | 1-500 | 每页行数 |
| `linkMode` | string | `"appendLinkColumn"` `"cellLink"` | 链接模式 |
| `links` | array | | 链接配置 `[{title, url, targetBlank}]` |

### displayMode 说明

- `seriesToRows`：每条时序为一行，calc 聚合为一个值
- `labelsOfSeriesToRows`：选择特定标签列展示
- `labelValuesToRows`：按标签维度聚合展示

## 推荐布局

`h=10, w=12`（一行放 2 个）

## 适用场景

主机资源概览表、告警列表、Top N 详情表
