# 通知规则 HTTP API（外部 A2A agent / 给用户出 curl 指令时用）

> 站内 AI 助手**不要**走这些接口（用内置 FC 工具）；本文件供外部 agent 程序化操作、或站内向用户解释"自己怎么用 curl 改"时引用。认证：`Authorization: Bearer <token>`。

## 路径 A：HTTP API（可程序化）

| 操作 | 方法 | 路径 | 注意 |
|---|---|---|---|
| 列表 | `GET` | `/api/n9e/notify-rules` | 仅返回当前用户授权团队下的规则 |
| 详情 | `GET` | `/api/n9e/notify-rule/<id>` | |
| 创建 | `POST` | `/api/n9e/notify-rules` | **Body 必须是数组**，即使只建 1 条：`[{...}]` |
| 更新 | `PUT` | `/api/n9e/notify-rule/<id>` | Body 是单对象，会**整体替换**——必须先 GET 再改再 PUT |
| 删除 | `POST` | `/api/n9e/notify-rules/del` | Body: `{"ids":[1,2,3]}` |
| 测试发送 | `POST` | `/api/n9e/notify-rule/test` | Body: `{"event_ids":[<history_event_id>], "notify_config":{...}}` |
| 拿自定义 webhook 参数 | `GET` | `/api/n9e/notify-rule-custom-params?notify_channel_id=<id>` | 用于复制其他规则的自定义参数 |
| 可用媒介列表 | `GET` | `/api/n9e/notify-channel-configs` | 拿 channel_id |
| 模板列表 | `GET` | `/api/n9e/message-templates?channel_id=<id>` | 拿 template_id |
| 事件标签 key | `GET` | `/api/n9e/event-tagkeys` | label_keys 可选 key |

**编辑动作的正确姿势**：

```text
1. GET /api/n9e/notify-rule/<id>      → 拿到完整 NotifyRule JSON
2. 在本地修改某个字段（如 notify_configs[1].severities = [1]）
3. PUT /api/n9e/notify-rule/<id>      → 整体提交回去
```

**不要试图 PATCH 局部更新**——PUT 走的是 `Update(...).Select("*")`（`models/notify_rule.go`），未传字段会被清空。

## 路径 B：UI

- 入口：`告警管理 → 通知规则`
- 适用：用户对 API 不熟、字段少、不熟悉 JSON 结构。

## 路径 C：直改 DB（最后手段）

- 表 `notify_rule`，`notify_configs` / `pipeline_configs` / `user_group_ids` 都是 JSON 字段。
- n9e 内存里有 `NotifyRuleCache`，改完会被缓存层在 ~9s 内自动重载，无需重启。
- 改前 `mysqldump -t notify_rule > backup.sql`。
