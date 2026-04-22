# stat — 单值统计

展示单个聚合值，可带迷你趋势图和颜色阈值，适合放在仪表盘顶部做概览。

## custom 配置

```json
{
  "version": "3.4.0",
  "textMode": "valueAndName",
  "colorMode": "value",
  "calc": "lastNotNull",
  "valueField": "Value",
  "colSpan": 0,
  "orientation": "auto",
  "textSize": {
    "title": null,
    "value": null
  },
  "graphMode": "none"
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `textMode` | string | `"valueAndName"` `"value"` `"name"` | 显示内容 |
| `colorMode` | string | `"value"` `"background"` | 颜色模式：文字着色或背景着色 |
| `calc` | string | 见 units.md 聚合函数 | 聚合计算方式 |
| `valueField` | string | | 取值字段 |
| `colSpan` | number | 0=自动, 1-10 | 每行列数 |
| `orientation` | string | `"auto"` `"horizontal"` `"vertical"` | 标题/值排列方向(仅colSpan=0) |
| `textSize.title` | number/null | | 标题字号(null自动) |
| `textSize.value` | number/null | | 值字号(null自动) |
| `graphMode` | string | `"none"` `"area"` | 迷你趋势图 |

## 推荐布局

`h=4, w=6`（一行放 4 个）

## 适用场景

概览统计（CPU%、内存%、在线主机数、运行时间）
