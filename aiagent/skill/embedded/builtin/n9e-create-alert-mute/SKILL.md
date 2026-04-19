---
name: n9e-create-alert-mute
description: 在夜莺(n9e)环境中创建告警屏蔽规则。当用户要求创建屏蔽规则、屏蔽告警、静默告警、添加告警抑制时使用。
---

# 夜莺(n9e) 创建告警屏蔽规则

在夜莺监控平台上创建告警屏蔽（静默）规则，用于在特定时间范围内按标签条件屏蔽匹配的告警事件。

---

## 前置条件

用户需要提供：
- **n9e 地址**：如 `http://<n9e-host>:<port>`
- **用户名/密码**：如 `<username>/<password>`
- **屏蔽内容描述**：如 "屏蔽 host=web01 的所有告警 2 小时"

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

### 第二步：询问业务组

调用 API 获取业务组列表：

```
GET /api/n9e/busi-groups
Authorization: Bearer <token>
```

将返回的业务组列表通过 **AskUserQuestion** 工具展示给用户，让用户选择要创建屏蔽规则的业务组。

### 第三步：根据用户描述构建屏蔽规则

根据用户的屏蔽需求，构建屏蔽规则 payload 并调用创建 API：

```
POST /api/n9e/busi-group/<busi_group_id>/alert-mutes
Authorization: Bearer <token>
Content-Type: application/json
Body: <屏蔽规则对象>
```

### 第四步：验证

```
GET /api/n9e/busi-group/<busi_group_id>/alert-mute/<mute_id>
Authorization: Bearer <token>
```

向用户输出创建结果摘要。

---

## 屏蔽规则 Payload 结构

```json
{
  "note": "屏蔽规则名称/标题",
  "cause": "屏蔽原因说明",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [1, 2, 3],
  "tags": [
    {"key": "ident", "func": "==", "value": "web01"}
  ],
  "mute_time_type": 0,
  "btime": 1712000000,
  "etime": 1712007200,
  "periodic_mutes": [],
  "cluster": "0"
}
```

---

## 字段说明

### 基础字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `note` | string | 是 | 屏蔽规则名称/标题 |
| `cause` | string | 否 | 屏蔽原因 |
| `prod` | string | 否 | 产品类型，默认 `"metric"` |
| `cate` | string | 否 | 数据源类型，如 `"prometheus"`、`"elasticsearch"`、`"loki"` 等 |
| `datasource_ids` | int[] | 否 | 数据源 ID 列表，空数组表示匹配全部数据源 |
| `severities` | int[] | 是 | 要屏蔽的告警级别，`[1, 2, 3]` 表示全部级别 |
| `cluster` | string | 否 | 固定填 `"0"` |

### severity 告警级别

| 值 | 含义 |
|---|---|
| 1 | 一级报警 (Critical) |
| 2 | 二级报警 (Warning) |
| 3 | 三级报警 (Info) |

### 标签过滤 (tags)

`tags` 是一个数组，每个元素定义一个标签匹配条件：

```json
{
  "key": "标签名",
  "func": "匹配操作符",
  "value": "匹配值"
}
```

| func 操作符 | 含义 | value 示例 |
|---|---|---|
| `==` | 精确匹配 | `"web01"` |
| `!=` | 不等于 | `"web01"` |
| `=~` | 正则匹配 | `"web.*"` |
| `!~` | 正则不匹配 | `"web.*"` |
| `in` | 在列表中（空格分隔） | `"web01 web02 web03"` |
| `not in` | 不在列表中（空格分隔） | `"web01 web02"` |

**多个 tag 之间是 AND 关系**，即告警事件必须同时匹配所有 tag 条件才会被屏蔽。

常用标签举例：
- `ident`：机器标识/主机名
- `rulename`：告警规则名称
- `__name__`：指标名称
- 自定义业务标签

### 屏蔽时间配置

屏蔽时间有两种模式，由 `mute_time_type` 字段决定：

#### 模式一：固定时间范围 (mute_time_type: 0)

在指定的起止时间内屏蔽告警。

| 字段 | 类型 | 说明 |
|---|---|---|
| `mute_time_type` | int | 固定为 `0` |
| `btime` | int64 | 开始时间，Unix 时间戳（秒） |
| `etime` | int64 | 结束时间，Unix 时间戳（秒），必须大于 `btime` |
| `periodic_mutes` | array | 留空数组 `[]` |

常用时长参考（从当前时间起算）：
- 1 小时：`etime = btime + 3600`
- 2 小时：`etime = btime + 7200`
- 6 小时：`etime = btime + 21600`
- 1 天：`etime = btime + 86400`
- 7 天：`etime = btime + 604800`

#### 模式二：周期性屏蔽 (mute_time_type: 1)

按周几和时间段周期性屏蔽告警。

| 字段 | 类型 | 说明 |
|---|---|---|
| `mute_time_type` | int | 固定为 `1` |
| `btime` | int64 | 周期生效的起始日期，Unix 时间戳（秒） |
| `etime` | int64 | 周期生效的结束日期，Unix 时间戳（秒），必须大于 `btime` |
| `periodic_mutes` | array | 周期屏蔽配置数组 |

`periodic_mutes` 数组元素结构：

```json
{
  "enable_days_of_week": "1 2 3 4 5",
  "enable_stime": "02:00",
  "enable_etime": "06:00"
}
```

| 字段 | 类型 | 说明 |
|---|---|---|
| `enable_days_of_week` | string | 生效的星期，空格分隔。0=周日, 1=周一, ..., 6=周六 |
| `enable_stime` | string | 每天生效开始时间，格式 `HH:mm` |
| `enable_etime` | string | 每天生效结束时间，格式 `HH:mm` |

可以配置多组 `periodic_mutes` 实现多个时间段的周期性屏蔽。

---

## 完整示例

### 示例一：固定时间屏蔽指定机器的告警（2 小时）

```json
{
  "note": "维护窗口：屏蔽 web01 告警",
  "cause": "web01 计划维护，预计 2 小时",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [1, 2, 3],
  "tags": [
    {"key": "ident", "func": "==", "value": "web01"}
  ],
  "mute_time_type": 0,
  "btime": 1712000000,
  "etime": 1712007200,
  "periodic_mutes": [],
  "cluster": "0"
}
```

### 示例二：固定时间屏蔽多台机器的特定告警

```json
{
  "note": "批量维护：屏蔽 web 集群 CPU 告警",
  "cause": "web 集群升级维护",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [2, 3],
  "tags": [
    {"key": "ident", "func": "in", "value": "web01 web02 web03"},
    {"key": "rulename", "func": "=~", "value": ".*CPU.*"}
  ],
  "mute_time_type": 0,
  "btime": 1712000000,
  "etime": 1712086400,
  "periodic_mutes": [],
  "cluster": "0"
}
```

### 示例三：周期性屏蔽（每天凌晨 2-6 点屏蔽）

```json
{
  "note": "日常维护窗口：凌晨屏蔽",
  "cause": "每日凌晨批处理任务期间屏蔽告警",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [2, 3],
  "tags": [
    {"key": "ident", "func": "=~", "value": "batch-.*"}
  ],
  "mute_time_type": 1,
  "btime": 1712000000,
  "etime": 1714592000,
  "periodic_mutes": [
    {
      "enable_days_of_week": "0 1 2 3 4 5 6",
      "enable_stime": "02:00",
      "enable_etime": "06:00"
    }
  ],
  "cluster": "0"
}
```

### 示例四：工作日屏蔽特定时间段

```json
{
  "note": "工作日午休屏蔽",
  "cause": "午休时间减少非紧急告警打扰",
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "severities": [3],
  "tags": [],
  "mute_time_type": 1,
  "btime": 1712000000,
  "etime": 1714592000,
  "periodic_mutes": [
    {
      "enable_days_of_week": "1 2 3 4 5",
      "enable_stime": "12:00",
      "enable_etime": "13:30"
    }
  ],
  "cluster": "0"
}
```

---

## 关键注意事项

1. **创建 API 接收对象而非数组**：与告警规则不同，屏蔽规则 payload 是单个对象 `{...}`，不是数组
2. **btime/etime 必须是 Unix 时间戳（秒）**：使用当前时间戳作为 btime，根据用户指定的时长计算 etime
3. **tags 为空数组时匹配所有告警**：如果不指定标签条件，该屏蔽规则将匹配业务组内所有告警
4. **severities 不能为空**：至少指定一个告警级别，`[1, 2, 3]` 表示屏蔽所有级别
5. **datasource_ids 为空数组匹配全部数据源**：如果不限制数据源，设为 `[]`
6. **周期性屏蔽也需要 btime/etime**：用于限定周期性屏蔽的整体生效日期范围
7. **多个 tag 之间是 AND 关系**：事件必须同时匹配所有 tag 才会被屏蔽
8. **cluster 字段**：固定填 `"0"`
9. **in/not in 的 value 用空格分隔**：如 `"web01 web02 web03"`，不要用逗号
