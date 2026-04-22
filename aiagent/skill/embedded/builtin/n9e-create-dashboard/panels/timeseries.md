# timeseries — 时序折线/柱状图

最常用的面板类型，用于展示指标随时间变化的趋势。

## custom 配置

```json
{
  "version": "3.4.0",
  "drawStyle": "lines",
  "lineInterpolation": "smooth",
  "lineWidth": 2,
  "fillOpacity": 0.01,
  "gradientMode": "none",
  "stack": "off",
  "showPoints": "none",
  "pointSize": 5,
  "spanNulls": false,
  "scaleDistribution": {
    "type": "linear",
    "log": 10
  }
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `drawStyle` | string | `"lines"` `"bars"` | 绘制样式 |
| `lineInterpolation` | string | `"linear"` `"smooth"` | 线条插值方式 |
| `lineWidth` | number | 0-10 | 线条宽度 |
| `fillOpacity` | number | 0-1 | 区域填充透明度 |
| `gradientMode` | string | `"none"` `"opacity"` | 渐变模式 |
| `stack` | string | `"off"` `"normal"` | 堆叠模式 |
| `showPoints` | string | `"none"` `"always"` | 数据点显示 |
| `pointSize` | number | 1-40 | 数据点大小 |
| `spanNulls` | boolean | | 是否连接空值 |
| `scaleDistribution.type` | string | `"linear"` `"log"` | Y轴刻度类型 |
| `scaleDistribution.log` | number | `10` `2` | 对数底数(log模式) |

## 推荐布局

`h=8, w=12`（一行放 2 个）

## 适用场景

CPU 趋势、内存趋势、网络流量、QPS、延迟分布等一切时序数据
