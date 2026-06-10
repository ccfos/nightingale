# 屏蔽规则 config 字段全表

数据模型 `models/alert_mute.go:AlertMute`。

## 基础字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `group_id` | int64 | 是 | 所属业务组 ID（工具可经 `group_id` 参数传入） |
| `note` | string | 是 | 屏蔽规则名称/标题 |
| `cause` | string | 否 | 屏蔽原因 |
| `prod` | string | 否 | 产品类型，默认 `"metric"`。**仅存储展示，不参与匹配** |
| `cate` | string | 否 | 数据源类型，如 `"prometheus"`。**仅存储展示，不参与匹配** |
| `cluster` | string | 否 | 固定填 `"0"`（V5 遗留字段） |
| `datasource_ids` | int[] | 否 | 数据源 ID 列表；空数组或含 `0` = 全部数据源（工具/Verify 自动归一为 `[0]`） |
| `severities` | int[] | 建议填 | 要屏蔽的告警级别；`[1,2,3]` = 全部。引擎语义：列表非空时事件级别必须在其中 |
| `tags` | array | 否 | 标签匹配条件，多条 **AND**；**空数组 = 屏蔽业务组内所有告警** |
| `mute_time_type` | int | 是 | `0`=固定时间范围，`1`=周期屏蔽 |
| `disabled` | int | 否 | `0`=启用（默认），`1`=禁用。禁用的规则完全不参与匹配 |

### severity 告警级别

| 值 | 含义 |
|---|---|
| 1 | 一级报警 (Critical) |
| 2 | 二级报警 (Warning) |
| 3 | 三级报警 (Info) |

## 标签过滤 (tags)

```json
{ "key": "标签名", "func": "匹配操作符", "value": "匹配值" }
```

| func | 含义 | value 示例 |
|---|---|---|
| `==` | 精确匹配 | `"web01"` |
| `!=` | 不等于 | `"web01"` |
| `=~` | 正则匹配 | `"web.*"` |
| `!~` | 正则不匹配 | `"web.*"` |
| `in` | 在列表中（空格分隔） | `"web01 web02 web03"` |
| `not in` | 不在列表中（空格分隔） | `"web01 web02"` |

- **多个 tag 之间是 AND**：事件必须同时匹配所有条件才被屏蔽（`alert/common/key.go:MatchTags`）。
- `in`/`not in` 的 value 既可写空格分隔字符串，也可直接传数组（如 `["web01","web02"]`），工具自动归一成空格分隔。
- `key` 不能为空；`func` 必须是上表六种之一（工具在落库前校验）。

常用标签：`ident`（机器标识/主机名）、`rulename`（告警规则名称）、`__name__`（指标名称）、自定义业务标签。

## 屏蔽时间配置

### 模式一：固定时间范围 (mute_time_type: 0)

在 `[btime, etime]` 闭区间内屏蔽（按事件触发时间判断）。**推荐用工具的 `duration` 参数表达时长，不自己算时间戳。**

| 字段 | 类型 | 说明 |
|---|---|---|
| `mute_time_type` | int | 固定 `0` |
| `duration`（**工具参数**，非 config 字段） | string | 屏蔽时长：`"2h"`/`"30m"`/`"7d"`/`"1d12h"`/`"1w"`（支持 s/m/h/d/w 及组合）。传了它就不用填 btime/etime |
| `btime` | int64 | 开始时间 Unix 秒；**省略默认当前时间** |
| `etime` | int64 | 结束时间 Unix 秒；用了 `duration` 不用填（工具按 `btime+duration` 算）。**`etime > btime` 是硬校验** |
| `periodic_mutes` | array | 留空 `[]` |

> 只有用户给**绝对起止时刻**（而非"屏蔽多久"）时才自己填 `btime`/`etime`。系统提示的 `Now:` 行已给出当前精确时间和 Unix 秒。

### 模式二：周期性屏蔽 (mute_time_type: 1)

按周几和时间段周期性屏蔽。

| 字段 | 类型 | 说明 |
|---|---|---|
| `mute_time_type` | int | 固定 `1` |
| `btime` / `etime` | int64 | **可省**（工具自动补 当前时间/一年后）。注意：**当前实现的周期匹配只看 periodic_mutes 的星期+时段，btime/etime 不参与判断**，它们只为通过 `etime > btime` 校验 |
| `periodic_mutes` | array | 周期配置，可多组（任一组命中即屏蔽，OR） |

`periodic_mutes` 元素结构：

```json
{
  "enable_days_of_week": "1 2 3 4 5",
  "enable_stime": "02:00",
  "enable_etime": "06:00"
}
```

| 字段 | 说明 |
|---|---|
| `enable_days_of_week` | 生效星期，空格分隔，**0=周日…6=周六**。也可直接写 `"工作日"`/`"每天"`/`"周末"`（及 weekday/everyday/weekend 等英文），工具自动转换 |
| `enable_stime` / `enable_etime` | 每天生效时段 `HH:mm`。写 `"全天"`（或 allday/24h）= 00:00~23:59；`stime == etime` 也视为全天 |

- **跨午夜原生支持**：`stime > etime`（如 `22:00`~`06:00`）按"`>= stime` 或 `< etime`"判断，不用拆两段。
- 时间判断用 n9e 进程的本地时区。

## 完整示例

### 示例一：固定时间屏蔽指定机器（2 小时）

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

> 调用时传工具参数 `duration: "1d"`。

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

### 示例三：周期性屏蔽（每天凌晨 2-6 点）

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

### 示例四：工作日午休时段屏蔽低级别告警

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

> 注意：示例四 tags 为空 = 屏蔽业务组内**所有** Info 级告警，落库前向用户确认影响面。
