---
name: n9e-import-prom-rule
description: |
  **批量导入 Prometheus 告警规则 YAML 文件**到夜莺（一次性建一组规则）。专用于处理远端 URL 或本地 YAML 文本，自动解析 `groups` / 纯 `rules` 数组 / 单条 rule 三种格式。
  ⚠️ **不要用这个 skill 做单条创建**——用户用自然语言描述一条告警需求时，请改用 n9e-create-alert-rule。
  触发：导入 / import / 批量 / URL / .yml 文件 / .yaml 文件 / awesome-prometheus-alerts / node-exporter.yml / prometheus rule file。
examples:
  - "帮我导入 https://raw.githubusercontent.com/.../node-exporter.yml"
  - "把这个 yaml 里的告警建到 n9e"
  - "导入 awesome-prometheus-alerts 的 mysql 那个文件"
  - "批量建一组 redis 的告警规则，从这个文件 ..."
builtin_tools:
  - http_fetch
  - preview_prom_rule_yaml
  - import_prom_rule_yaml
  - list_busi_groups
  - list_datasources
---

# Skill: 导入 Prometheus 告警规则

## 概述

把 Prometheus 官方 rule YAML（含 `groups`、纯 `rules` 数组、或单条规则三种形态）批量建到 n9e。典型来源：

- 远端 URL（如 GitHub raw 上的 awesome-prometheus-alerts）
- 用户直接粘贴的 YAML 文本

工作流固定四步：**抓 YAML 到文件 → 选业务组 + 数据源 → 预览 → 落库**。

⚠️ **核心约束：YAML 文件不进 LLM 上下文**。`http_fetch` 用 `save_to_file=true` 把内容写到临时文件，后续 `preview_prom_rule_yaml` 和 `import_prom_rule_yaml` 都用 `payload_file` 读这个路径。文件大小可能几 KB 到几 MB，让它进 prompt 会显著拖慢、增加成本，还容易被 LLM 截断改写。

## 可用工具

| 工具 | 作用 |
|---|---|
| `http_fetch` | GET 一个公网 URL。**`save_to_file=true` 时把正文写到临时文件，返回里只含 `file_path` 不含 body**。仅 http/https，自动拒绝内网/回环地址 |
| `preview_prom_rule_yaml` | 解析 YAML 不写库。**优先用 `payload_file` 入参**（http_fetch 返回的 file_path），避免大文件进上下文。返回每条规则的 name/severity/prom_ql 等用于让用户确认 |
| `import_prom_rule_yaml` | 解析 YAML 并按业务组+数据源批量落库。**优先用 `payload_file`**。返回每条规则的 id 或 error |
| `list_busi_groups` | 查可见的业务组，让用户挑 `group_id` |
| `list_datasources` | 查数据源，按 `plugin_type=prometheus` 过滤 |

## 执行步骤

### 第一步：抓 YAML 到临时文件

**用户给了 URL** —— 一定带 `save_to_file=true`：

```
http_fetch(url="https://.../node-exporter.yml", save_to_file=true)
```

返回是：
```json
{"status_code":200,"content_type":"text/plain","size":8731,"truncated":false,
 "file_path":"/var/folders/.../n9e-aiagent-fetch-xxx.yml"}
```

记住 `file_path`，后两步都要用。

**用户直接粘贴了 YAML 文本** —— 跳过 http_fetch，直接把文本作为后续工具的 `payload` 入参传（这种 case YAML 反正已经在上下文里了，没必要再落盘）。

**异常处理**：
- `truncated=true` 表示超过 `max_bytes`（默认 1 MiB）。再调一次 `http_fetch(url=..., save_to_file=true, max_bytes=8388608)`（8 MiB 是硬上限）。
- `status_code` 非 2xx：把状态码告诉用户，问要不要换 URL。
- "blocked: ... resolves to non-public address" 这类错误：用户给的是内网地址，让用户把 YAML 内容直接粘过来。

### 第二步：选业务组和数据源

并发调用（不要串行）：
- `list_busi_groups()` 查可见业务组。优先 `is_default=true` 的；只有一个时直接用；多于一个时**用 AskUserQuestion 让用户选**
- `list_datasources(plugin_type="prometheus")` 查 Prom 数据源。只有一个直接用；多于一个让用户选

如果用户请求里已经指明了业务组名 / 数据源名，按名称匹配后直接用，不必再问。

### 第三步：预览

```
preview_prom_rule_yaml(payload_file=<第一步的 file_path>)
```

返回是 `{total, items:[{name, severity, prom_ql, for_duration_sec, append_tags, annotations}]}`。

**汇报给用户的话要短**，挑前 3-5 条 + 总计就够，不要把所有规则全列出来：

> 解析到 38 条规则（critical 9 / warning 27 / info 2），例如：
> - `HostHighCpuLoad` (warning, for=0s)
> - `HostOutOfMemory` (warning, for=2m)
> - `HostOomKillDetected` (warning, for=0s)
> 是否全部导入到业务组「default」的 Prometheus 数据源「prom-prod」？

### 第四步：落库

用户确认后：

```
import_prom_rule_yaml(
  group_id=<第二步>,
  datasource_ids="[<第二步 ds id>]",
  payload_file=<第一步的 file_path>
)
```

`datasource_ids` 必须是 JSON 数组字符串，比如 `"[1]"` 或 `"[1,3]"`。

返回结构：

```json
{
  "total":   38,
  "created": 36,
  "skipped": 2,        // 重名规则，自动跳过，不是失败
  "failed":  0,        // 真正的写入错误（DB/校验等）
  "items": [
    {"name":"HostHighCpuLoad", "status":"skipped_duplicate"},
    {"name":"HostOutOfMemory", "status":"created", "id":177},
    {"name":"BadOne", "status":"failed", "error":"<message>"}
  ]
}
```

每条规则的 `status` 三选一：
- `created` — 新建成功，有 `id`
- `skipped_duplicate` — 同名规则已存在，**未做任何改动**，不需要重试
- `failed` — 真错误（DB、校验等），看 `error`

#### 处理重名（重要）

⚠️ **同名规则会被自动跳过（status=skipped_duplicate），不是失败**。看到 skipped > 0 时**绝对不要**用 `name_prefix` "重试整份 YAML"——那会让已经成功创建的 N 条规则全部多写一份带前缀的副本，造成翻倍数据污染。

正确的汇报方式：直接告诉用户哪几条因重名跳过，让用户决定怎么办：

> ✅ 已导入 36 条规则。另有 2 条因同名已存在被跳过：HostHighCpuLoad、HostOutOfMemory。
> 如果想覆盖它们，请到告警规则页面删掉旧的再重新导入；如果想让新旧并存（如做对比测试），可以加 name_prefix 重新导入但要清楚**所有 38 条都会被加前缀**。

只有用户**明确要求并存**（极少见的对比测试场景）才用 `name_prefix`/`name_suffix`，且要提前告知 LLM 会把全量规则都加前缀。

#### 真正的失败（status=failed）才需要排查

如果出现 `status=failed`，看 `error` 字段：常见的有数据源校验失败、字段格式错误等。这些是 YAML 本身或环境问题，name_prefix 治不了。

#### 谨慎模式

用户说"先建好但别启用" → 传 `disabled=1`。

### 第五步：输出结果

**保持简短**：

> ✅ 已向业务组「default」导入 36 条告警规则。
> ⚠️ 2 条因同名已存在被跳过：HostHighCpuLoad、HostOutOfMemory。需要覆盖请在告警规则页面手动删除旧规则后重导。

不要把 36 条规则的 ID 全列出来，前端会用卡片展示。**不要主动建议 "用 name_prefix 重试"——除非用户明确要并存。**

## payload vs payload_file 怎么选

| 场景 | 用哪个 |
|---|---|
| 通过 http_fetch 抓的远端文件 | **`payload_file`**（http_fetch 配 `save_to_file=true`） |
| 用户在聊天里直接粘贴的小段 YAML（< 50 行） | `payload`（YAML 反正已经在上下文了，再落盘没意义） |
| 用户在聊天里直接粘贴的大段 YAML（数十条规则） | 提示用户用 URL 或上传文件；如果坚持粘贴，仍用 `payload` |

**两者二选一，不能同时传**（工具会报错）。

## severity 映射（自动）

工具自动把 `labels.severity` 字符串映射到 n9e 的数字：

| Prom labels.severity | n9e severity |
|---|---|
| critical / error / fatal / page / sev1 | 1 |
| warning / warn / sev2 | 2 |
| info / notice / sev3 | 3 |
| 其他 / 缺失 | 2（默认 warning） |

不需要 LLM 手动转换。

## 其他 labels 处理

除 `severity` 之外的所有 labels 会自动拼成 n9e 的 `append_tags`（格式 `k=v`，空格会被去掉）。

## 常见坑

1. **`datasource_ids` 必须是 JSON 数组字符串**：传 `"[1]"`，不是 `"1"` 或 `1`。
2. **`http_fetch` 不能抓内网**：用户给的是 `http://10.x.x.x/...` 会直接拒绝。让用户改用公网 URL 或直接粘贴内容。
3. **`payload_file` 必须是 http_fetch 写出来的**：tool 会校验路径必须在 `os.TempDir()` 下且以 `n9e-aiagent-fetch-` 开头。不能用任意路径。
4. **YAML 里的 `expr` 原样落库**：n9e 直接存原表达式（含阈值），不要 LLM 拆 `>`/`<` 和阈值。
5. **`for` 字段转秒**：`5m` → 300，`1h` → 3600。原 YAML 没写 `for`，n9e 用 0 秒（立即触发）。
6. **不要去查 `list_alert_rules` 检测重名**。直接调 import，按返回的 error 决定是否加前缀重试。

## 示例对话

**用户：** 帮我导入 https://raw.githubusercontent.com/samber/awesome-prometheus-alerts/refs/heads/master/dist/rules/host-and-hardware/node-exporter.yml

**Thought →** 并发抓 URL（存到文件）+ 查业务组 + 查数据源。

**Action 1:** `http_fetch(url="https://...", save_to_file=true)` → `{file_path: "/var/.../n9e-aiagent-fetch-abc.yml", size: 8731, ...}`
**Action 2:** `list_busi_groups()` → 只有 default
**Action 3:** `list_datasources(plugin_type="prometheus")` → 只有 prom-prod

**Action 4:** `preview_prom_rule_yaml(payload_file="/var/.../n9e-aiagent-fetch-abc.yml")` → 38 条规则

**Final answer（预览阶段）:**
> 解析到 38 条规则…… 是否导入到业务组「default」/数据源「prom-prod」？

**用户：** 嗯

**Action 5:** `import_prom_rule_yaml(group_id=1, datasource_ids="[2]", payload_file="/var/.../n9e-aiagent-fetch-abc.yml")`

**Final answer:**
> ✅ 已导入 38 条告警规则到「default / prom-prod」。
