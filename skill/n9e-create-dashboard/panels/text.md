# text — 文本/Markdown

静态文本面板，支持 Markdown 渲染和变量替换。

## custom 配置

```json
{
  "version": "3.4.0",
  "textSize": 12,
  "textColor": "#000000",
  "textDarkColor": "#FFFFFF",
  "bgColor": "rgba(0, 0, 0, 0)",
  "justifyContent": "center",
  "alignItems": "center",
  "content": ""
}
```

| 字段 | 类型 | 可选值 | 说明 |
|------|------|--------|------|
| `textSize` | number | | 字体大小(px) |
| `textColor` | string | hex颜色 | 亮色主题文字颜色 |
| `textDarkColor` | string | hex颜色 | 暗色主题文字颜色 |
| `bgColor` | string | css颜色 | 背景色 |
| `justifyContent` | string | `"unset"` `"flexStart"` `"center"` `"flexEnd"` | 水平对齐 |
| `alignItems` | string | `"unset"` `"flexStart"` `"center"` `"flexEnd"` | 垂直对齐 |
| `content` | string | | Markdown 内容(支持 `$variable_name` 变量替换) |

## 推荐布局

按内容量调节，常用 `h=4, w=6`

## 适用场景

仪表盘说明文字、分区标注、展示链接
