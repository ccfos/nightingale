# 订阅不生效排查 + 行为语义

## "订阅了没收到"排查链

引擎匹配链 `alert/dispatch/dispatch.go:handleSub`，按以下顺序判断，**任何一关不过就跳过该订阅**。按序核对：

| # | 检查项 | 不过的常见原因 |
|---|---|---|
| 0 | 缓存同步 | 刚改完规则，内存缓存最多延迟 **9 秒** |
| 1 | `disabled` | 规则被禁用（`disabled=1`，缓存层直接过滤） |
| 2 | 数据源 | `datasource_ids` 非全部且事件的 `datasource_id` 不在列表里 |
| 3 | prod | `prod` 非空且与事件 RuleProd 不相等（注意是精确匹配） |
| 4 | cate | 填了 `"host"` 但事件不是 host 类（其他取值不会导致失配） |
| 5 | 标签 | `tags` 多条是 AND；`in` 的 value 写成逗号分隔会失配（必须空格） |
| 6 | 业务组名 | `busi_groups` 对事件**业务组名**匹配——业务组改名后按名硬绑的条件失联；正则没考虑全名 |
| 7 | 持续时长 | `for_duration` 大于事件已持续时长（`trigger_time - first_trigger_time`）——告警刚触发时一定不命中，要等持续够久的**下一次**通知周期 |
| 8 | 级别 | `severities` 不含事件级别 |
| 9 | 下游出口 | 以上全过、克隆事件已产生，但 `notify_rule_ids` 指向的**通知规则本身**没配对（渠道/接收人/适用属性不命中）→ 转 n9e-notify-rule-copilot 排查 |

还要确认源头：**原始告警事件得真的产生**——订阅不会无中生有；事件被屏蔽（评估阶段拦截）就根本到不了订阅这一步。

## 行为语义（回答用户疑问时引用）

| 行为 | 语义 |
|---|---|
| 订阅会替代原始通知吗 | **不会**。原始事件照常走自己的通知路径；订阅克隆一份额外转发。同一个人在两条链路都被配置就会收到两次——这是设计 |
| 会重复打回调（webhook）吗 | 不会。克隆事件的 callbacks 默认被**清空**（`models/alert_subscribe.go:ModifyEvent`），只有旧版显式 `redefine_webhooks=1` 才带上订阅自己的 webhooks |
| 订阅范围受 group_id 限制吗 | 不受。`group_id` 只是管理归属（权限）；订阅天然收所有业务组的事件，收窄靠 `busi_groups`/`rule_ids`/`tags` |
| `for_duration` 怎么生效 | 比较的是事件的 `trigger_time - first_trigger_time`。事件首次触发时差值为 0，必不命中；告警持续重复通知到第 N 次、差值超过阈值后，那次事件的克隆才被转发——所以"升级"依赖告警规则本身配了重复通知 |
| 新版能改写级别/渠道吗 | 不能。`notify_version=1` 时 Verify 会清空 redefine_* 字段；级别/渠道路由去通知规则层做 |
| 恢复事件也会被订阅吗 | 会走同一条匹配链；恢复通知是否发出取决于下游通知规则的配置（如 `is_recovered` 属性过滤） |
| 改动多久生效 | 缓存每 9 秒轮询，最多 9 秒，无需重启 |
| 怎么看一个事件是不是订阅转发的 | 克隆事件带 `sub_rule_id`（订阅规则 ID），通知记录/事件详情可见 |

## 其他坑

| 现象 | 原因 | 处理 |
|---|---|---|
| 创建报 `severities is required` | 新旧版都必填 | 至少填 `[1,2,3]` |
| 创建报 `no notify rules selected` | `notify_version=1` 但 `notify_rule_ids` 空 | 先 `list_notify_rules` 拿 ID，没有就先建通知规则 |
| 创建报 `new_channels is required` | 旧版指定了 `user_group_ids` 却没填 `new_channels` | 补 `new_channels`，或改用新版 |
| 保存后 redefine 字段全没了 | 新版 Verify 主动清空旧版字段 | 预期行为，不是丢数据 |
| 升级订阅一直不触发 | 告警规则没配重复通知（notify_repeat_step=0），事件不会再来第二次 | 让用户检查告警规则的重复通知间隔 |
| 全局订阅事件量爆炸 | rule_ids/tags/busi_groups 全空 = 复制所有事件 | 至少加一层过滤；或下游通知规则收窄 |
| 业务组改名后订阅失联 | `busi_groups` 按名匹配 | 改用 `=~` 正则，或改名时同步订阅 |

## 验证手段

- 站内：`get_alert_subscribe_detail` 核对字段；`get_notify_rule_detail` 核对下游出口。
- HTTP（给用户出指令）：`POST /api/n9e/alert-subscribe/alert-subscribes-tryrun` 用历史事件 ID + 订阅草稿试跑，能看到匹配在哪一步失败、新版还会真实走一次通知规则测试发送。见 `http-api.md`。
- 真实验证：触发一条匹配的告警，到 `历史告警 → 详情` 看是否出现带 `sub_rule_id` 的克隆事件及其通知记录。
