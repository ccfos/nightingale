# AI Skill API

页面接口需要管理员权限（`auth` + `admin`），Service 接口（`/v1/n9e`）使用 BasicAuth 认证。

## 数据结构

### AISkill

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int64 | - | 主键，自增 |
| name | string | 是 | Skill 名称 |
| description | string | 否 | 描述，建议说明用途和触发场景 |
| instructions | string | 是 | 提示词指令，支持 Markdown |
| license | string | 否 | 许可证，如 `MIT`、`Apache-2.0` |
| compatibility | string | 否 | 兼容性说明，如环境依赖、网络需求等 |
| metadata | map[string]string | 否 | 扩展元数据，如 `{"author": "org", "version": "1.0"}` |
| allowed_tools | string | 否 | 预授权工具列表，空格分隔，如 `Bash(git:*) Read` |
| enabled | bool | 否 | 是否启用，请显式传入 `true` 或 `false` |
| created_at | int64 | - | 创建时间（Unix 时间戳） |
| created_by | string | - | 创建人 |
| updated_at | int64 | - | 更新时间（Unix 时间戳） |
| updated_by | string | - | 更新人 |
| files | AISkillFile[] | - | 关联的资源文件列表（仅详情接口返回，不含 content） |

> `license`、`compatibility`、`metadata`、`allowed_tools` 字段参考 [Agent Skills Specification](https://agentskills.io/specification)。

### AISkillFile

| 字段 | 类型 | 说明 |
|------|------|------|
| id | int64 | 主键，自增 |
| skill_id | int64 | 关联的 Skill ID |
| name | string | 文件相对路径，如 `references/common/llm.md`、`scripts/api.py` |
| content | string | 文件内容（仅文件详情接口返回） |
| size | int64 | 文件大小（字节），创建时自动计算 |
| created_at | int64 | 创建时间（Unix 时间戳） |
| created_by | string | 创建人 |
| updated_at | int64 | 更新时间（Unix 时间戳） |
| updated_by | string | 更新人 |

---

## 获取 Skill 列表

```
GET /api/n9e/ai-skills
```

### 查询参数

| 参数 | 类型 | 说明 |
|------|------|------|
| search | string | 可选，按 name 或 description 模糊搜索 |

### 响应

```json
{
  "dat": [
    {
      "id": 1,
      "name": "query-generator",
      "description": "生成 PromQL/SQL 查询语句",
      "instructions": "# Query Generator\n...",
      "license": "Apache-2.0",
      "compatibility": "Requires network access",
      "metadata": {
        "author": "nightingale",
        "version": "1.0"
      },
      "allowed_tools": "Bash(git:*) Read",
      "enabled": true,
      "created_at": 1710000000,
      "created_by": "admin",
      "updated_at": 1710000000,
      "updated_by": "admin"
    }
  ],
  "err": ""
}
```

> 列表接口不返回 `files` 字段。

---

## 获取 Skill 详情

```
GET /api/n9e/ai-skill/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Skill ID |

### 响应

返回 Skill 完整信息，并包含关联的资源文件列表（不含文件 content）。文件的 `name` 字段为压缩包内的相对路径，前端可按 `/` 分割渲染为树形结构。

```json
{
  "dat": {
    "id": 1,
    "name": "firemap-skill",
    "description": "灭火图分析技能",
    "instructions": "# Firemap Skill\n...",
    "enabled": true,
    "created_at": 1710000000,
    "created_by": "admin",
    "updated_at": 1710000000,
    "updated_by": "admin",
    "files": [
      {
        "id": 10,
        "skill_id": 1,
        "name": "references/common/llm.md",
        "size": 1024,
        "created_at": 1710000000,
        "created_by": "admin",
        "updated_at": 1710000000,
        "updated_by": "admin"
      },
      {
        "id": 11,
        "skill_id": 1,
        "name": "references/firemap/abnormal-analysis.md",
        "size": 3072,
        "created_at": 1710000000,
        "created_by": "admin",
        "updated_at": 1710000000,
        "updated_by": "admin"
      },
      {
        "id": 12,
        "skill_id": 1,
        "name": "scripts/api.py",
        "size": 4096,
        "created_at": 1710000000,
        "created_by": "admin",
        "updated_at": 1710000000,
        "updated_by": "admin"
      }
    ]
  },
  "err": ""
}
```

### 错误

- `404` Skill 不存在

---

## 创建 Skill

```
POST /api/n9e/ai-skills
```

### 请求体

```json
{
  "name": "query-generator",
  "description": "生成 PromQL/SQL 查询语句",
  "instructions": "# Query Generator\n根据用户输入生成查询语句...",
  "license": "Apache-2.0",
  "compatibility": "Requires network access",
  "metadata": {
    "author": "nightingale",
    "version": "1.0"
  },
  "allowed_tools": "Bash(git:*) Read",
  "enabled": true
}
```

### 校验规则

- `name` 必填（自动 trim）
- `instructions` 必填（自动 trim）

### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回新创建的 Skill ID。

---

## 更新 Skill

```
PUT /api/n9e/ai-skill/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Skill ID |

### 请求体

同创建接口。

### 可更新字段

`name`、`description`、`instructions`、`license`、`compatibility`、`metadata`、`allowed_tools`、`enabled`。

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `404` Skill 不存在

---

## 删除 Skill

删除 Skill 时会级联删除关联的所有资源文件。

```
DELETE /api/n9e/ai-skill/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Skill ID |

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `404` Skill 不存在

---

## 导入 Skill（新建）

从压缩包导入创建新 Skill。压缩包根目录必须包含 `SKILL.md` 文件（含有效的 YAML frontmatter），其他文件作为资源文件存入数据库。如果同名 Skill 已存在则拒绝创建。

```
POST /api/n9e/ai-skills/import
```

### 请求格式

`multipart/form-data`

| 字段 | 类型 | 说明 |
|------|------|------|
| file | file | `.zip` 或 `.tar.gz`/`.tgz` 压缩包 |

### 压缩包结构

```
SKILL.md                           # 必须，Skill 定义文件
references/                        # 可选，参考资料
  common/
    llm.md
    workspace.md
  firemap/
    abnormal-analysis.md
    query-firemap.md
scripts/                           # 可选，脚本文件
  api.py
  rule_from_template.py
```

### SKILL.md 格式

必须包含有效的 YAML frontmatter，且 `name` 字段不能为空：

```markdown
---
name: my-skill
description: 技能描述
license: MIT
compatibility: Requires git, docker
metadata:
  author: my-org
  version: "1.0"
allowed-tools: Bash(git:*) Read
---

# Skill 指令内容

这里是 instructions 部分...
```

- `name` 必填，用于唯一标识 Skill
- `instructions`（frontmatter 之后的正文部分）必填，不能为空
- 没有有效 frontmatter 或 `name` 为空时，接口返回错误

### 限制

| 限制项 | 值 | 说明 |
|--------|------|------|
| 压缩包大小 | 10MB | 上传文件大小上限 |
| 解压后总大小 | 50MB | 防御高压缩比攻击 |
| SKILL.md 大小 | 64KB | 对应数据库 TEXT 字段上限 |
| 单个资源文件大小 | 16MB | 对应数据库 MEDIUMTEXT 字段上限 |
| 资源文件数量 | 50 | 每个 Skill 最多 50 个资源文件 |

### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回新创建的 Skill ID。

### 错误

- `400` 仅支持 `.zip` 和 `.tar.gz`/`.tgz` 文件
- `400` 压缩包超过 10MB
- `400` 根目录未找到 `SKILL.md`
- `400` `SKILL.md` 缺少有效的 YAML frontmatter 或 `name` 为空
- `400` `name` 或 `instructions` 为空（校验失败）
- `400` 同名 Skill 已存在
- `400` 文件数量或大小超限

---

## 导入 Skill（更新）

从压缩包更新已有 Skill。按 Skill ID 定位，全量替换：压缩包中存在的文件会覆盖同名旧文件，压缩包中不存在的旧文件会被删除。如果 SKILL.md 中的 `name` 与其他 Skill 冲突则拒绝。

```
PUT /api/n9e/ai-skill/:id/import
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Skill ID |

### 请求格式

同"导入 Skill（新建）"。

### 限制

同"导入 Skill（新建）"。

### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回更新的 Skill ID。

### 错误

- `404` Skill 不存在
- `400` 仅支持 `.zip` 和 `.tar.gz`/`.tgz` 文件
- `400` `SKILL.md` 缺少有效的 YAML frontmatter 或 `name` 为空
- `400` `name` 与其他 Skill 冲突

---

## 获取资源文件详情

获取单个资源文件的完整内容。

```
GET /api/n9e/ai-skill-file/:fileId
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| fileId | int64 | 文件 ID |

### 响应

```json
{
  "dat": {
    "id": 10,
    "skill_id": 1,
    "name": "references/common/llm.md",
    "content": "# LLM Reference\n文件完整内容...",
    "size": 1024,
    "created_at": 1710000000,
    "created_by": "admin",
    "updated_at": 1710000000,
    "updated_by": "admin"
  },
  "err": ""
}
```

### 错误

- `404` 文件不存在

---

## 删除资源文件

```
DELETE /api/n9e/ai-skill-file/:fileId
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| fileId | int64 | 文件 ID |

### 响应

```json
{
  "dat": "",
  "err": ""
}
```

### 错误

- `404` 文件不存在

---

## Service API（v1）

以下接口供其他服务调用，使用 BasicAuth 认证（需开启 `APIForService`）。写入类接口的 `created_by` / `updated_by` 固定为 `system`。

---

### 获取 Skill 列表

```
GET /v1/n9e/ai-skills
```

行为与页面接口 `GET /api/n9e/ai-skills` 一致。

#### 查询参数

| 参数 | 类型 | 说明 |
|------|------|------|
| search | string | 可选，按 name 或 description 模糊搜索 |

#### 响应

```json
{
  "dat": [
    {
      "id": 1,
      "name": "firemap-skill",
      "description": "灭火图分析技能",
      "instructions": "# Firemap Skill\n...",
      "enabled": true,
      "created_at": 1710000000,
      "created_by": "admin",
      "updated_at": 1710000000,
      "updated_by": "admin"
    }
  ],
  "err": ""
}
```

> 列表接口不返回 `files` 字段。

---

### 获取 Skill 详情（含文件内容）

```
GET /v1/n9e/ai-skill/:id
```

返回 Skill 完整信息及所有资源文件（**含 `content` 字段**），服务端可一次请求获取全部数据。

> 与页面接口 `GET /api/n9e/ai-skill/:id` 的区别：页面接口的 files 不含 `content`（前端按需加载），Service 接口的 files 含 `content`（一次拿齐）。

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Skill ID |

#### 响应

```json
{
  "dat": {
    "id": 1,
    "name": "firemap-skill",
    "description": "灭火图分析技能",
    "instructions": "# Firemap Skill\n...",
    "enabled": true,
    "created_at": 1710000000,
    "created_by": "admin",
    "updated_at": 1710000000,
    "updated_by": "admin",
    "files": [
      {
        "id": 10,
        "skill_id": 1,
        "name": "references/common/llm.md",
        "content": "# LLM Reference\n模型调用说明...",
        "size": 1024,
        "created_at": 1710000000,
        "created_by": "admin",
        "updated_at": 1710000000,
        "updated_by": "admin"
      },
      {
        "id": 11,
        "skill_id": 1,
        "name": "scripts/api.py",
        "content": "print('hello from api.py')\n",
        "size": 27,
        "created_at": 1710000000,
        "created_by": "admin",
        "updated_at": 1710000000,
        "updated_by": "admin"
      }
    ]
  },
  "err": ""
}
```

#### 错误

- `404` Skill 不存在

---

### 创建/更新 Skill（Upsert）

```
POST /v1/n9e/ai-skills
```

按 `name` 做 Upsert：同名 Skill 已存在则更新，不存在则创建。

#### 请求体

```json
{
  "name": "query-generator",
  "description": "生成 PromQL/SQL 查询语句",
  "instructions": "# Query Generator\n根据用户输入生成查询语句...",
  "license": "Apache-2.0",
  "compatibility": "Requires network access",
  "metadata": {
    "author": "nightingale",
    "version": "1.0"
  },
  "allowed_tools": "Bash(git:*) Read",
  "enabled": true
}
```

#### 校验规则

- `name` 必填（自动 trim）
- `instructions` 必填（自动 trim）

#### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回 Skill ID（创建时为新 ID，更新时为已有 ID）。

---

### 导入 Skill（新建）

```
POST /v1/n9e/ai-skills/import
```

从压缩包导入创建新 Skill，行为与页面接口 `POST /api/n9e/ai-skills/import` 一致，`created_by` / `updated_by` 固定为 `system`。

#### 请求格式

`multipart/form-data`

| 字段 | 类型 | 说明 |
|------|------|------|
| file | file | `.zip` 或 `.tar.gz`/`.tgz` 压缩包 |

#### 限制

同页面接口"导入 Skill（新建）"。

#### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回新创建的 Skill ID。

#### 错误

同页面接口"导入 Skill（新建）"。

---

### 导入 Skill（更新）

```
PUT /v1/n9e/ai-skill/:id/import
```

从压缩包更新已有 Skill，行为与页面接口 `PUT /api/n9e/ai-skill/:id/import` 一致，全量替换资源文件，`updated_by` 固定为 `system`。

#### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Skill ID |

#### 请求格式

`multipart/form-data`

| 字段 | 类型 | 说明 |
|------|------|------|
| file | file | `.zip` 或 `.tar.gz`/`.tgz` 压缩包 |

#### 限制

同页面接口"导入 Skill（更新）"。

#### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回更新的 Skill ID。

#### 错误

- `404` Skill 不存在
- 其他同页面接口"导入 Skill（更新）"
