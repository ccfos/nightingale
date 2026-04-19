# heatmap — 热力图

二维热力图，X/Y 轴分别映射字段，颜色表示值的大小。

## custom 配置

```json
{
  "version": "3.4.0",
  "calc": "lastNotNull",
  "valueField": "Value",
  "xAxisField": null,
  "yAxisField": null,
  "scheme": "Blues"
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `calc` | string | 见 units.md 聚合函数 | 聚合计算方式 |
| `valueField` | string | | 取值字段(颜色值) |
| `xAxisField` | string | | X轴映射字段(必填) |
| `yAxisField` | string | | Y轴映射字段(必填) |
| `scheme` | string | 见下方色系 | 配色方案 |

## 可用色系

`Blues` `Greens` `Greys` `Oranges` `Purples` `Reds` `BuGn` `BuPu` `GnBu` `OrRd` `PuBu` `PuBuGn` `PuRd` `RdBu` `RdGy` `RdPu` `RdYlBu` `RdYlGn` `YlGnBu` `YlGn` `YlOrBr` `YlOrRd`

## 推荐布局

`h=8, w=12`

## 适用场景

请求延迟分布（时间 x 桶）、资源使用热力矩阵
