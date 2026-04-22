# 通用 options 配置 (IOptions)

所有面板类型共享以下 options 结构（按需使用）。

## standardOptions

```json
{
  "standardOptions": {
    "util": "percent",
    "decimals": 1,
    "min": 0,
    "max": 100,
    "dateFormat": "YYYY-MM-DD HH:mm:ss",
    "displayName": ""
  }
}
```

## thresholds

```json
{
  "thresholds": {
    "mode": "absolute",
    "steps": [
      { "color": "#3FC453", "value": null, "type": "base" },
      { "color": "#FF9919", "value": 60 },
      { "color": "#FF656B", "value": 80 }
    ]
  }
}
```

- `mode`：`"absolute"` 绝对值 | `"percentage"` 百分比
- `steps[0]` 必须有 `"type": "base"`，`value` 为 `null`

## thresholdsStyle (仅 timeseries)

```json
{
  "thresholdsStyle": {
    "mode": "dashed"
  }
}
```

可选值：`"off"` | `"line"` | `"dashed"` | `"line+area"` | `"dashed+area"`

## legend

```json
{
  "legend": {
    "displayMode": "table",
    "placement": "bottom",
    "calcs": ["lastNotNull", "max", "avg"],
    "behaviour": "showItem",
    "selectMode": "single",
    "heightInPercentage": 30,
    "widthInPercentage": 30,
    "columns": ["max", "min", "avg", "sum", "last"],
    "detailName": "详情",
    "detailUrl": ""
  }
}
```

- `displayMode`：`"list"` | `"table"` | `"hidden"`
- `placement`：`"bottom"` | `"right"`
- `calcs` 可选值：`"lastNotNull"` | `"last"` | `"max"` | `"min"` | `"avg"` | `"sum"` | `"variance"` | `"stdDev"`
- `behaviour`：`"showItem"` 点击显示 | `"hideItem"` 点击隐藏
- `selectMode`：`"single"` | `"multiple"`

## tooltip

```json
{
  "tooltip": {
    "mode": "all",
    "sort": "desc"
  }
}
```

- `mode`：`"single"` 仅当前系列 | `"all"` 全部系列
- `sort`：`"none"` | `"asc"` | `"desc"`

## valueMappings

```json
{
  "valueMappings": [
    {
      "type": "range",
      "match": { "from": 0, "to": 50 },
      "result": { "color": "#3FC453", "text": "正常" }
    },
    {
      "type": "special",
      "match": { "special": 0 },
      "result": { "color": "#FF656B", "text": "离线" }
    },
    {
      "type": "specialValue",
      "match": { "specialValue": "null" },
      "result": { "color": "#999", "text": "N/A" }
    },
    {
      "type": "textValue",
      "match": { "textValue": "error.*" },
      "result": { "color": "#FF656B", "text": "异常" }
    }
  ]
}
```

- `type`：`"range"` 范围 | `"special"` 固定数值 | `"specialValue"` 特殊值(`"null"`/`"empty"`) | `"textValue"` 文本/正则

## overrides

```json
{
  "overrides": [
    {
      "matcher": { "type": "byName", "value": "series_name" },
      "properties": {
        "standardOptions": { "util": "bytesIEC" },
        "valueMappings": [],
        "rightYAxisDisplay": "normal"
      }
    }
  ]
}
```

- `matcher.type`：`"byName"` 按系列名 | `"byFrameRefID"` 按查询 refId (A/B/C)
- `rightYAxisDisplay`：`"normal"` | `"hidden"`（仅 timeseries 支持双 Y 轴）

## transformationsNG

```json
{
  "transformationsNG": [
    { "id": "joinByField", "options": { "field": "Time" }, "disabled": false }
  ]
}
```

可用 id：`"joinByField"` | `"organize"` | `"merge"` | `"groupedAggregateTable"`
