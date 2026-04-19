# hexbin — 蜂窝图

六边形热力图，适合一次展示大量实例的状态概览。

## custom 配置

```json
{
  "version": "3.4.0",
  "textMode": "valueAndName",
  "calc": "lastNotNull",
  "valueField": "Value",
  "colorRange": "rgb(255,246,240),rgb(253,194,109),rgb(185,71,59)",
  "colorDomainAuto": true,
  "colorDomain": [],
  "reverseColorOrder": false,
  "fontBackground": false,
  "detailUrl": ""
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `textMode` | string | `"valueAndName"` `"name"` `"value"` | 六边形内文本 |
| `calc` | string | 见 units.md 聚合函数 | 聚合计算方式 |
| `valueField` | string | | 取值字段 |
| `colorRange` | string | 逗号分隔的颜色 | 色阶(也可用 `"thresholds"` 跟随阈值) |
| `colorDomainAuto` | boolean | | 自动计算色域范围 |
| `colorDomain` | number[] | | 手动色域 `[min, max]` |
| `reverseColorOrder` | boolean | | 反转颜色顺序 |
| `fontBackground` | boolean | | 文字背景 |
| `detailUrl` | string | | 点击跳转URL(支持模板变量) |

**detailUrl 模板变量：** `${__field.name}` `${__field.value}` `${__field.labels.xxx}`

## 推荐布局

`h=8, w=12`

## 适用场景

大规模主机健康状态一览、集群节点状态矩阵
