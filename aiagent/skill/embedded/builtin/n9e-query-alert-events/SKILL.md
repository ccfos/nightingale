---
name: n9e-query-alert-events
description: 在夜莺(n9e)环境中查询告警事件。当用户要求查看告警、查询活跃告警、搜索历史告警、查看告警详情、统计告警事件时使用。
---

# 夜莺(n9e) 查询告警事件

在夜莺监控平台上查询和查看告警事件，支持查询当前活跃告警（未恢复）、历史告警（已恢复/未恢复）、以及获取单条告警的详细信息。

---

## 前置条件

用户需要提供：
- **n9e 地址**：如 `http://<n9e-host>:<port>`
- **用户名/密码**：如 `<username>/<password>`
- **查询需求描述**：如 "最近1小时有哪些一级告警"、"查看活跃告警"、"告警ID 123的详情"

如果用户未提供以上信息，使用 AskUserQuestion 工具询问。

---

## 执行步骤

### 第一步：登录获取 Token

```
POST /api/n9e/auth/login
Content-Type: application/json
Body: {"username":"<用户名>","password":"<密码>"}
```

从响应中提取 `dat.access_token`，后续请求都带上 `Authorization: Bearer <token>`。

### 第二步：根据用户需求选择查询类型

根据用户意图判断使用哪种查询方式：

- **查看活跃告警**（当前未恢复的告警）→ 使用活跃告警查询 API
- **查看历史告警**（包含已恢复和未恢复）→ 使用历史告警查询 API
- **查看告警详情**（指定某条告警的完整信息）→ 使用告警详情 API

判断规则：
- 用户提到"活跃"、"当前"、"未恢复"、"正在告警" → 活跃告警
- 用户提到"历史"、"过去"、"已恢复"、"恢复了" → 历史告警
- 用户提到具体的告警 ID → 告警详情
- 未明确指定时，默认查询活跃告警

### 第三步：执行查询

#### 方式一：查询活跃告警

```
GET /api/n9e/alert-cur-events/list?<query_params>
Authorization: Bearer <token>
```

#### 方式二：查询历史告警

```
GET /api/n9e/alert-his-events/list?<query_params>
Authorization: Bearer <token>
```

#### 方式三：查询告警详情

活跃告警详情：

```
GET /api/n9e/alert-cur-event/<event_id>
Authorization: Bearer <token>
```

历史告警详情：

```
GET /api/n9e/alert-his-event/<event_id>
Authorization: Bearer <token>
```

### 第四步：格式化输出

将查询结果以可读的 Markdown 表格或列表形式展示给用户，包含关键信息：告警名称、级别、触发时间、持续时间、触发值、标签等。

---

## 查询参数说明

### 活跃告警查询参数（alert-cur-events/list）

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `severity` | string | 否 | 全部 | 告警级别，逗号分隔，如 `"1,2"` |
| `query` | string | 否 | 空 | 搜索关键词，匹配规则名称或标签 |
| `stime` | int64 | 否 | 无限制 | 开始时间，Unix 时间戳（秒） |
| `etime` | int64 | 否 | 无限制 | 结束时间，Unix 时间戳（秒） |
| `hours` | int64 | 否 | 0 | 最近 N 小时（与 stime/etime 二选一） |
| `limit` | int | 否 | 20 | 每页数量 |
| `p` | int | 否 | 1 | 页码 |
| `bgid` | int64 | 否 | 0 | 业务组 ID 过滤 |
| `rid` | int64 | 否 | 0 | 告警规则 ID 过滤 |
| `datasource_ids` | string | 否 | 全部 | 数据源 ID，逗号分隔 |
| `prods` | string | 否 | 全部 | 产品类型，逗号分隔，如 `"metric,host"` |
| `cate` | string | 否 | `$all` | 数据源类别，逗号分隔，如 `"prometheus,host"` |
| `my_groups` | bool | 否 | false | 只看自己业务组的告警 |
| `event_ids` | string | 否 | 空 | 指定事件 ID，逗号分隔 |

### 历史告警查询参数（alert-his-events/list）

| 参数 | 类型 | 必填 | 默认值 | 说明 |
|---|---|---|---|---|
| `severity` | int | 否 | -1 | 告警级别，单个值，-1 表示全部 |
| `is_recovered` | int | 否 | -1 | 恢复状态：0=未恢复, 1=已恢复, -1=全部 |
| `query` | string | 否 | 空 | 搜索关键词，匹配规则名称或标签 |
| `stime` | int64 | 否 | 无限制 | 开始时间，Unix 时间戳（秒） |
| `etime` | int64 | 否 | 无限制 | 结束时间，Unix 时间戳（秒） |
| `hours` | int64 | 否 | 0 | 最近 N 小时（与 stime/etime 二选一） |
| `limit` | int | 否 | 20 | 每页数量 |
| `p` | int | 否 | 1 | 页码 |
| `bgid` | int64 | 否 | 0 | 业务组 ID 过滤 |
| `rid` | int64 | 否 | 0 | 告警规则 ID 过滤 |
| `datasource_ids` | string | 否 | 全部 | 数据源 ID，逗号分隔 |
| `prods` | string | 否 | 全部 | 产品类型，逗号分隔 |
| `cate` | string | 否 | `$all` | 数据源类别，逗号分隔 |

### severity 告警级别

| 值 | 含义 |
|---|---|
| 1 | 一级报警 (Critical) |
| 2 | 二级报警 (Warning) |
| 3 | 三级报警 (Info) |

### 时间参数用法

有两种方式指定时间范围（二选一）：

**方式一**：使用 `hours` 参数（推荐，更简单）
- `hours=1` → 最近 1 小时
- `hours=6` → 最近 6 小时
- `hours=24` → 最近 24 小时
- `hours=168` → 最近 7 天

**方式二**：使用 `stime` + `etime` 参数（精确控制）
- 都是 Unix 时间戳（秒）
- 只传 `stime` 时，`etime` 自动设为当前时间 + 24 小时

---

## 响应数据结构

### 列表响应

```json
{
  "dat": {
    "total": 42,
    "list": [<告警事件对象>, ...]
  }
}
```

### 活跃告警事件字段

```json
{
  "id": 12345,
  "rule_id": 10,
  "rule_name": "CPU使用率过高",
  "rule_note": "CPU使用率超过80%持续5分钟",
  "rule_prod": "metric",
  "severity": 1,
  "cate": "prometheus",
  "cluster": "default",
  "datasource_id": 1,
  "group_id": 2,
  "group_name": "生产环境",
  "hash": "rule_10_xxxx",
  "target_ident": "web-server-01",
  "target_note": "Web服务器",
  "first_trigger_time": 1712000000,
  "trigger_time": 1712003600,
  "last_eval_time": 1712003600,
  "trigger_value": "85.6",
  "prom_ql": "cpu_usage_active{ident=\"web-server-01\"}",
  "prom_eval_interval": 30,
  "prom_for_duration": 300,
  "tags": ["ident=web-server-01", "cpu=cpu-total"],
  "annotations": {"description": "web-server-01 CPU高"},
  "notify_version": 1,
  "notify_channels": [],
  "notify_groups_obj": [],
  "notify_rules": [{"id": 1, "name": "运维通知"}],
  "callbacks": [],
  "runbook_url": ""
}
```

### 历史告警事件额外字段

历史告警事件在活跃告警的基础上增加：

```json
{
  "is_recovered": 1,
  "recover_time": 1712007200
}
```

| is_recovered 值 | 含义 |
|---|---|
| 0 | 未恢复（告警仍在触发） |
| 1 | 已恢复 |

---

## 辅助查询 API

### 获取可用的事件标签 key

```
GET /api/n9e/event-tagkeys
Authorization: Bearer <token>
```

返回可用于搜索过滤的标签键列表。

### 获取标签值

```
GET /api/n9e/event-tagvalues?key=<tag_key>
Authorization: Bearer <token>
```

返回指定标签键的前 20 个高频值。

### 获取告警事件关联的数据源

```
GET /api/n9e/alert-cur-events-datasources
Authorization: Bearer <token>
```

### 获取业务组列表（用于 bgid 过滤）

```
GET /api/n9e/busi-groups
Authorization: Bearer <token>
```

---

## 常见查询示例

### 示例一：查看所有活跃告警

```
GET /api/n9e/alert-cur-events/list?limit=50
```

### 示例二：查看最近1小时的一级告警

```
GET /api/n9e/alert-cur-events/list?severity=1&hours=1
```

### 示例三：按关键词搜索活跃告警

```
GET /api/n9e/alert-cur-events/list?query=CPU&limit=20
```

### 示例四：查看最近24小时的历史告警（仅已恢复）

```
GET /api/n9e/alert-his-events/list?hours=24&is_recovered=1&limit=50
```

### 示例五：查看指定业务组的活跃告警

```
GET /api/n9e/alert-cur-events/list?bgid=2&limit=50
```

### 示例六：查看指定告警规则触发的事件

```
GET /api/n9e/alert-cur-events/list?rid=10&limit=50
```

### 示例七：组合过滤（一二级告警 + 关键词 + 时间范围）

```
GET /api/n9e/alert-cur-events/list?severity=1,2&query=web&hours=6&limit=30
```

### 示例八：获取单条告警详情

```
GET /api/n9e/alert-cur-event/12345
```

---

## 关键注意事项

1. **活跃告警 vs 历史告警**：活跃告警仅包含当前未恢复的告警；历史告警包含已恢复和未恢复的告警
2. **severity 参数格式不同**：活跃告警支持逗号分隔多个级别（如 `"1,2"`），历史告警只接受单个值（如 `1`）
3. **默认不限时间**：活跃告警查询默认不限制时间范围，历史告警建议指定 `hours` 或 `stime`/`etime` 避免返回过多数据
4. **分页**：使用 `limit` 和 `p`（页码）参数分页，`limit` 默认 20
5. **tags 格式**：响应中的 `tags` 是 `["key=value"]` 格式的字符串数组
6. **时间字段都是 Unix 时间戳（秒）**：`trigger_time`、`first_trigger_time`、`recover_time`、`last_eval_time` 等
7. **query 搜索范围**：关键词会同时匹配告警规则名称（`rule_name`）和标签（`tags`）
8. **告警详情 URL 不同**：活跃告警用 `/alert-cur-event/<id>`，历史告警用 `/alert-his-event/<id>`（注意单复数）
9. **输出格式**：将结果格式化为 Markdown 表格输出给用户，包含告警名称、级别、触发时间、目标、触发值等关键列
