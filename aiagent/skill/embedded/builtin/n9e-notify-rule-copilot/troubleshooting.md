# 通知规则已知坑速查 + 测试验证

## 已知坑速查

| 现象 | 大概率原因 | 处理 |
|---|---|---|
| 测试发送 OK，真实告警没出来 | 接收人 `contact_info.<ContactKey>` 为空 → `sendtos` 空 → 静默不发 | 转 `n9e-alert-rule-troubleshoot` 流程 B；本 skill 只负责让用户检查渠道的 `ContactKey` 和用户的 contact_info |
| 群机器人渠道（钉钉/企微/飞书）一直没发出 | params 没填该渠道的自定义参数（`access_token`/`key`），或 token 填错——这类渠道发送只看 token，与接收人无关 | 对照 `list_notify_channels` 的 `custom_params` 补齐 params；token 哪里拿见 `reference.md` 速查表对应文档链接 |
| 告警规则保存了但通知记录一直为空 | 告警规则没关联到这条通知规则（`alert_rule.notify_rule_ids` 为空 / 仍走老版 `notify_groups`） | 告警规则列表 → 批量更新 → 关联通知规则 |
| 业务组改名后规则突然失配 | `attributes.group_name == "old-name"` 按名字硬绑 | 改用 `=~` 加正则，或同步改这条规则 |
| `attributes` 用 `in` 多个值无效 | value 写成逗号分隔 `"a,b,c"` | 改成**空格**：`"a b c"` |
| 多个 NotifyConfig 部分匹配失败时日志暴增 | 现版本日志级别问题 | 建议用户加一条"兜底 NotifyConfig"（recipes.md 模板 F） |
| 同一 webhook 被 N 条规则共用，单点宕机阻塞所有规则 | 队头阻塞 | 关键链路用独立 channel，提示用户拆 channel |
| 编辑保存后某个字段被清空 | PATCH 误用，或前端表单 normalizeValues 把空时间段过滤掉了 | 用 PUT 时**先 GET 再改再 PUT**，保留所有字段 |
| 跨午夜时段（如 22:00–02:00）不生效 | 引擎不跨天 | 拆 `22:00–23:59` + `00:00–02:00` 两段 |
| `week` 写反了（把 1 当周日） | 用了中国习惯 1=Mon 而非 ISO 0=Sun | 纠正：0=周日，1=周一 … 6=周六 |
| `is_recovered` 值类型踩坑 | 写成 `true`（bool）而不是 `"true"`（字符串） | TagFilter 的 value 是 string，必须用 `"true"` / `"false"` |
| 同一个 label key 想要 OR | 结构上不支持 | 改用 `attributes` 的 `in`，或拆多条 NotifyConfig |
| 名称带空格在 `in` 里失效 | 业务组名含空格被空格分隔吞掉 | 改用正则 `=~` 转义 |
| 用户没权限看到这条规则 | `user_group_ids` 没包含此用户所在团队 | 加上对应团队 ID |
| 创建报 `forbidden` | 当前用户不在 `user_group_ids` 任何一个团队 | 加上自己所在团队或让 admin 操作 |
| 按 ident 路由全部失配 | 事件标签里没有 `ident`（如 categraf 直写时序库丢失） | 让用户确认数据流，用 `GET /api/n9e/event-tagkeys` 看实际标签 |
| UI 找不到"切换新版"按钮 | 按钮位置历经多次变迁（beta14 隐藏 / 8.4.x 挪位置） | 升到 8.5.1+，去**告警规则列表的"批量更新"弹窗**找 |

## 测试与验证

### 用真实事件做 dry-run

`POST /api/n9e/notify-rule/test` 的语义比 UI 上"测试通知"按钮强：它会**用历史事件 ID + 你传入的 NotifyConfig** 做匹配并真实发送：

```
Body: {"event_ids":[<history_event_id>], "notify_config":{...}}

hisEvents = AlertHisEventGetByIds(event_ids)
for each event:
    dispatch.NotifyRuleMatchCheck(notify_config, event)   ← 真实匹配函数
SendNotifyChannelMessage(notify_config, events)           ← 真实发送
```

意味着：

- 拿一条**真实历史事件 ID**（从历史告警里挑），传入草稿 NotifyConfig，能立刻看到"会不会命中"和"发出去的样子"——比 UI 上凭空 mock event 准。
- 失败时返回的 error 能区分是匹配失败（哪一步）还是 channel 调用失败。
- 但**它不能验证 `notify_rule_ids` 关联**——这条规则有没有被告警规则挂上去是另一回事，要去 alert_rule 表 / 告警规则页面看。

### 编辑后的最小验证清单

每次修改一条通知规则，让用户走这 3 步确认：

1. `GET /api/n9e/notify-rule/<id>`（站内用 `get_notify_rule_detail`）看是不是改对了字段。
2. `POST /api/n9e/notify-rule/test` 用一条相关历史事件验证匹配 + 发送。
3. 真实告警出来后到 `历史告警 → 详情 → 通知记录` 看是否有这条规则的发送日志。
