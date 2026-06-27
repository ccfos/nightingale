# 订阅规则 config 字段全表

数据模型 `models/alert_subscribe.go:AlertSubscribe`。

## 基础字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 订阅规则名称 |
| `note` | string | 否 | 备注说明 |
| `disabled` | int | 否 | `0`=启用（默认），`1`=禁用。禁用的订阅在缓存层被过滤，完全不参与匹配 |
| `group_id` | int64 | 是 | **管理归属业务组**（决定谁能看/改这条订阅），**不参与事件匹配**（工具可经 `group_id` 参数传入） |
| `prod` | string | 否 | 产品类型。非空时与事件的 RuleProd **精确匹配**；空 = 不过滤。常见值 `"metric"`/`"logging"`/`"host"` |
| `cate` | string | 否 | 数据源类型。**当前实现只有 `"host"` 有实际过滤作用**（只匹配 host 事件），其他取值等同不过滤；空 = 不过滤 |
| `datasource_ids` | int[] | 否 | 数据源 ID 列表；空数组或含 `0` = 全部（工具自动归一为 `[0]`）；事件无数据源（dsId=0）时跳过此过滤 |
| `cluster` | string | 否 | 固定填 `"0"`（V5 遗留字段） |

## 订阅过滤条件

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_ids` | int[] | 否 | 订阅的告警规则 ID 列表；空 = 订阅**所有**告警规则的事件（全局订阅） |
| `severities` | int[] | **是** | 订阅的告警级别，新旧版都校验非空；`[1,2,3]` = 全部 |
| `for_duration` | int64 | 否 | 秒。告警持续时长（`trigger_time - first_trigger_time`）**超过**该值才转发——做"告警升级"用（如 `300` = 持续 5 分钟未恢复才走订阅）。`0` = 不限制 |
| `tags` | array | 否 | 事件标签过滤，多条 **AND**，结构同下 |
| `busi_groups` | array | 否 | 按事件**业务组名**过滤，多条 **AND**。元素 `{"key":"groups","func":"=~","value":"生产.*"}`——key 约定写 `"groups"`（实现上 key 不参与匹配，func/value 对事件 GroupName 匹配） |

### tags 元素结构与操作符

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

常用标签：`ident`（机器标识）、`rulename`（告警规则名）、`__name__`（指标名）、自定义业务标签。

### severity 告警级别

| 值 | 含义 |
|---|---|
| 1 | 一级报警 (Critical) |
| 2 | 二级报警 (Warning) |
| 3 | 三级报警 (Info) |

## 通知配置：新版 vs 旧版

| 字段 | 类型 | 说明 |
|---|---|---|
| `notify_version` | int | `1`=新版（推荐，经通知规则转发）；`0`=旧版（直填用户组+渠道） |
| `notify_rule_ids` | int[] | 新版必填非空：克隆事件的通知出口改写为这些通知规则 |

**新版（notify_version=1）校验会清空全部旧版字段**：`user_group_ids`、`redefine_channels`/`new_channels`、`redefine_webhooks`/`webhooks`、`redefine_severity`/`new_severity`（`models/alert_subscribe.go:Verify`）。也就是说**改写级别/渠道/回调是旧版能力**；新版想变级别/渠道，去通知规则层面做路由（→ notify-rule-copilot）。

旧版（notify_version=0）字段，仅在维护存量配置时会遇到：

| 字段 | 说明 |
|---|---|
| `user_group_ids` | 接收用户组 ID，**空格分隔字符串**（如 `"1 2"`）；指定了它就必须同时指定 `new_channels` |
| `redefine_severity` / `new_severity` | =1 时克隆事件级别改为 new_severity |
| `redefine_channels` / `new_channels` | =1 时克隆事件通知渠道改为 new_channels（空格分隔） |
| `redefine_webhooks` / `webhooks` | =1 时克隆事件回调改为 webhooks（JSON 数组）。**不开启时克隆事件的回调被清空**（防重复打原始回调）——这一条新版同样适用 |
| `extra_config` | 扩展配置 JSON 对象，引擎无固定用途，照抄 `{}` 即可 |

## 完整示例

### 示例一：订阅所有 Critical 告警转发给值班通知规则（最常见）

```json
{
  "name": "订阅所有 Critical 告警",
  "note": "所有严重告警抄送值班链路",
  "disabled": 0,
  "prod": "",
  "cate": "",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1],
  "for_duration": 0,
  "tags": [],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [1]
}
```

### 示例二：按标签订阅特定机器的告警

```json
{
  "name": "订阅 web 集群告警",
  "note": "所有 web 开头机器的告警抄送 web 团队",
  "disabled": 0,
  "prod": "metric",
  "cate": "",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1, 2, 3],
  "for_duration": 0,
  "tags": [
    {"key": "ident", "func": "=~", "value": "web.*"}
  ],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [1]
}
```

### 示例三：告警升级——指定规则持续 5 分钟未恢复再通知上级

```json
{
  "name": "CPU 告警升级",
  "note": "CPU 相关告警持续 5 分钟未恢复，升级通知到二线",
  "disabled": 0,
  "prod": "metric",
  "cate": "",
  "datasource_ids": [1],
  "cluster": "0",
  "rule_ids": [10, 11],
  "severities": [1, 2],
  "for_duration": 300,
  "tags": [],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [2]
}
```

### 示例四：跨业务组订阅——只收"生产"业务组的数据库告警

```json
{
  "name": "订阅生产环境数据库告警",
  "note": "生产业务组下数据库相关告警抄送 DBA",
  "disabled": 0,
  "prod": "metric",
  "cate": "",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1, 2],
  "for_duration": 0,
  "tags": [
    {"key": "rulename", "func": "=~", "value": ".*数据库.*|.*MySQL.*|.*Redis.*"}
  ],
  "busi_groups": [
    {"key": "groups", "func": "=~", "value": "生产.*"}
  ],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [2, 3]
}
```

### 示例五：告警事件汇聚转发到工单系统

```json
{
  "name": "告警转发到工单系统",
  "note": "Warning 以上告警统一经通知规则转发到内部工单系统",
  "disabled": 0,
  "prod": "",
  "cate": "",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1, 2],
  "for_duration": 0,
  "tags": [],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [5]
}
```
