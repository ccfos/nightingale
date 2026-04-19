# tableNG — 新表格

基于 ag-grid 的增强版表格，支持筛选、列宽缓存。

## custom 配置

```json
{
  "version": "3.4.0",
  "showHeader": true,
  "filterable": false,
  "sortColumn": null,
  "sortOrder": null,
  "cellOptions": {
    "type": "none",
    "mode": "basic",
    "valueDisplayMode": "text",
    "wrapText": false
  }
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `showHeader` | boolean | | 显示表头 |
| `filterable` | boolean | | 启用列筛选 |
| `sortColumn` | string/null | | 默认排序列 |
| `sortOrder` | string/null | `"ascend"` `"descend"` | 排序方向 |
| `cellOptions.type` | string | `"none"` `"color-text"` `"color-background"` `"gauge"` | 单元格样式 |
| `cellOptions.mode` | string | `"basic"` `"gradient"` `"lcd"` | gauge模式下的子样式 |
| `cellOptions.valueDisplayMode` | string | `"text"` `"color"` `"hidden"` | gauge模式下值显示 |
| `cellOptions.wrapText` | boolean | | 文本换行 |

## 推荐布局

`h=10, w=12`（一行放 2 个）

## 适用场景

大数据量表格、需要列筛选排序的场景
