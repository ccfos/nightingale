---
name: n9e-generate-message-template
description: 生成或修改夜莺(n9e)告警通知消息模板。当用户要求写通知模板、改消息格式、加主机名/恢复值/级别、钉钉/飞书/Lark/邮件/短信/电话模板时使用。
---

# 夜莺(n9e) 通知消息模板生成

夜莺的消息模板是 Go `text/template` / `html/template` 语法（邮件走 `text/template`，其他走 `html/template` 再做转义）。用户在「通知管理 → 消息模板」页面编辑，保存后被通知规则引用，触发告警时按渲染数据替换变量后发送到各通道。

本技能专注于**写/改模板片段本身**，不涉及创建通知规则或通道配置。

---

## 渲染上下文

后端执行时传入的 `renderData`：

| key | 类型 | 说明 |
|---|---|---|
| `.events` | `[]*AlertCurEvent` | 当前批次的告警事件列表，通常只有 1 条 |
| `.domain` | `string` | n9e 站点 URL，用于拼跳转链接 |

**自动注入的简写**（不用手写）：

```gotemplate
{{ $events := .events }}
{{ $event := index $events 0 }}
{{ $labels := $event.TagsMap }}
{{ $value := $event.TriggerValue }}
```

所以模板里可以直接使用 `$event.xxx`、`$labels.agent_hostname`、`$value`。

---

## `$event` 可用字段（`*AlertCurEvent`）

### 常用字段
| 字段 | 类型 | 说明 |
|---|---|---|
| `.Id` | int64 | 告警事件 ID（拼跳转链用） |
| `.RuleId` | int64 | 告警规则 ID |
| `.RuleName` | string | 规则名称 |
| `.RuleNote` | string | 规则备注 |
| `.Severity` | int | 告警级别：1=Critical, 2=Warning, 3=Info |
| `.PromQl` | string | 告警触发表达式 |
| `.RuleAlgo` | string | 规则算法类型 |
| `.TriggerTime` | int64 | 触发时间 unix 秒 |
| `.TriggerValue` | string | 触发时的指标值（已为字符串） |
| `.FirstTriggerTime` | int64 | 首次异常时间（连续告警） |
| `.LastEvalTime` | int64 | 最近一次评估时间，**恢复时作为恢复时间使用** |
| `.IsRecovered` | bool | 是否已恢复 |
| `.NotifyCurNumber` | int | 本次通知是第几次发送 |
| `.TargetIdent` | string | 监控对象（通常是 agent_hostname） |
| `.TargetNote` | string | 对象备注 |
| `.GroupId` / `.GroupName` | int64 / string | 业务组 |
| `.Cluster` | string | 数据源集群名 |
| `.Cate` | string | 数据源类型（prometheus / host / mysql / ...） |
| `.RunbookUrl` | string | 运行手册链接 |

### 标签 / 注解 / 触发值对象
| 字段 | 类型 | 说明 |
|---|---|---|
| `.TagsMap` | `map[string]string` | 事件标签，优先用：`{{$labels.agent_hostname}}` |
| `.TagsJSON` | `[]string` | 形如 `["k=v", ...]` 的字符串数组（早期模板常用） |
| `.AnnotationsJSON` | `map[string]string` | 注解，常用 `.AnnotationsJSON.recovery_value` |
| `.TriggerValuesJson.ValuesWithUnit` | map | 带单位的触发值（多值场景） |

### 通知对象
| 字段 | 类型 | 说明 |
|---|---|---|
| `.NotifyUsersObj` | `[]*User` | 本次要通知的用户对象（有 Username/Nickname/Phone/Email） |
| `.NotifyGroupsObj` | `[]*UserGroup` | 关联用户组 |

> 用户对象在钉钉/飞书/Lark 模板里常用来拼 at，见下文 `ats` 系列 helper。

---

## 可用 helper 函数（`tplx.TemplateFuncMap`）

### 时间
- `timeformat <unix>` — 默认 `"2006-01-02 15:04:05"`；传第二个参数覆盖：`{{timeformat $event.TriggerTime "15:04:05"}}`
- `timestamp` — 当前时间字符串
- `now.Unix` — 当前 unix 秒（`now` 是 Go template 内置）
- `humanizeDuration <sec>` — `"3m15s"`
- `humanizeDurationInterface <interface>` — 同上但接受 interface
- `toTime <unix>` — 返回 `time.Time`，可链式调用
- `parseDuration <"5m">` — 返回 `time.Duration`

### 数值 / 格式
- `formatDecimal <v> <n>` — 保留 n 位小数（`{{formatDecimal $event.TriggerValue 2}}`）
- `humanize <v>` — K/M/G 单位（1000 进制，SI）
- `humanize1024 <v>` — Ki/Mi/Gi（1024 进制）
- `humanizePercentage <v>` / `humanizePercentageH <v>` — 百分比
- `add / sub / mul / div <a> <b>` — 四则
- `printf "%.2f" <v>` — Go `fmt.Sprintf`

### 字符串
- `toUpper / toLower / title` — 大小写
- `contains <s> <sub>` — 子串
- `match <regex> <s>` — 正则匹配（bool）
- `reReplaceAll <regex> <repl> <s>` — 正则替换
- `split <s> <sep>` / `join <slice> <sep>`
- `stripPort <host:port>` / `stripDomain <host.domain>`
- `b64enc` / `b64dec` — base64

### 链接 / 转义
- `escape <s>` — URL 路径转义
- `unescaped <s>` — 输出 raw HTML（不转义）
- `safeHtml <s>` — 同上
- `urlconvert <s>`

### 标签 / 触发值
- `label <key> <labelMap>` / `value <key> <m>` / `strvalue <v>`
- `first <slice>` — 取第一个
- `tagsMapToStr <map>` — 标签拼成 `k=v,k=v`
- `sortByLabel <items> <key>` — 按标签排序

### @人（钉钉/飞书/Lark 专用）
- `ats <users> <platform>` — 生成 at 片段
- `batchContactsAts <contacts> <platform>` — 批量 at
- `batchContactsAtsInFeishuEmail <contacts>` / `batchContactsAtsInFeishuId <contacts>` — 飞书专用
- `batchContactsJoinComma <contacts>` / `batchContactsJsonMarshal <contacts>`
- `mappingAndJoin <map> <kvSep> <itemSep>`

### 其它
- `jsonMarshal <v>` — 序列化为 JSON 字符串
- `mapDifference <a> <b>` — 集合差

---

## 各通道（`notify_channel_ident`）语法差异

消息模板绑在具体通道上，通道 ident 决定文本引擎和转义行为：

| Ident | 引擎 | 说明 |
|---|---|---|
| `email` | `text/template` | 不再次转义，HTML 模板直接写标签 |
| `slackwebhook` / `slackbot` | `html/template` | 渲染后把 `"` / `\n` 转义并包成 `template.HTML`；通常写 Markdown |
| 其它（`dingtalk` / `feishu` / `feishucard` / `larkcard` / `wecom` / `tx-sms` / `ali-voice` …） | `html/template` | 渲染后对 `"` `\n` `\r` 做 JSON 字符串转义，适合直接塞入 webhook payload |

> 重要：`html/template` 会对 `<` `>` `&` 自动转义。**不想转义的地方要用 `{{unescaped "…"}}` 或 `{{safeHtml .X}}`**，否则钉钉里渲染出来会看到字面 `&lt;`。

---

## 写模板的工作流

1. **确认通道 ident**：用户说"钉钉"就是 `dingtalk`，"飞书卡片"是 `feishucard`，"邮件"是 `email`。不同通道风格差异大。
2. **判断是否需要渲染恢复状态**：默认要分 `$event.IsRecovered` 两个分支。只有短信/语音可以省略。
3. **挑字段**：
   - 标签优先用 `$labels.<key>`，而不是 `$event.TagsJSON`。
   - 触发值用 `$event.TriggerValue`；要两位小数就 `{{formatDecimal $event.TriggerValue 2}}`。
   - 时间戳一律过 `timeformat`。
   - 跳转链接拼 `{{.domain}}/share/alert-his-events/{{$event.Id}}`。
4. **级别显示**：
   - 数字：`{{$event.Severity}}`
   - 中文：`{{if eq $event.Severity 1}}一级{{else if eq $event.Severity 2}}二级{{else}}三级{{end}}`
   - 英文：`Critical / Warning / Info`
5. **@人（钉钉/飞书）**：
   - 钉钉 `@全员`：在模板末尾加一行 `@all`（钉钉按空格分词匹配）。精确 at 手机号：`{{range $event.NotifyUsersObj}}@{{.Phone}} {{end}}`。
   - 飞书卡片：用 `{{batchContactsAtsInFeishuEmail $event.NotifyUsersObj}}` 拼出 `<at email=...></at>` 片段。
6. **输出**：用 markdown 代码块 ``` ```gotemplate ``` 包裹模板主体，末尾给一段**变量/函数说明**，每个非平凡变量/函数单独一行。

---

## 输出格式

回复给用户时：

1. 一到两句导语说明模板用途、适配哪个通道。
2. ```gotemplate
   <模板内容>
   ```
3. `**变量说明**`：列出模板里用到的非平凡字段和函数（`$event.TargetIdent`、`formatDecimal`、`timeformat` 之类），每项一行。
4. 如果用户的需求有歧义（比如"加主机名"——到底是 `target_ident` 还是某个 tag），**在导语里直接点出做了什么假设**，不要反问。

语言跟随用户输入（中文输入就用中文）。

---

## 内置参考模板（改造起点）

### 钉钉 markdown（完整版）

```gotemplate
#### {{if $event.IsRecovered}}<font color="#008800">💚{{$event.RuleName}}</font>{{else}}<font color="#FF0000">💔{{$event.RuleName}}</font>{{end}}
---
{{$duration := sub now.Unix $event.FirstTriggerTime}}{{if $event.IsRecovered}}{{$duration = sub $event.LastEvalTime $event.FirstTriggerTime}}{{end}}
- **告警级别**: S{{$event.Severity}}
{{- if $event.RuleNote}}
- **规则备注**: {{$event.RuleNote}}
{{- end}}
{{- if $event.TargetIdent}}
- **监控对象**: {{$event.TargetIdent}}
{{- end}}
{{- if not $event.IsRecovered}}
- **触发时值**: {{$event.TriggerValue}}
- **触发时间**: {{timeformat $event.TriggerTime}}
- **持续时长**: {{humanizeDurationInterface $duration}}
{{- else}}
- **恢复时间**: {{timeformat $event.LastEvalTime}}
- **持续时长**: {{humanizeDurationInterface $duration}}
{{- end}}
- **事件标签**:
{{- range $k, $v := $labels}}
{{- if ne $k "rulename"}}
    - {{$k}}: {{$v}}
{{- end}}
{{- end}}
[事件详情]({{.domain}}/share/alert-his-events/{{$event.Id}}) | [屏蔽1小时]({{.domain}}/alert-mutes/add?__event_id={{$event.Id}})
```

### 飞书卡片（简洁版）

```gotemplate
{{- if $event.IsRecovered}}
**级别状态:** S{{$event.Severity}} Recovered
**告警名称:** {{$event.RuleName}}
**事件标签:** {{$event.TagsJSON}}
**恢复时间:** {{timeformat $event.LastEvalTime}}
{{- else}}
**级别状态:** S{{$event.Severity}} Triggered
**告警名称:** {{$event.RuleName}}
**事件标签:** {{$event.TagsJSON}}
**触发时间:** {{timeformat $event.TriggerTime}}
**触发时值:** {{$event.TriggerValue}}
{{- if $event.RuleNote}}
**告警描述:** {{$event.RuleNote}}
{{- end}}
{{- end}}
```

### 短信 / 语音（极简）

```gotemplate
级别状态: S{{$event.Severity}} {{if $event.IsRecovered}}Recovered{{else}}Triggered{{end}} 规则: {{$event.RuleName}} 对象: {{$event.TargetIdent}}
```

---

## 典型改造场景

### 1) "在钉钉模板里加主机名"

```gotemplate
- **主机**: {{$event.TargetIdent}}
{{- if $labels.ip}}
- **IP**: {{$labels.ip}}
{{- end}}
```

> 说明：`target_ident` 一般就是主机名；如果需要 IP，优先看标签 `ip`、`instance`、`host`。

### 2) "把 trigger_value 保留两位小数"

```gotemplate
- **触发时值**: {{formatDecimal $event.TriggerValue 2}}
```

> `TriggerValue` 本身是字符串，`formatDecimal` 会先转浮点再格式化，非数字会原样返回。

### 3) "钉钉模板末尾 @ 告警接收人"

```gotemplate
...模板正文...

{{- range $event.NotifyUsersObj}}@{{.Phone}} {{end}}
```

> 钉钉按 "空格 + 手机号" 识别被 at 的用户。或者统一 `@all`。

### 4) "恢复时显示恢复时的值"

```gotemplate
{{- if $event.IsRecovered}}
{{- if $event.AnnotationsJSON.recovery_value}}
- **恢复时值**: {{formatDecimal $event.AnnotationsJSON.recovery_value 4}}
{{- end}}
- **恢复时间**: {{timeformat $event.LastEvalTime}}
{{- end}}
```

> 恢复值由告警引擎在恢复时写入 `AnnotationsJSON.recovery_value`，**仅在有恢复值的情况下存在**，用 `if` 保护。

### 5) "告警级别用中文"

```gotemplate
- **级别**: {{if eq $event.Severity 1}}一级（紧急）{{else if eq $event.Severity 2}}二级（重要）{{else}}三级（提示）{{end}}
```

### 6) "只发生产环境告警相关机器"

模板本身不做过滤——过滤应放在**通知规则**的 `attributes` / `label_keys`。模板里只负责展示。如果用户问到这里要提示一下。

---

## 关键注意事项

1. **`html/template` 会 HTML 转义**。钉钉/飞书/Lark 里 `<font>`、`<at>` 等标签**必须用 `unescaped` 包裹或放在内容里由 Go template 自己识别**。邮件走 `text/template` 不受此限。
2. **`TriggerValue` 是字符串**：直接比较大小要用 `parseDuration`/自定义，常规只做显示就好。
3. **不要手动写 `{{$events := .events}}` 等头部**——系统会自动注入。
4. **恢复分支不能漏**：`NotifyRecovered=1` 的规则会复用同一模板发恢复消息。
5. **标签 key 里的中划线/点**：`$labels.agent_hostname` 能用，但 `$labels.app-name` 不行，要用 `index $labels "app-name"`。
6. **时间字段全部是 unix 秒**（`int64`），不要直接 `{{$event.TriggerTime}}` 当文本，用 `timeformat` 或 `toTime`。
7. **多告警合并**：`.events` 可能有多条，钉钉/飞书默认模板只渲染第 0 条（`$event`）；如果用户要批量展示，用 `{{range .events}}`。
8. **链接用 `.domain`**：不要硬编码 `http://localhost:17000`，否则切环境就坏。

---

## 常见错误

- ❌ `{{$event.TriggerValue | printf "%.2f"}}` — `printf` 第一个参数才是 format，且 `TriggerValue` 是 string。
- ✅ `{{formatDecimal $event.TriggerValue 2}}`

- ❌ `{{$event.TriggerTime}}` 直接展示 — 会输出 unix 秒。
- ✅ `{{timeformat $event.TriggerTime}}`

- ❌ 钉钉里写 `<font color="red">` 不转义 — `html/template` 会转义成 `&lt;font&gt;`。
- ✅ 放在条件分支里（Go template 的 `{{if}}` 分支字面量不会被 HTML 转义内部标签），或整体 `{{unescaped "…"}}`。夜莺官方钉钉模板直接写 `<font>` 能生效，是因为外层是纯文本区段（不在属性值里）——遵循官方样例即可。

- ❌ 忘记分恢复分支 — 恢复通知里触发时间误导人。
- ✅ 所有动态内容先用 `{{if $event.IsRecovered}}…{{else}}…{{end}}` 包好。
