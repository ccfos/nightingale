# row — 分组行

折叠/展开面板分组，不显示数据，仅做布局组织。

## 完整结构

```json
{
  "version": "3.4.0",
  "id": "row-id",
  "name": "分组名称",
  "type": "row",
  "description": "",
  "layout": { "h": 1, "w": 24, "x": 0, "y": 0, "i": "row-id" },
  "targets": [],
  "options": {},
  "custom": {},
  "overrides": [],
  "collapsed": false,
  "panels": []
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `collapsed` | boolean | 是否折叠 |
| `panels` | IPanel[] | 折叠时包含的子面板 |

## 布局

固定 `h=1, w=24`

## 适用场景

将面板按 CPU/内存/磁盘/网络 等类别分组
