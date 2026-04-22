# gauge — 仪表盘

圆弧形仪表盘，直观显示当前值在阈值区间中的位置。

## custom 配置

```json
{
  "version": "3.4.0",
  "textMode": "valueAndName",
  "calc": "lastNotNull",
  "valueField": "Value"
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `textMode` | string | `"valueAndName"` `"value"` | 显示内容 |
| `calc` | string | 见 units.md 聚合函数 | 聚合计算方式 |
| `valueField` | string | | 取值字段 |

## 默认阈值

gauge 面板的 options.thresholds 默认带三色阈值：

```json
{
  "thresholds": {
    "steps": [
      { "color": "#3FC453", "value": null, "type": "base" },
      { "color": "#FF9919", "value": 60 },
      { "color": "#FF656B", "value": 80 }
    ]
  }
}
```

## 推荐布局

`h=6, w=6`（一行放 4 个）

## 适用场景

SLA 达标率、服务可用性、单一关键指标的当前状态
