# 面板通用结构 (IPanel)

所有面板类型共享的基础结构。`options` 和 `custom` 的具体内容取决于面板类型。

## 完整结构

```json
{
  "version": "3.4.0",
  "id": "panel-unique-id",
  "name": "面板标题",
  "type": "timeseries",
  "description": "面板描述（鼠标悬浮显示）",
  "datasourceCate": "prometheus",
  "datasourceValue": 1,
  "layout": {
    "h": 8,
    "w": 12,
    "x": 0,
    "y": 0,
    "i": "panel-unique-id"
  },
  "targets": [],
  "options": {},
  "custom": {},
  "overrides": [],
  "transformationsNG": [],
  "repeat": null,
  "maxPerRow": null,
  "maxDataPoints": null,
  "queryOptionsTime": null
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `version` | string | 固定 `"3.4.0"` |
| `id` | string | 唯一标识，必须与 `layout.i` 一致 |
| `name` | string | 面板标题 |
| `type` | string | 面板类型，见下方列表 |
| `description` | string | 描述文字 |
| `datasourceCate` | string | 数据源类型：`"prometheus"` `"elasticsearch"` `"tdengine"` 等 |
| `datasourceValue` | number/string | 数据源 ID，或 `"${datasource}"` 引用变量 |
| `layout` | IGridPos | 位置和尺寸 |
| `targets` | ITarget[] | 查询配置，见 [target.md](target.md) |
| `options` | IOptions | 通用可视化选项，见 [common-options.md](common-options.md) |
| `custom` | object | 面板类型特有配置，见各面板文件 |
| `overrides` | IOverride[] | 字段级覆盖，见 [common-options.md](common-options.md) |
| `transformationsNG` | array | 数据变换，见 [common-options.md](common-options.md) |
| `repeat` | string/null | 按变量名重复面板 |
| `maxPerRow` | number/null | repeat 模式下每行最大面板数 |
| `maxDataPoints` | number/null | 覆盖全局最大数据点 |
| `queryOptionsTime` | object/null | 覆盖面板时间范围 `{"start":"now-1h","end":"now"}` |

## 所有面板类型

`"timeseries"` | `"stat"` | `"gauge"` | `"barGauge"` | `"pie"` | `"table"` | `"tableNG"` | `"hexbin"` | `"heatmap"` | `"barchart"` | `"text"` | `"iframe"` | `"row"`

## 布局系统 (IGridPos)

```json
{
  "h": 8,
  "w": 12,
  "x": 0,
  "y": 0,
  "i": "panel-unique-id"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `w` | number | 宽度（网格列数，总宽 24） |
| `h` | number | 高度（网格行数，每行约30px） |
| `x` | number | 左偏移（0-23） |
| `y` | number | 上偏移（逐行累加） |
| `i` | string | 必须与面板 `id` 一致 |

### 各面板推荐尺寸

| 类型 | w | h | 每行个数 | 备注 |
|------|---|---|----------|------|
| row | 24 | 1 | 1 | 固定全宽 |
| stat | 6 | 4 | 4 | 概览区域 |
| timeseries | 12 | 8 | 2 | 最常用 |
| gauge | 6 | 6 | 4 | |
| barGauge | 8 | 8 | 3 | |
| pie | 6 | 6 | 4 | |
| table / tableNG | 12 | 10 | 2 | 需要更多垂直空间 |
| hexbin | 12 | 8 | 2 | |
| heatmap | 12 | 8 | 2 | |
| barchart | 12 | 8 | 2 | |
| text | 6 | 4 | 按需 | |
| iframe | 24 | 12 | 1 | 通常全宽 |

### 布局计算规则

1. 从 `y=0` 开始，逐行往下排列
2. `row` 面板占一行：`y` 递增 1
3. 同一行内多个面板：相同 `y` 值，`x` 依次递增（0, 6, 12, 18 或 0, 12）
4. 下一行：`y` = 上一行 `y` + 上一行面板的 `h`

**示例布局：**
```
y=0:  [row: CPU]                          w=24, h=1
y=1:  [stat: CPU%]  [stat: Mem%]  [stat: Disk%]  [stat: Uptime]
       w=6,h=4       w=6,h=4       w=6,h=4        w=6,h=4
       x=0           x=6           x=12            x=18
y=5:  [row: 详情]                         w=24, h=1
y=6:  [timeseries: CPU趋势]    [timeseries: 系统负载]
       w=12,h=8, x=0             w=12,h=8, x=12
y=14: [row: 磁盘]                        w=24, h=1
y=15: [barGauge]    [timeseries]    [timeseries]
       w=8,h=8,x=0  w=8,h=8,x=8    w=8,h=8,x=16
```

## 面板重复 (repeat)

按变量值自动复制面板：

```json
{
  "repeat": "ident",
  "maxPerRow": 2
}
```

选择 3 个主机时，自动生成 3 个面板，每行 2 个。面板内的 `$ident` 自动绑定到各自的值。
