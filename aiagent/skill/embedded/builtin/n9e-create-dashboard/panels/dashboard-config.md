# 仪表盘整体结构 (IDashboardConfig)

## 创建 API 请求体

```json
{
  "name": "仪表盘名称",
  "tags": "空格分隔的标签",
  "configs": "<configs 对象的 JSON.stringify 字符串>"
}
```

**重要：`configs` 字段必须是 JSON 字符串，不是嵌套对象。**

## configs 对象

```json
{
  "version": "3.4.0",
  "links": [],
  "graphTooltip": "sharedCrosshair",
  "graphZoom": "default",
  "var": [],
  "panels": []
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `version` | string | `"3.4.0"` | 固定值 |
| `links` | array | | 仪表盘级别外链 |
| `graphTooltip` | string | `"default"` `"sharedCrosshair"` `"sharedTooltip"` | 图表联动方式 |
| `graphZoom` | string | `"default"` `"updateTimeRange"` | 缩放行为 |
| `var` | IVariable[] | | 模板变量，见 [variable.md](variable.md) |
| `panels` | IPanel[] | | 面板数组，见 [panel-structure.md](panel-structure.md) |

### graphTooltip 说明

- `default`：各图表独立 tooltip
- `sharedCrosshair`：鼠标悬浮时所有时序图同步显示十字线（推荐）
- `sharedTooltip`：所有时序图同步显示完整 tooltip

### links 结构

```json
{
  "links": [
    {
      "title": "链接名称",
      "url": "https://example.com/path?var=$ident",
      "targetBlank": true
    }
  ]
}
```

支持 `$variable_name` 变量替换。

## 仪表盘级 iframe 模式

将整个仪表盘替换为一个 iframe：

```json
{
  "version": "3.4.0",
  "mode": "iframe",
  "iframe_url": "https://example.com/embed"
}
```
