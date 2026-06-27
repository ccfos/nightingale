# 屏蔽规则 HTTP API（外部 A2A agent / 给用户出 curl 指令时用）

> 站内 AI 助手**不要**走这些接口（用内置 FC 工具）。认证：`Authorization: Bearer <token>`。路由定义见 `center/router/router.go`。

| 操作 | 方法 | 路径 | 注意 |
|---|---|---|---|
| 列表（跨业务组） | `GET` | `/api/n9e/busi-groups/alert-mutes` | 当前用户可见的屏蔽；支持 `query` 搜索、分页、`expired=1` 查已过期 |
| 列表（单业务组） | `GET` | `/api/n9e/busi-group/:id/alert-mutes` | |
| 详情 | `GET` | `/api/n9e/busi-group/:id/alert-mute/:amid` | |
| 创建 | `POST` | `/api/n9e/busi-group/:id/alert-mutes` | Body 是**单个** AlertMute JSON 对象 |
| 更新 | `PUT` | `/api/n9e/busi-group/:id/alert-mute/:amid` | Body 单对象，**整体替换**——先 GET 再改再 PUT |
| 批量改字段 | `PUT` | `/api/n9e/busi-group/:id/alert-mutes/fields` | Body: `{"ids":[...],"fields":{...}}`，适合批量禁用/延期 |
| 删除 | `DELETE` | `/api/n9e/busi-group/:id/alert-mutes` | Body: `{"ids":[1,2,3]}` |
| 预览命中事件 | `POST` | `/api/n9e/busi-group/:id/alert-mutes/preview` | 用屏蔽草稿预览会命中的活跃事件，**建大范围屏蔽前先跑这个** |
| 试跑 | `POST` | `/api/n9e/alert-mute-tryrun` | 用规则草稿做匹配试跑 |
| 批量清理过期屏蔽 | `DELETE` | `/api/n9e/alert-mutes` | **需 admin**；后台异步清理过期的固定时段屏蔽（周期屏蔽不清理） |

权限：创建/更新/删除需业务组读写权限（bgrw）+ 对应 `/alert-mutes/*` 菜单权限；列表只需只读。

直改 DB（最后手段）：表 `alert_mute`，`tags`/`periodic_mutes`/`severities`/`datasource_ids` 是 JSON/序列化字段；内存缓存 ~9s 自动重载，无需重启；改前先备份。
