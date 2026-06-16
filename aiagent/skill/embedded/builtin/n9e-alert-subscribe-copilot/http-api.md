# 订阅规则 HTTP API（外部 A2A agent / 给用户出 curl 指令时用）

> 站内 AI 助手**不要**走这些接口（用内置 FC 工具）。认证：`Authorization: Bearer <token>`。路由定义见 `center/router/router.go`。

| 操作 | 方法 | 路径 | 注意 |
|---|---|---|---|
| 列表（跨业务组） | `GET` | `/api/n9e/busi-groups/alert-subscribes` | 当前用户可见业务组下的订阅 |
| 列表（单业务组） | `GET` | `/api/n9e/busi-group/:id/alert-subscribes` | |
| 详情 | `GET` | `/api/n9e/alert-subscribe/:sid` | |
| 创建 | `POST` | `/api/n9e/busi-group/:id/alert-subscribes` | Body 是**单个** AlertSubscribe JSON 对象；group_id 取 URL |
| 更新 | `PUT` | `/api/n9e/busi-group/:id/alert-subscribes` | Body 是**数组** `[{...}]`（与创建相反）；按显式字段列表更新，仍建议先 GET 详情、改完整对象再 PUT |
| 删除 | `DELETE` | `/api/n9e/busi-group/:id/alert-subscribes` | Body: `{"ids":[1,2,3]}` |
| 试跑 | `POST` | `/api/n9e/alert-subscribe/alert-subscribes-tryrun` | Body: `{"event_id":<历史事件ID>,"config":{...订阅草稿...}}`；逐关校验匹配，新版还会走通知规则真实测试发送——**改完先 tryrun 再保存** |

权限：创建/更新/删除需业务组读写权限（bgrw）+ 对应 `/alert-subscribes/*` 菜单权限；列表只需只读。

直改 DB（最后手段）：表 `alert_subscribe`，`tags`/`busi_groups`/`webhooks`/`extra_config`/`notify_rule_ids` 等是 JSON/序列化字段；内存缓存 ~9s 自动重载，无需重启；改前先备份。
