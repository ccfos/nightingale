# AI 助手会话接口

本文说明 Nightingale AI 助手会话相关接口。

所有成功响应均使用 Nightingale 标准 JSON 信封：

```json
{
  "dat": {},
  "err": "",
  "request_id": "..."
}
```

## 重命名会话

```text
POST /api/n9e/assistant/chat/rename
```

需要登录。调用者只能重命名自己创建的会话。

### 请求体

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `chat_id` | string | 是 | 要重命名的会话 ID |
| `title` | string | 是 | 新标题；不能是空字符串或仅由空白字符组成 |

```json
{
  "chat_id": "ad4075b8-ea4e-43d5-954c-71bb951874ca",
  "title": "生产环境告警排查"
}
```

### 响应

成功时，`dat` 是更新后的会话对象：

```json
{
  "dat": {
    "chat_id": "ad4075b8-ea4e-43d5-954c-71bb951874ca",
    "title": "生产环境告警排查",
    "last_update": 1730000000,
    "page_from": {
      "page": "dashboards"
    },
    "user_id": 1,
    "is_new": false,
    "is_renamed": true
  },
  "err": "",
  "request_id": "..."
}
```

`is_renamed` 表示标题已被用户手动设置。若在发送首条消息前改名，服务端不会再使用首条消息内容自动覆盖该标题。改名本身不会修改 `last_update`，因此不会影响历史会话列表的排序。

### 错误

| HTTP 状态码 | 场景 |
| --- | --- |
| 400 | `chat_id` 缺失，或 `title` 为空/仅空白字符 |
| 200 | 会话不存在或不属于当前用户；按 n9e 通用错误格式在 `err` 中返回错误信息 |
