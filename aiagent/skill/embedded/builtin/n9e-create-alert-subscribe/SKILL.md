---
name: n9e-create-alert-subscribe
description: 在夜莺(n9e)环境中创建告警订阅规则。当用户要求创建订阅规则、订阅告警、添加告警订阅、配置告警事件转发时使用。
---

# 夜莺(n9e) 创建告警订阅规则

在夜莺监控平台上创建告警订阅规则，用于按条件筛选告警事件并通过通知规则转发给指定的通知对象。

---

## 前置条件

用户需要提供：
- **n9e 地址**：如 `http://<n9e-host>:<port>`
- **用户名/密码**：如 `<username>/<password>`
- **订阅内容描述**：如 "订阅所有 CPU 相关的告警并通知运维组"

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

将返回的业务组列表通过 **AskUserQuestion** 工具展示给用户，让用户选择要创建订阅规则的业务组。

### 第三步：根据用户描述构建订阅规则

根据用户的订阅需求，构建订阅规则 payload 并调用创建 API：

```
POST /api/n9e/busi-group/<busi_group_id>/alert-subscribes
Authorization: Bearer <token>
Content-Type: application/json
Body: <订阅规则对象>
```

如果用户需要选择具体的告警规则来订阅，先获取告警规则列表：

```
GET /api/n9e/busi-group/<busi_group_id>/alert-rules
Authorization: Bearer <token>
```

将返回的告警规则列表通过 **AskUserQuestion** 工具展示给用户，让用户选择要订阅的告警规则。

### 第四步：询问并关联通知规则

获取通知规则列表：

```
GET /api/n9e/notify-rules
Authorization: Bearer <token>
```

将返回的通知规则列表通过 **AskUserQuestion** 工具展示给用户，让用户选择要关联的通知规则。

### 第五步：验证

```
GET /api/n9e/busi-group/<busi_group_id>/alert-subscribes
Authorization: Bearer <token>
```

在返回列表中找到刚创建的订阅规则，向用户输出创建结果摘要。

---

## 订阅规则 Payload 结构

```json
{
  "name": "订阅规则名称",
  "note": "订阅规则备注说明",
  "disabled": 0,
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [],
  "cluster": "0",
  "rule_ids": [],
  "severities": [1, 2, 3],
  "for_duration": 0,
  "tags": [],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": []
}
```

---

## 字段说明

### 基础字段

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | 是 | 订阅规则名称 |
| `note` | string | 否 | 订阅规则备注说明 |
| `disabled` | int | 否 | 是否禁用，`0`=启用（默认），`1`=禁用 |
| `prod` | string | 否 | 产品类型，默认 `"metric"`，日志类型用 `"logging"`，机器监控用 `"host"` |
| `cate` | string | 否 | 数据源类型，如 `"prometheus"`、`"elasticsearch"`、`"loki"`、`"host"` 等 |
| `datasource_ids` | int[] | 否 | 数据源 ID 列表，空数组表示匹配全部数据源 |
| `cluster` | string | 否 | 固定填 `"0"` |

### 订阅过滤条件

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `rule_ids` | int[] | 否 | 订阅的告警规则 ID 列表，空数组表示订阅所有告警规则 |
| `severities` | int[] | 是 | 订阅的告警级别，`[1, 2, 3]` 表示全部级别 |
| `for_duration` | int | 否 | 告警持续时长过滤（秒），用于告警升级场景，`0` 表示不过滤 |
| `tags` | array | 否 | 事件标签过滤条件 |
| `busi_groups` | array | 否 | 业务组过滤条件 |

### severity 告警级别

| 值 | 含义 |
|---|---|
| 1 | 一级报警 (Critical) |
| 2 | 二级报警 (Warning) |
| 3 | 三级报警 (Info) |

### 事件标签过滤 (tags)

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

**多个 tag 之间是 AND 关系**，即告警事件必须同时匹配所有 tag 条件才会被订阅。

常用标签举例：
- `ident`：机器标识/主机名
- `rulename`：告警规则名称
- `__name__`：指标名称
- 自定义业务标签

### 业务组过滤 (busi_groups)

`busi_groups` 是一个数组，用于按告警事件所属业务组过滤，每个元素结构：

```json
{
  "key": "groups",
  "func": "匹配操作符",
  "value": "匹配值"
}
```

`key` 固定为 `"groups"`，`func` 和 `value` 的用法同 tags 过滤。

多个 busi_groups 条件之间也是 AND 关系。

### 通知配置

通过关联通知规则来配置告警通知：

| 字段 | 类型 | 说明 |
|---|---|---|
| `notify_version` | int | 固定为 `1` |
| `notify_rule_ids` | int[] | 关联的通知规则 ID 列表 |

---

## 完整示例

### 示例一：使用通知规则订阅所有 Critical 告警（推荐方式）

```json
{
  "name": "订阅所有 Critical 告警",
  "note": "订阅所有严重告警事件，通过通知规则转发",
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

### 示例二：按标签过滤订阅特定机器的告警

```json
{
  "name": "订阅 web 集群告警",
  "note": "订阅所有 web 开头的机器的告警",
  "disabled": 0,
  "prod": "metric",
  "cate": "prometheus",
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

### 示例三：订阅指定告警规则并配置告警升级

```json
{
  "name": "订阅 CPU 告警并升级通知",
  "note": "订阅 CPU 相关告警规则，持续 5 分钟后触发升级通知",
  "disabled": 0,
  "prod": "metric",
  "cate": "prometheus",
  "datasource_ids": [1],
  "cluster": "0",
  "rule_ids": [10, 11],
  "severities": [2, 3],
  "for_duration": 300,
  "tags": [],
  "busi_groups": [],
  "extra_config": {},
  "notify_version": 1,
  "notify_rule_ids": [1, 2]
}
```

### 示例四：按业务组和标签组合过滤订阅

```json
{
  "name": "订阅生产环境数据库告警",
  "note": "订阅生产业务组下数据库相关的告警事件",
  "disabled": 0,
  "prod": "metric",
  "cate": "prometheus",
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

### 示例五：订阅告警并通过通知规则转发

```json
{
  "name": "告警转发到工单系统",
  "note": "订阅所有 Warning 以上告警，通过通知规则转发到内部工单系统",
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

---

## 关键注意事项

1. **创建 API 接收对象而非数组**：与告警规则不同，订阅规则 payload 是单个对象 `{...}`，不是数组
2. **name 字段是订阅规则名称**：用于标识订阅规则，note 是备注说明
3. **notify_version 固定为 1**：通过 `notify_rule_ids` 关联通知规则
4. **severities 不能为空**：至少指定一个告警级别
5. **datasource_ids 为空数组匹配全部数据源**：如不限制数据源，设为 `[]`
6. **rule_ids 为空数组匹配所有告警规则**：如不限制具体规则，设为 `[]`
7. **tags 和 busi_groups 为空数组时不过滤**：匹配所有标签和所有业务组
8. **in/not in 的 value 用空格分隔**：如 `"web01 web02 web03"`，不要用逗号
9. **for_duration 用于告警升级**：设置持续时长过滤，避免新产生的告警在指定时间内被重复订阅
10. **cluster 字段**：固定填 `"0"`
11. **prod 和 cate 可为空字符串**：表示不限制产品类型和数据源类型
