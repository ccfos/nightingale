# 复杂语义 → NotifyConfig 拆解模板 + 完整示例

把用户的自然语言路由意图映射到下面最贴近的模板，给出**完整 JSON 草稿**——不要让用户自己填字段名。`<xxx_id>` 占位符用 `list_notify_channels` / `list_teams` 查到的真实 ID 替换。

## 模板 A：分级走不同通道

> 「P1 告警打电话 + 钉钉，P2/P3 告警只发钉钉」

```json
{
  "name": "线上分级告警路由",
  "description": "P1: 电话 + 钉钉；P2/P3: 钉钉",
  "enable": true,
  "user_group_ids": [<oncall_team_id>],
  "notify_configs": [
    {
      "channel_id": <voice_channel_id>,
      "template_id": 0,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1],
      "time_ranges": []
    },
    {
      "channel_id": <dingtalk_channel_id>,
      "template_id": 0,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": []
    }
  ]
}
```

**关键决策**：钉钉那条 severities 写全 `[1,2,3]`（P1 也发，作为电话之外的"留痕"通道），不要写 `[2,3]` 漏掉 P1 的钉钉记录。

## 模板 B：工作时间 vs 非工作时间不同动作

> 「工作时间（周一到周五 9-18 点）发钉钉群；非工作时间打电话 + 钉钉」

```json
{
  "name": "值班动作分时段",
  "user_group_ids": [<oncall_team_id>],
  "notify_configs": [
    {
      "channel_id": <dingtalk_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": [{ "week": [1,2,3,4,5], "start": "09:00", "end": "18:00" }]
    },
    {
      "channel_id": <voice_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2],
      "time_ranges": [
        { "week": [1,2,3,4,5], "start": "18:00", "end": "23:59" },
        { "week": [1,2,3,4,5], "start": "00:00", "end": "09:00" },
        { "week": [0, 6],     "start": "00:00", "end": "00:00" }
      ]
    },
    {
      "channel_id": <dingtalk_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2],
      "time_ranges": [
        { "week": [1,2,3,4,5], "start": "18:00", "end": "23:59" },
        { "week": [1,2,3,4,5], "start": "00:00", "end": "09:00" },
        { "week": [0, 6],     "start": "00:00", "end": "00:00" }
      ]
    }
  ]
}
```

**关键决策**：周末 24h 走电话路径，所以是 `week=[0,6]` 全天；工作日跨午夜要拆 18:00–23:59 + 00:00–09:00 两段（不能写 18:00–09:00，引擎当天看）。

## 模板 C：恢复时不打电话

> 「告警时打电话 + 钉钉；恢复时只发钉钉，不要打电话」

```json
{
  "notify_configs": [
    {
      "channel_id": <voice_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1],
      "time_ranges": [],
      "attributes": [
        { "key": "is_recovered", "func": "==", "value": "false" }
      ]
    },
    {
      "channel_id": <dingtalk_channel_id>,
      "params": { "user_group_ids": [<oncall_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": []
    }
  ]
}
```

**关键决策**：电话那条加 `is_recovered == "false"` 属性即可；钉钉那条不加属性，告警和恢复都走。这是标准解法。

## 模板 D：按业务组路由到不同群

> 「一套告警规则覆盖三个业务组（zone-a/zone-b/zone-c），每个业务组的告警推送到对应钉钉群」

```json
{
  "notify_configs": [
    {
      "channel_id": <ding_zone_a_id>,
      "severities": [1, 2, 3],
      "attributes": [{ "key": "group_name", "func": "==", "value": "zone-a" }]
    },
    {
      "channel_id": <ding_zone_b_id>,
      "severities": [1, 2, 3],
      "attributes": [{ "key": "group_name", "func": "==", "value": "zone-b" }]
    },
    {
      "channel_id": <ding_zone_c_id>,
      "severities": [1, 2, 3],
      "attributes": [{ "key": "group_name", "func": "==", "value": "zone-c" }]
    }
  ]
}
```

**关键决策**：用 `attributes.group_name` 而不是 `label_keys`，因为业务组是告警规则的归属属性，不是事件 label。**注意**：业务组改名后这里会失配，提醒用户业务组改名要同步规则；或改用 `=~` 加正则更稳。

## 模板 E：按标签灰度

> 「只通知 env=prod 且 service=payment 的告警，其他全部忽略」

```json
{
  "notify_configs": [
    {
      "channel_id": <ding_channel_id>,
      "severities": [1, 2, 3],
      "label_keys": [
        { "key": "env",     "value": "prod" },
        { "key": "service", "value": "payment" }
      ]
    }
  ]
}
```

**关键决策**：两条 label_keys 是 AND，事件必须同时带这两个标签。如果 `service` 在某些告警事件里不存在（如主机失联告警），这条规则会直接不命中——可以先用 `GET /api/n9e/event-tagkeys` 看事件实际有哪些 key。

## 模板 F：兜底通知（避免漏告）

> 「我有 N 条精细通知规则，但担心漏，再来一条接所有事件发给 SRE 团队做留痕」

```json
{
  "name": "全量留痕兜底",
  "description": "所有事件都进 SRE 钉钉群，纯留痕用",
  "notify_configs": [
    {
      "channel_id": <sre_archive_ding_id>,
      "template_id": <minimal_template_id>,
      "params": { "user_group_ids": [<sre_team_id>] },
      "severities": [1, 2, 3],
      "time_ranges": []
    }
  ]
}
```

**关键决策**：留一条"无任何过滤"的规则做兜底。注意告警规则侧的 `notify_rule_ids` 也得显式挂上这条，否则不生效。

---

# 基础完整示例

## 示例一：基础通知规则（全天候通知所有级别告警）

```json
{
  "name": "运维团队告警通知",
  "description": "所有级别的告警都通知运维团队",
  "enable": true,
  "user_group_ids": [1],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_group_ids": [1] },
      "severities": [1, 2, 3],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    }
  ]
}
```

## 示例二：按级别分渠道通知

```json
{
  "name": "分级通知策略",
  "description": "一级告警电话通知，二三级告警邮件通知",
  "enable": true,
  "user_group_ids": [1, 2],
  "notify_configs": [
    {
      "channel_id": 2,
      "template_id": 3,
      "params": { "user_ids": [1, 2], "user_group_ids": [1] },
      "severities": [1],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    },
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_group_ids": [2] },
      "severities": [2, 3],
      "time_ranges": [],
      "label_keys": [],
      "attributes": []
    }
  ]
}
```

## 示例三：工作时间通知 + 属性过滤

```json
{
  "name": "生产环境工作时间通知",
  "description": "仅工作日工作时间通知生产环境的告警",
  "enable": true,
  "user_group_ids": [1],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_group_ids": [1] },
      "severities": [1, 2],
      "time_ranges": [
        { "week": [1, 2, 3, 4, 5], "start": "09:00", "end": "18:00" }
      ],
      "label_keys": [],
      "attributes": [
        { "key": "group_name", "func": "=~", "value": "prod-.*" }
      ]
    }
  ]
}
```

## 示例四：按标签和属性联合过滤

```json
{
  "name": "API服务告警通知",
  "description": "通知 API 服务相关的未恢复告警",
  "enable": true,
  "user_group_ids": [3],
  "notify_configs": [
    {
      "channel_id": 1,
      "template_id": 1,
      "params": { "user_group_ids": [3] },
      "severities": [1, 2, 3],
      "time_ranges": [],
      "label_keys": [
        { "key": "service", "value": "api" }
      ],
      "attributes": [
        { "key": "is_recovered", "func": "==", "value": "false" },
        { "key": "severity", "func": "in", "value": "1 2" }
      ]
    }
  ]
}
```
