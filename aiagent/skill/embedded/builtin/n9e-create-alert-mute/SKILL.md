---
name: n9e-create-alert-mute
description: 在夜莺(n9e)环境中创建告警屏蔽规则。当用户要求创建屏蔽规则、屏蔽告警、静默告警、添加告警抑制时使用。
tags:
  - internal
---

# 夜莺(n9e) 创建告警屏蔽规则

在夜莺监控平台上创建告警屏蔽（静默）规则，用于在特定时间范围内按标签条件屏蔽匹配的告警事件。

---

## 前提

你是 n9e 站内 AI 助手，运行在 n9e 进程内、已以当前用户身份认证。**直接调用内置工具创建，不要登录、不要调 HTTP API、不要用 http_fetch 打自家接口。**

---

## 执行步骤

### 第一步：确定业务组

屏蔽规则属于某个业务组。用 `list_busi_groups` 列出可选业务组，拿到 `group_id`。
> 用户在对话里点名了业务组、或前端已弹出业务组选择表单时，直接用其 ID，不必再问。

### 第二步：根据用户描述构建 config

按下文「config 结构」把屏蔽规则拼成一个 JSON 对象：
- `tags` 是标签匹配条件数组，决定屏蔽哪些告警（如 `{"key":"ident","func":"==","value":"web01"}`）。
- **时间不用自己算 Unix 时间戳**：
  - 固定时段（`mute_time_type=0`）：优先把时长传给工具的 `duration` 参数（如 `"2h"`/`"7d"`），`btime` 省略即默认当前时间，工具自动算 `etime`。只有用户给的是绝对起止时刻时才自己填 `btime`/`etime`。
  - 周期时段（`mute_time_type=1`）：填 `periodic_mutes` 即可，`btime`/`etime` 可省（默认从现在起一年）。

### 第三步：调用 create_alert_mute

调用 `create_alert_mute` 工具，`group_id` 传第一步的业务组（也可写在 config 里），`config` 传上一步的 JSON 字符串；固定时段屏蔽把时长传给 `duration` 参数（如 `"2h"`）。
- 若没带 `group_id`，工具会自动弹出业务组选择表单，用户选完会续上本次创建。

### 第四步：回报结果

工具返回 `{id, group_id, cause, btime, etime}`。据此向用户简要汇报即可。

---

## config 结构

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
  "periodic_mutes": [],
  "cluster": "0"
}
```

> 固定时段屏蔽的时长走工具的 `duration` 参数（如 `"2h"`），所以 config 里通常不用写 `btime`/`etime`。

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

> `in`/`not in` 的 `value` 既可写空格分隔字符串，也可直接传数组（如 `["web01","web02"]`），工具会自动归一成空格分隔。

常用标签举例：
- `ident`：机器标识/主机名
- `rulename`：告警规则名称
- `__name__`：指标名称
- 自定义业务标签

### 屏蔽时间配置

屏蔽时间有两种模式，由 `mute_time_type` 字段决定：

#### 模式一：固定时间范围 (mute_time_type: 0)

在指定的起止时间内屏蔽告警。**推荐用工具的 `duration` 参数表达时长，不用自己算时间戳。**

| 字段 | 类型 | 说明 |
|---|---|---|
| `mute_time_type` | int | 固定为 `0` |
| `duration`（工具参数，非 config 字段） | string | 屏蔽时长，如 `"2h"`/`"30m"`/`"7d"`/`"1d12h"`（支持 s/m/h/d/w）。传了它就不用填 btime/etime |
| `btime` | int64 | 开始时间，Unix 秒；**省略默认当前时间** |
| `etime` | int64 | 结束时间，Unix 秒；用了 `duration` 就不用填（工具按 `btime+duration` 自动算） |
| `periodic_mutes` | array | 留空数组 `[]` |

> 只有当用户给的是**绝对起止时刻**（而非"屏蔽多久"）时，才需要自己填 `btime`/`etime`（Unix 秒，`etime>btime`）。系统提示的 `Now:` 行已给出当前精确时间和 Unix 秒，可据此换算。

#### 模式二：周期性屏蔽 (mute_time_type: 1)

按周几和时间段周期性屏蔽告警。

| 字段 | 类型 | 说明 |
|---|---|---|
| `mute_time_type` | int | 固定为 `1` |
| `btime` | int64 | 周期生效的起始日期，Unix 秒；**可省，默认当前时间** |
| `etime` | int64 | 周期生效的结束日期，Unix 秒；**可省，默认从现在起一年** |
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
| `enable_days_of_week` | string | 生效的星期，空格分隔，0=周日…6=周六。**也可直接写 `"工作日"`/`"每天"`/`"周末"`，工具自动转换** |
| `enable_stime` | string | 每天生效开始时间，格式 `HH:mm`；写 `"全天"` 表示 00:00 |
| `enable_etime` | string | 每天生效结束时间，格式 `HH:mm`；与 stime 一起写 `"全天"` 即 00:00~23:59 |

可以配置多组 `periodic_mutes` 实现多个时间段的周期性屏蔽。

---

## 完整示例

### 示例一：固定时间屏蔽指定机器的告警（2 小时）

> 调用时传工具参数 `duration: "2h"`，config 里无需 btime/etime。

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
  "periodic_mutes": [],
  "cluster": "0"
}
```

### 示例二：固定时间屏蔽多台机器的特定告警（1 天）

> 调用时传工具参数 `duration: "1d"`，config 里无需 btime/etime。

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
  "periodic_mutes": [
    {
      "enable_days_of_week": "工作日",
      "enable_stime": "12:00",
      "enable_etime": "13:30"
    }
  ],
  "cluster": "0"
}
```

---

## 关键注意事项

1. **config 是单个 JSON 对象**：`create_alert_mute` 的 `config` 传一个屏蔽规则对象 `{...}`，不是数组
2. **时间不用自己算 Unix 时间戳**：固定时段把时长传 `duration` 参数（如 `"2h"`/`"7d"`），`btime` 省略默认当前时间、`etime` 工具自动算；只有用户给绝对起止时刻时才填 `btime`/`etime`（Unix 秒，`etime>btime`）
   - `datasource_ids` 不传或为空时，工具默认按"全部数据源"处理
3. **tags 为空数组时匹配所有告警**：如果不指定标签条件，该屏蔽规则将匹配业务组内所有告警
4. **severities 不能为空**：至少指定一个告警级别，`[1, 2, 3]` 表示屏蔽所有级别
5. **datasource_ids 为空数组匹配全部数据源**：如果不限制数据源，设为 `[]`
6. **周期性屏蔽的 btime/etime 可省**：缺省为"现在起一年"；周期是否生效只看 `periodic_mutes` 的星期+时段，btime/etime 仅作整体生效区间
7. **多个 tag 之间是 AND 关系**：事件必须同时匹配所有 tag 才会被屏蔽
8. **cluster 字段**：固定填 `"0"`
9. **in/not in 的 value 用空格分隔**：如 `"web01 web02 web03"`，不要用逗号
