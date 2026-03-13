# AI Skill API

所有接口需要管理员权限（`auth` + `admin`）。

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
| enabled | int | 否 | 是否启用，默认 1 |
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
| name | string | 文件名 |
| content | string | 文件内容（仅文件详情接口返回） |
| size | int64 | 文件大小（字节），创建时自动计算 |
| created_at | int64 | 创建时间（Unix 时间戳） |
| created_by | string | 创建人 |

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
      "enabled": 1,
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

返回 Skill 完整信息，并包含关联的资源文件列表（不含文件 content）。

```json
{
  "dat": {
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
    "enabled": 1,
    "created_at": 1710000000,
    "created_by": "admin",
    "updated_at": 1710000000,
    "updated_by": "admin",
    "files": [
      {
        "id": 10,
        "skill_id": 1,
        "name": "reference.md",
        "size": 2048,
        "created_at": 1710000000,
        "created_by": "admin"
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
  "enabled": 1
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

## 导入 Skill

从 `.md` 文件导入 Skill，支持 YAML frontmatter 格式。

```
POST /api/n9e/ai-skills/import
```

### 请求格式

`multipart/form-data`

| 字段 | 类型 | 说明 |
|------|------|------|
| file | file | `.md` 文件 |

### 文件格式

支持标准的 YAML frontmatter + Markdown body：

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

- 如果文件包含有效的 frontmatter，则从中提取 `name`、`description`、`license`、`compatibility`、`metadata`、`allowed-tools`
- 如果没有 frontmatter，则以文件名作为 `name`，全部内容作为 `instructions`

### 响应

```json
{
  "dat": 1,
  "err": ""
}
```

返回新创建的 Skill ID。

### 错误

- `400` 仅支持 `.md` 文件

---

## 上传 Skill 资源文件

```
POST /api/n9e/ai-skill/:id/files
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Skill ID |

### 请求格式

`multipart/form-data`

| 字段 | 类型 | 说明 |
|------|------|------|
| file | file | 资源文件 |

### 限制

- 允许的文件类型：`.md`、`.txt`、`.json`、`.yaml`、`.yml`、`.csv`
- 单文件最大 2MB
- 每个 Skill 最多 20 个资源文件

### 响应

```json
{
  "dat": 10,
  "err": ""
}
```

返回新创建的文件 ID。

### 错误

- `404` Skill 不存在
- `400` 文件类型不支持 / 文件超过 2MB / 文件数量超过 20

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
    "name": "reference.md",
    "content": "# Reference\n文件完整内容...",
    "size": 2048,
    "created_at": 1710000000,
    "created_by": "admin"
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
