---
name: n9e-notify-channel-copilot
description: 帮助用户修改、新建或排障夜莺(n9e)通知媒介(notify_channel)。当用户要求改钉钉/飞书/企微/邮件/短信/语音/Webhook 等媒介的 URL、请求体、签名、headers、代理、TLS、@人或接收人字段，或问"怎么接入 X 平台"、"为什么发不出去/报 9499/Bad Request"时使用。本技能专注**媒介通道层配置**——若用户改的是"消息内容/字段/渲染"，应改用 n9e-generate-message-template。
tags:
  - internal
---

# 夜莺(n9e) 通知媒介修改

## 适用范围（先确定用户在改哪一层）

夜莺通知链路分三层，每层痛点不同：

| 层 | 实体 | 关键文件 | 本 skill 是否管 |
|---|---|---|---|
| **媒介** Notify Channel | `notify_channel` 表，`NotifyChannelConfig` | `models/notify_channel.go`、`alert/sender/provider/*.go` | **是** |
| 消息模板 Notify Template | `notify_tpl` 表 | `models/notify_tpl.go` | 否（用 `n9e-generate-message-template`） |
| 通知规则 Notify Rule | `notify_rule` 表 | `models/notify_rule.go` | 否 |

**判断口径**：
- 用户原话出现"URL/Webhook 地址/请求头/超时/代理/签名/秘钥/AppID/AppSecret/CorpID/接入"——**媒介层**，进本 skill。
- 用户原话出现"模板/正文/字段/变量/渲染/标题/卡片颜色"——**模板层**，转 `n9e-generate-message-template`。
- 用户原话出现"发给谁/接收人/订阅/过滤/标签匹配"——**规则层**，不在本 skill 范围。

---

## 数据模型 `NotifyChannelConfig`

`models/notify_channel.go`:

```go
type NotifyChannelConfig struct {
    ID          int64
    Name        string                // 显示名
    Ident       string                // 媒介类型（路由 provider 的 key，下表）
    Description string
    Enable      bool

    ParamConfig   *NotifyParamConfig   // 用户参数：contact_key + 自定义 params
    RequestType   string               // http | smtp | script | flashduty | pagerduty
    RequestConfig *RequestConfig       // 按 ident/request_type 取对应子结构

    Weight int
}
```

`RequestConfig` 是 union：按媒介类型只填其中一个字段：

| 字段 | 适用 ident |
|---|---|
| `HTTPRequestConfig` | 所有走纯 HTTP webhook 的媒介（dingtalk、feishu、wecom、telegram、slackwebhook、callback、…） |
| `SMTPRequestConfig` | `email` |
| `ScriptRequestConfig` | `script` |
| `FlashDutyRequestConfig` | `flashduty` |
| `PagerDutyRequestConfig` | `pagerduty` |
| `DingtalkAppRequestConfig` | `dingtalkapp`（钉钉应用，目前未注册，见 `provider/init.go`） |
| `FeishuAppRequestConfig` | `feishuapp` |
| `WecomAppRequestConfig` | `wecomapp` |

---

## 内置 ident 一览（`models/user.go` + `alert/sender/provider/init.go`）

| Ident | Provider | Request 类型 | 一句话 |
|---|---|---|---|
| `dingtalk` | DingtalkProvider | HTTP | 钉钉群机器人 webhook |
| `wecom` | WecomProvider | HTTP | 企业微信群机器人 webhook |
| `feishu` | simpleHTTPProvider | HTTP | 飞书群机器人 markdown（早期） |
| `feishucard` | FeishuCardProvider | HTTP | 飞书消息卡片（支持切色/@人） |
| `lark` | simpleHTTPProvider | HTTP | Lark（国际版飞书）markdown |
| `larkcard` | LarkCardProvider | HTTP | Lark 卡片 |
| `feishuapp` | FeishuAppProvider | HTTP (App) | 飞书应用机器人（私聊/群） |
| `wecomapp` | WecomAppProvider | HTTP (App) | 企微自建应用 |
| `telegram` | simpleHTTPProvider | HTTP | Telegram Bot |
| `discord` | simpleHTTPProvider | HTTP | Discord webhook |
| `slackbot` / `slackwebhook` | simpleHTTPProvider | HTTP | Slack |
| `mattermostbot` / `mattermostwebhook` | simpleHTTPProvider | HTTP | Mattermost |
| `jira` / `jsm_alert` | simpleHTTPProvider | HTTP | Jira / JSM 工单类 |
| `email` | EmailProvider | SMTP | 邮件 |
| `tx-sms` | TencentSmsProvider | HTTP | 腾讯云短信 |
| `tx-voice` | TencentVoiceProvider | HTTP | 腾讯云语音 |
| `ali-sms` | AliyunSmsProvider | HTTP | 阿里云短信 |
| `ali-voice` | AliyunVoiceProvider | HTTP | 阿里云语音 |
| `pagerduty` | PagerDutyProvider | HTTP | PagerDuty |
| `flashduty` | FlashDutyProvider | HTTP | Flashduty 集成 |
| `script` | ScriptProvider | Script | shell/python 脚本（兼容老 notify.py） |
| `callback` | CallbackProvider | HTTP | 通用 HTTP 回调 |

**注册机制**：`provider/registry.go` 的 `Resolve()`：先按 `Ident` 精确查 → 找不到按 `RequestType` 兜底到通用 provider（`callback`/`script`/`email`/`flashduty`/`pagerduty`）。所以**自定义 ident（如 `my-internal-webhook`）只要 `request_type=http` 就能用 callback 兜底**。

---

## `HTTPRequestConfig` 字段详解

```go
type HTTPRequestConfig struct {
    URL           string                 // 完整 URL，可含 {{ ... }} 模板变量
    Method        string                 // GET | POST | PUT
    Headers       map[string]string      // header 值也可含模板变量
    Proxy         string                 // 形如 http://proxy:port，留空走直连
    Timeout       int                    // 毫秒，默认 10000
    Concurrency   int                    // 并发，默认 5
    RetryTimes    int                    // 默认 3
    RetryInterval int                    // 毫秒，默认 100
    TLS           *TLSConfig             // {Enable, CertFile, KeyFile, CAFile, SkipVerify}
    Request       RequestDetail          // {Parameters: query string, Form, Body}
}
```

**Body 字段是字符串**，里面用 Go template 语法引用事件，**真实数据按 `n9e-generate-message-template` 那套字段字典写**（`$event.RuleName`、`$labels.ident`、`timeformat`、`unescaped` 等）。Body 渲染走 `html/template`，所以 `<`、`&` 会被转义——JSON Body 通常不受影响，但**模板里写 HTML 标签时要 `{{unescaped "<b>..."}}`**。

**URL / Headers / Parameters 同样走模板渲染**（`alert/sender/provider/http_common.go:113-142 replaceVariables`）。关键细节：
- 只有含 `{{` 才会走 `html/template`，否则原样保留（`needsTemplateRendering` 先过滤）。
- 上下文与 Body 共享，是下一节那 6 个变量。

例：
- URL 按级别分路由：`http://bot/notify?level={{$event.Severity}}`
- Header 注入鉴权：`Authorization: Bearer {{$params.token}}`
- 把所有接收人手机号串到 query：
  `?ats={{range $i,$s := $sendtos}}{{if $i}},{{end}}{{$s}}{{end}}`

---

## 模板上下文 — 你能用的变量

所有 HTTP 类 provider 共用 `alert/sender/provider/http_common.go:SendHTTPRequest`，渲染 Body / URL / Headers / Parameters 时统一注入这 6 个变量（line 34-44）：

| 变量 | 来源 | 典型用途 |
|---|---|---|
| `$event` | `events[0]`，本批次第一条 `AlertCurEvent` | `{{$event.RuleName}}` / `{{$event.Severity}}` / `{{$event.TriggerValue}}` |
| `$events` | `[]*AlertCurEvent` 整批 | callback 默认模板 `{{ jsonMarshal $events }}` |
| `$sendtos` | `[]string`，按媒介的 `ContactKey` 从接收人 `contact_info` 解出 | `{{range $sendtos}}...{{end}}`、`{{ jsonMarshal $sendtos }}` |
| `$sendto` | `sendtos[0]`，仅当 `len(sendtos)>0` 时存在 | 单人渲染 |
| `$params` | 媒介 `ParamConfig.Custom.Params` | 短信模板的 `SignName` / `TemplateId` |
| `$tpl` | 通知模板里挂的自定义字段 | 模板与媒介解耦时用 |

此外 `$event.NotifyUsersObj` 也是合法的：`[]*User` 数组，包含本批次全部接收人完整 User 对象（`models/alert_cur_event.go:65` + `alert/dispatch/dispatch.go:971`）。原是 v6 notify.py 时代留的字段（`gorm:"-"`，runtime-only），v8 仍在填充。和 `$sendtos` 的区别：

- `$sendtos`：纯字符串数组，已按 ContactKey 解析好，**简单场景首选**。
- `$event.NotifyUsersObj`：完整 User 对象数组，模板里同时要拿 Phone+Email+Username 等多字段时用，如 `{{range $event.NotifyUsersObj}}{{.Phone}} {{.Username}}{{end}}`。

**`$sendtos` 是怎么来的**（`alert/dispatch/dispatch.go:451-541 GetNotifyConfigParams`）：
拿到 notify_rule 这条 NotifyConfig 的 `user_ids` + `user_group_ids` → 查 UserCache → 按媒介的 `ParamConfig.UserInfo.ContactKey` 从每个 user 的 `contact_info` JSON 取值 → 去重 → 组 `[]string`。

⚠️ 因此：
- `ContactKey=phone` 但用户 `contact_info.phone` 为空 → 此人不进 sendtos。
- `ContactKey=dingtalk_robot_token` 这种自定义键，用户 `contact_info` JSON 里也要有同名键。否则 `$sendtos` 是空数组，`$sendto` 直接未定义。
- 这是「测试通知 OK 但真实告警发不出去」最常见的根因（见调试章节"测试 OK 但实际告警发不出去"专项）。

---

## 各媒介的"必填字段地图"

下面按媒介给出最小可用配置——用户问"怎么接入 X"时，直接告诉他填这几格。

### 1) 钉钉群机器人 `dingtalk`

- `request_type=http`，`HTTPRequestConfig`:
  - `URL`: 群机器人 webhook（钉钉后台 → 群设置 → 智能群助手 → 添加机器人 → 自定义）
  - `Method`: `POST`
  - `Headers`: `Content-Type: application/json`
  - `Body`: JSON markdown，参考 `provider/dingtalk_provider.go`。`text` 字段里要含**关键词**（钉钉群机器人加白校验：你在钉钉建机器人时填的关键字必须出现在消息文本中）。
- **加签 vs 关键字**：n9e 内置 dingtalk provider **只支持关键字校验**，加签机制需要在 URL query 拼 `&timestamp=&sign=`，目前没有原生开关。如果用户必须用加签：
  1. 用 `callback` 媒介自己拼带签名的 URL（写在 `URL` 里渲染时不好做，需要前置脚本），或
  2. 改用 `script` 媒介调脚本。
  3. **推荐说法**："钉钉机器人请改用关键字校验；加签机制开源版未原生支持。"
- **@人**：依赖消息体中的 `at.atMobiles` / `atUserIds` 数组。内置 dingtalk provider 的请求体由消息模板拼出，**模板里用 `{{batchContactsAts}}` 或自己 range `$event.NotifyUsersObj` 取 `.Phone`**。

### 2) 企业微信群机器人 `wecom`

- `URL`: 群机器人 webhook
- `Method`: `POST`, `Content-Type: application/json`
- **限制**：群机器人 markdown 不支持 `<font color>`；@人靠 `mentioned_mobile_list`/`mentioned_list`；**新版企业微信已下线群机器人**，新建群没 webhook 地址了——这时只能改走 `wecomapp` 自建应用。

### 3) 企微自建应用 `wecomapp`

- `request_type=http`，但用 `WecomAppRequestConfig`:
  - `CorpID`, `CorpSecret`, `AgentID`（企业微信管理后台拿）
  - `Proxy` / `Timeout` / `RetryTimes` / `RetrySleep`
- 走 `provider/wecomapp_provider.go`，自动管 access_token 刷新。
- **接收人字段**：用户的 `contact_info.wecom_userid`（在用户管理里填）。

### 4) 飞书群机器人（markdown）`feishu`

- 走 `simpleHTTPProvider`，模板驱动。`HTTPRequestConfig.URL` 填群机器人 webhook，`Body` 填 JSON 模板。
- **签名校验**：飞书群机器人秘钥（secret）和钉钉加签一样属于"消息体内嵌时间戳+签名"。**n9e 内置 feishu provider 同样没有自动签名**。处理方案：
  1. 群机器人创建时**不勾选「签名校验」**，改用「自定义关键词」或「IP 白名单」。
  2. 若必须用签名 → 自定义 `script` 媒介。
- **反斜杠 Bad Request 9499**：飞书 webhook 接收的是 JSON，反斜杠 `\` 是 JSON 转义字符。Windows 路径 `D:\foo`、`device="D:"` 这种标签值如果直接进 body 会破坏 JSON。**模板里用 `{{$labels.path | jsonMarshal}}` 把字符串转成合法 JSON 字符串**（带引号），或在 PromQL/规则源头用 `label_replace` 把 `\` 替换掉。

### 5) 飞书卡片 `feishucard` / Lark 卡片 `larkcard`

- 走 `FeishuCardProvider` / `LarkCardProvider`，发飞书 v2 消息卡片。
- 配置项跟 `feishu` 一样（URL + 可选 secret）。`Body` 是完整的 `interactive` 卡片 JSON。
- **卡片切色**：飞书卡片只认枚举色（`red / orange / yellow / green / turquoise / blue / indigo / purple / carmine / grey`），写 hex 无效。颜色写在 `header.template` 字段，由模板根据 `IsRecovered / Severity` 渲染。
- **@人**：用 `<at email=...></at>` 或 `<at id=...></at>`；模板用 `{{batchContactsAtsInFeishuEmail $event.NotifyUsersObj}}` 或 `{{batchContactsAtsInFeishuId ...}}`。

### 6) 飞书自建应用 `feishuapp`

- 用 `FeishuAppRequestConfig`:
  - `AppID`, `AppSecret`
  - `ReceiveIDType`: `open_id` / `user_id` / `email` / `chat_id`（决定 `contact_info.feishu_*` 用哪个字段）
- 走 `provider/feishuapp_provider.go`，自动管 tenant_access_token。

### 7) 邮件 `email`

- `request_type=smtp`，`SMTPRequestConfig`:
  - `Host`, `Port`, `Username`, `Password`, `From`
  - `InsecureSkipVerify`: 自签证书时设 true
  - `Batch`: 一次最多塞几个收件人（防止超过 SMTP 服务器单次收件上限）
- **邮件标题模板**：单独存在 `notify_tpl` 表，ident 是 `mailsubject`（参 `EmailSubject` 常量）。`#2220 标题包含所有标签会泄漏信息` 的修复路径就是改这个模板，不动 channel。
- **HTML vs 纯文本**：邮件正文模板走 `text/template`（不转义），所以可以直接写 HTML 标签。其它 IM 类是 `html/template`，要 `unescaped` 兜底。

### 8) 短信/语音（腾讯云/阿里云）`tx-sms` / `tx-voice` / `ali-sms` / `ali-voice`

- 共同结构：用 `HTTPRequestConfig`，但**真实凭证靠 `ParamConfig.Custom.Params` 自定义参数填**（SecretId / SecretKey / SDKAppId / TemplateId / SignName 等）。
- **模板变量缺失报错**："测试通知显示模板变量缺少对应参数值"——短信模板的参数顺序/数量必须和服务商后台审批通过的模板**严格一致**。
  - 排查路径：① 服务商后台 → 找到 TemplateId → 看模板内容有几个 `${1}` `${2}`；② n9e 模板里 `params` 数组要按这个数量填；③ 字段名/顺序要对齐。
- **中文乱码（语音/回调）**：n9e 默认按 UTF-8 编码 body，部分语音服务商接口要求 GBK 或 url-encode 中文——参数里走 `{{$event.RuleName | escape}}` 试试，或在脚本媒介里转码。

### 9) PagerDuty `pagerduty`

- `PagerDutyRequestConfig`: `Proxy`, `ApiKey`（账户级 API Key，不是 routing key），`Timeout`, `RetryTimes`, `RetrySleep`。
- 走 PagerDuty Events API v2。**ApiKey 别填错成 Integration Key**（社区高频踩坑）。

### 10) Flashduty `flashduty`

- `FlashDutyRequestConfig`: `IntegrationUrl`（一个集成一个 URL）, `Proxy`, `Timeout`, `RetryTimes`, `RetrySleep`。
- 商业版 Flashduty 提供的集成入口，模板由 Flashduty 后端处理，n9e 这边几乎是"透传 events 数组"。

### 11) Script `script`

- `ScriptRequestConfig`:
  - `ScriptType`: `python` / `shell`
  - `Script`: 脚本内容（运行时写入临时文件再执行）
  - `Path`: 也可以直接给已存在的脚本路径
  - `Timeout`: 毫秒
- **告警数据通过 stdin 以 JSON 形式传入**——脚本里读 stdin 解析。
- **历史包袱**：v6 时代的 `notify.py` 就是这条路径的兜底——任何 IM/系统的怪需求（自定义签名、私有协议、复杂 at 逻辑）最后都能用 script 兜住。

### 12) Callback `callback`（通用 HTTP）

- 任何"打 HTTP 把事件 JSON 发过去"的场景都走它。
- `HTTPRequestConfig.Body` 默认模板是 `{{ jsonMarshal $events }}`（注意是 `$events` 复数，整批传过去）。
- **自定义 ident 兜底也走 callback**：`my-custom-webhook` 这种 ident 只要 `request_type=http` 就能跑（见 `Registry.Resolve`）。

⚠️ **v6/v7 升级上来的用户最常踩**：`$sendtos` 已经在模板上下文里自动注入（`http_common.go:36`），但 Callback **默认 Body 模板 `{{ jsonMarshal $events }}` 只输出 events**，并没引用 sendtos。下游 jenkins / 外呼 / 自愈脚本想拿"本次通知的接收人联系方式"会拿不到。改法是把 Body 显式加上：

```json
{
  "events": {{ jsonMarshal $events }},
  "sendtos": {{ jsonMarshal $sendtos }}
}
```

不是上下文里没有，是默认模板没引用。

---

## 修改通知媒介的三条路径

### 路径 A：UI（推荐）
- 路径：`系统配置 → 通知配置 → 通知媒介` → 选媒介 → 编辑
- 适用：90% 场景。改 URL、改 timeout、改 body、改 headers、加自定义参数。
- 一个**重要坑**：UI 上"媒介类型"（即 `Ident`）一旦创建**不允许修改**。要换类型只能删除重建。

### 路径 B：HTTP API
- `POST /api/n9e/notify-channel`（新建）、`PUT /api/n9e/notify-channel/:id`（更新）、`DELETE /api/n9e/notify-channel/:id`
- 看 `center/router/router_notify_channel.go` 找具体路径和请求体格式。
- 适用：批量改、迁移、CI 灌配置。

### 路径 C：直改 DB（最后手段）
- 表 `notify_channel`，`request_config` 是 JSON 字段。
- 注意：① 改完要让 server 重新载入（n9e 每 9 秒拉一次，不用重启）；② JSON 改错会导致 Verify 失败，整条媒介不可用——改前 `mysqldump -t notify_channel > backup.sql`。

---

## 调试与排错

### 看媒介有没有真的发出请求

两层证据，从粗到细。

**第一层：`notification_record` 表**——每次媒介调用一条记录，无论成功失败：

```sql
SELECT id, target, channel, status, error_message, send_time, details
FROM notification_record
WHERE channel = '<媒介 ident>'
ORDER BY id DESC LIMIT 20;
```

- `status=success`：发出去了，对端怎么处理跟 n9e 无关。
- `status=failure`：`error_message` 通常带对端 HTTP 状态码或错误描述。
- **`details` 字段是 `varchar(2048)`**（`models/notification_record.go:22`），长 body 会被截断 —— 这时走第二层。

**第二层：center 服务日志**——`alert/sender/provider/http_common.go` 里的真实日志格式：

| 级别 | 出处 | grep 关键字 | 内容 |
|---|---|---|---|
| Info | line 55 | `url:` | 渲染后的 URL / Headers / Parameters |
| Error | line 63/69/80 | `send_http: failed` | 失败时的 url + request_body + error，**info 级即可看到** |
| Debug | line 78 | `send http request:` | 完整 req + resp + 响应 body |
| Debug | line 213 | `URL:` | 完整 `URL, Method, Headers, params, Body`（含模板渲染后内容） |

操作：
1. 失败排查 **不需要开 debug**，直接 `grep 'send_http: failed' n9e-center.log` 就能拿到 url + body + error。
2. 想看成功但内容异常的完整请求体 → 把 center log level 改 `debug`（默认 info），再 `grep -E 'send http request:|^.*URL:' n9e-center.log`。
3. Debug 日志不受 `notification_record.details` 2048 字节截断影响。

### 常见报错速查

| 现象 | 大概率原因 | 排查 |
|---|---|---|
| 飞书 `{"code":9499,"msg":"Bad Request"}` | body JSON 不合法，多半是反斜杠/未转义引号 | 模板里所有标签值用 `{{... \| jsonMarshal}}` 或确认引号都转义了 |
| 钉钉 "关键词不匹配" | 钉钉机器人开了关键字校验，但文本中没出现 | 把规则名/告警标题里固定带上关键字，或机器人加白名单 |
| 钉钉/飞书 "timestamp is invalid" / "sign not match" | 开了签名校验但 n9e 没自动签 | 改用关键字/IP 白名单，或走 script 媒介 |
| 邮件 `tls: handshake failure` | SMTP 服务器证书校验失败 | `InsecureSkipVerify: true` 或换 port（587 STARTTLS / 465 SSL） |
| `connect: i/o timeout` | 网络不通或需要代理 | `HTTPRequestConfig.Proxy` 填代理地址，并确认机器能解析 webhook 域名 |
| 短信 "模板变量缺少对应参数值" | 阿里云/腾讯云短信模板里 `${1}` 个数和 n9e 里 params 数组对不上 | 服务商后台对照模板内容，按顺序补齐 |
| 自定义 ident 保存提示 `unsupported channel` | ident 没注册且 request_type 不在 fallback 表里 | `RequestType` 必须填 `http/script/smtp/flashduty/pagerduty` 之一 |
| 一条媒介测试 OK 实际告警发不出 | sendtos 在真实告警时为空（接收人 contact_info 缺字段） / notify_rule 没选这条媒介 | 见下面"测试 OK 但实际告警发不出去"专项 |

### 测试 OK 但实际告警发不出去

社区高频问题（#2851）。根因是**测试和真实告警的 sendtos 来源不一样**：

- **测试按钮**：`POST /notify-rule/test` → 测试者在 UI 表单里直接填接收人，sendtos 由表单填的值产生。媒介本身的 URL/Body/Headers 没问题就能发出。
- **真实告警**：走 `alert/dispatch/dispatch.go GetNotifyConfigParams`，从 notify_rule 的 `user_ids` / `user_group_ids` → 查 user → 按 `ContactKey` 从 `contact_info` 取字段 → 组 sendtos。**任意一处缺失都会让 sendtos 为空**，这一路就静默不发。

排查三步：

1. 看 notify_rule 这条配置的接收人范围：
   ```sql
   SELECT user_ids, user_group_ids FROM notify_rule WHERE id=<id>;
   ```
2. 把这些 user 拉出来，看 ContactKey 对应字段是否为空：
   ```sql
   SELECT id, username, contact_info FROM users WHERE id IN (...);
   ```
   ContactKey 是 `phone` 时看 `contact_info.phone`；自定义 key（如 `dingtalk_robot_token`）同名取。
3. 走业务组 / 团队的情况，再查团队成员表：
   ```sql
   SELECT user_id FROM team_user WHERE team_id=<id>;
   ```

任意一层空 → sendtos 空 → 真实告警这一路不发，但测试发表单填了就能发。

### 端到端验证步骤

1. **媒介页"测试"按钮**：后端是 `POST /notify-rule/test`（`center/router/router_notify_rule.go:142, 172-264`），直接调 `Provider.Notify` **真实发送一条**——表单里填的接收人/标题/正文会真的过 webhook 出去到群里。
   注意这步**不调 sendtos 解析逻辑**，表单填啥就发啥。能发说明这条媒介的 URL / Headers / Body 模板 / 网络 / 凭证 / 签名都没问题；不能发就是这条媒介本身的链路有问题。
   （代码里**没有** `Provider.Check` 方法，不要在文档/口头上误导用户去找。）
2. 测试通过仍发不出 → 进上面"测试 OK 但实际告警发不出去"专项排查 sendtos。
3. 真实告警发出但内容异常 → 进上一节"看媒介有没有真的发出请求"第二层，开 debug log 抓完整请求体。

---

## "新增/复制一个媒介" 的标准动作

用户问"我想接入 Slack / 飞书加签 / 内部 HTTP 系统"，给他这套模板：

1. **选 ident**：
   - 公开常用平台（Slack/Discord/Telegram/Lark/Jira）→ 用内置 ident。
   - 私有系统/自建 HTTP 服务 → ident 随便起（如 `my-internal-bot`），`request_type=http` 即可。
   - 复杂签名/状态机/编码 → ident 自起，`request_type=script`。

2. **填 `RequestConfig`**：HTTP 类的填 `HTTPRequestConfig`，至少 `URL` + `Method` + `Headers.Content-Type` + `Body`。

3. **`Body` 模板的最小骨架**（拿钉钉 markdown 举例）：

   ```json
   {
     "msgtype": "markdown",
     "markdown": {
       "title": "{{$event.RuleName}}",
       "text": "#### {{if $event.IsRecovered}}恢复{{else}}告警{{end}}: {{$event.RuleName}}\n- 对象: {{$event.TargetIdent}}\n- 触发值: {{$event.TriggerValue}}\n- 时间: {{timeformat $event.TriggerTime}}"
     },
     "at": {
       "atMobiles": [{{range $i, $u := $event.NotifyUsersObj}}{{if $i}},{{end}}"{{$u.Phone}}"{{end}}],
       "isAtAll": false
     }
   }
   ```

4. **接收人字段** `ParamConfig.UserInfo.ContactKey`：
   - 钉钉群机器人 / 飞书群机器人 → 留空（群级别，不挑人）
   - 钉钉应用 / 飞书应用 / 企微应用 → 用 `dingtalk_userid` / `feishu_userid` / `wecom_userid`
   - 邮件 → `email`
   - 短信/语音 → `phone`
   - 完全自定义的 contact（如 `slack_user_id`）→ 自己起 key 名，去 `user` 表 `contact_info` JSON 里填值。

5. **测试 → 保存 → 通知规则选上**。

---

## 输出风格

用户问"怎么改 X" 时按这个套路答：

1. 一句话点出**改哪一层**（媒介/模板/规则）。如果用户其实在问模板/规则，先纠到对的 skill。
2. 给出**字段级指令**：动 `notify_channel.request_config.http_request_config.headers` 这种精确路径，不是泛泛"去后台改一下"。
3. 如果有内置 ident 能用，**优先内置**（feishucard 比手写 feishu webhook 强）。
4. 涉及签名/特殊编码/反斜杠这种已知坑，**直接报"踩过"**并给方案，不要让用户走一遍试错。
5. 涉及已知坑时，可引用相关 issue（如 `#2821`）作支撑。
6. 全程**不替用户改库或调 API**——只告诉他改哪个字段、怎么验证。
