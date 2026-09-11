# AI Skill API

The UI endpoints require administrator privileges (`auth` + `admin`); the service endpoints (`/v1/n9e`) use BasicAuth.

## Data structures

### AISkill

| Field | Type | Required | Description |
|------|------|------|------|
| id | int64 | - | Primary key, auto-increment |
| name | string | Yes | Skill name |
| description | string | No | Description; state what it is for and when it triggers |
| instructions | string | Yes | The prompt instructions, Markdown supported |
| license | string | No | License, e.g. `MIT` or `Apache-2.0` |
| compatibility | string | No | Compatibility notes, such as environment dependencies or network requirements |
| metadata | map[string]string | No | Extra metadata, e.g. `{"author": "org", "version": "1.0"}` |
| allowed_tools | string | No | Pre-authorized tool list, space separated, e.g. `Bash(git:*) Read` |
| enabled | bool | No | Whether the skill is enabled; pass `true` or `false` explicitly |
| source_type | string | No | Source type: `local` / `git`, default `local` |
| git_info | AISkillGitInfo | No | Git source information. For built-in skills only `current_commit` is returned |
| has_new_version | bool | - | Returned for built-in Git skills; determined asynchronously from a background cache |
| created_at | int64 | - | Creation time (Unix timestamp) |
| created_by | string | - | Creator |
| updated_at | int64 | - | Last update time (Unix timestamp) |
| updated_by | string | - | Last updated by |
| files | AISkillFile[] | - | The associated resource files (returned by the detail endpoint only, without `content`) |

> The Git token is write-only and is never returned in a response. For built-in skills every endpoint returns only `git_info.current_commit` and hides the rest of the Git configuration; `has_new_version` is meaningful for built-in Git skills.

> The `license`, `compatibility`, `metadata`, and `allowed_tools` fields follow the [Agent Skills Specification](https://agentskills.io/specification).

### AISkillGitInfo

| Field | Type | Required | Description |
|------|------|------|------|
| url | string | No | HTTPS Git repository URL |
| ref_type | string | No | Git reference type: `branch` / `tag` / `commit` |
| ref | string | No | Git branch, tag, or commit |
| auth_type | string | No | Git authentication type: `none` / `token` |
| token | string | No | Git token; write-only, never returned in a response |
| subdir | string | No | Relative directory of the skill inside the repository |
| current_commit | string | No | The commit currently synced |

### AISkillFile

| Field | Type | Description |
|------|------|------|
| id | int64 | Primary key, auto-increment |
| skill_id | int64 | ID of the associated skill |
| name | string | Relative file path, e.g. `references/common/llm.md` or `scripts/api.py` |
| content | string | File content (returned by the file detail endpoint only) |
| size | int64 | File size in bytes, computed automatically on creation |
| created_at | int64 | Creation time (Unix timestamp) |
| created_by | string | Creator |
| updated_at | int64 | Last update time (Unix timestamp) |
| updated_by | string | Last updated by |

---

## List skills

```
GET /api/n9e/ai-skills
```

### Query parameters

| Parameter | Type | Description |
|------|------|------|
| search | string | Optional; fuzzy search on name or description |

### Response

```json
{
  "dat": [
    {
      "id": 1,
      "name": "query-generator",
      "description": "Generates PromQL/SQL queries",
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

> The list endpoint does not return the `files` field.

---

## Get skill details

```
GET /api/n9e/ai-skill/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Skill ID |

### Response

Returns the complete skill along with its resource files (without file `content`). A file's `name` is its relative path inside the archive, so the frontend can split on `/` and render a tree.

```json
{
  "dat": {
    "id": 1,
    "name": "firemap-skill",
    "description": "Firemap analysis skill",
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

### Errors

- `404` the skill does not exist

---

## Create a skill

```
POST /api/n9e/ai-skills
```

### Request body

```json
{
  "name": "query-generator",
  "description": "Generates PromQL/SQL queries",
  "instructions": "# Query Generator\nGenerate a query from the user's input...",
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

### Validation rules

- `name` is required (trimmed automatically)
- `instructions` is required (trimmed automatically)

### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the ID of the newly created skill.

---

## Update a skill

```
PUT /api/n9e/ai-skill/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Skill ID |

### Request body

Same as the create endpoint.

### Updatable fields

`name`, `description`, `instructions`, `license`, `compatibility`, `metadata`, `allowed_tools`, `enabled`.

### Response

```json
{
  "dat": "",
  "err": ""
}
```

### Errors

- `404` the skill does not exist

---

## Install a skill from Git

Pulls a skill from an HTTPS Git repository. The repository root, or the directory given by `git_subdir`, must contain a `SKILL.md`.

```
POST /api/n9e/ai-skills/git/install
```

### Request body

```json
{
  "git_url": "https://github.com/example/my-skill.git",
  "git_ref_type": "branch",
  "git_ref": "main",
  "git_auth_type": "token",
  "git_token": "github_pat_xxx",
  "git_subdir": "skills/foo",
  "enabled": true
}
```

### Notes

- `git_auth_type` supports `none` and `token`; with `token`, `git_token` is required and may be plaintext or an `enc:` RSA ciphertext. For credentials that need a username, such as a Deploy Token, use the `username:token` format; without a colon the default username is used.
- `git_ref_type` supports `branch`, `tag`, and `commit`.
- `git_subdir` must be a relative path inside the repository.
- A successful pull creates `ai_skill` and `ai_skill_file` records and records `git_info.current_commit`.
- `git_token` is stored encrypted and is never echoed back in a response.

### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the ID of the newly created skill.

---

## Update the Git configuration

Updates only the Git source configuration of an existing Git skill. It does not pull the repository and does not overwrite the skill's content or resource files.

```
PUT /api/n9e/ai-skill/:id/git/install
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Skill ID |

### Request body

Fields that are not supplied keep their existing values. When `git_auth_type=token` and no token has been saved yet, `git_token` must be supplied.

```json
{
  "git_url": "https://github.com/example/my-skill.git",
  "git_ref_type": "branch",
  "git_ref": "main",
  "git_auth_type": "token",
  "git_token": "new-token-if-rotated",
  "git_subdir": "skills/foo"
}
```

### Response

```json
{
  "dat": 1,
  "err": ""
}
```

### Errors

- `404` the skill does not exist
- `400` the target skill does not come from Git
- `400` the Git configuration of a built-in Git skill cannot be changed through this endpoint

---

## Update a skill from Git

Pulls from Git again and overwrites the content and resource files of an existing Git skill.

```
POST /api/n9e/ai-skill/:id/git/update
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Skill ID |

### Request body

An ordinary Git skill may be given a new Git configuration; fields that are not supplied keep their existing values.

```json
{
  "git_ref_type": "branch",
  "git_ref": "main",
  "git_token": "new-token-if-rotated",
  "git_subdir": ""
}
```

For a built-in Git skill the Git configuration in the request body is ignored and only the Git information preset in the database is used, so the built-in source can be neither exposed nor tampered with by the frontend.

### Response

```json
{
  "dat": 1,
  "err": ""
}
```

### Errors

- `404` the skill does not exist
- `400` the target skill does not come from Git

---

## Delete a skill

Deleting a skill cascades to all of its resource files.

```
DELETE /api/n9e/ai-skill/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Skill ID |

### Response

```json
{
  "dat": "",
  "err": ""
}
```

### Errors

- `404` the skill does not exist

---

## Import a skill (create)

Creates a new skill from an archive. The archive root must contain a `SKILL.md` file (with valid YAML frontmatter); every other file is stored in the database as a resource file. If a skill with the same name already exists, creation is rejected.

```
POST /api/n9e/ai-skills/import
```

### Request format

`multipart/form-data`

| Field | Type | Description |
|------|------|------|
| file | file | A `.zip` or `.tar.gz`/`.tgz` archive |

### Archive structure

```
SKILL.md                           # required, the skill definition file
references/                        # optional, reference material
  common/
    llm.md
    workspace.md
  firemap/
    abnormal-analysis.md
    query-firemap.md
scripts/                           # optional, script files
  api.py
  rule_from_template.py
```

### SKILL.md format

It must contain valid YAML frontmatter, and the `name` field must not be empty:

```markdown
---
name: my-skill
description: Skill description
license: MIT
compatibility: Requires git, docker
metadata:
  author: my-org
  version: "1.0"
allowed-tools: Bash(git:*) Read
---

# Skill instructions

This is the instructions section...
```

- `name` is required and uniquely identifies the skill
- `instructions` (the body after the frontmatter) is required and must not be empty
- If there is no valid frontmatter, or `name` is empty, the endpoint returns an error

### Limits

| Limit | Value | Description |
|--------|------|------|
| Archive size | 10MB | Maximum size of the uploaded file |
| Total size after extraction | 50MB | Guards against high-compression-ratio attacks |
| SKILL.md size | 64KB | Matches the database TEXT column limit |
| Size of a single resource file | 16MB | Matches the database MEDIUMTEXT column limit |
| Number of resource files | 50 | At most 50 resource files per skill |

### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the ID of the newly created skill.

### Errors

- `400` only `.zip` and `.tar.gz`/`.tgz` files are supported
- `400` the archive is larger than 10MB
- `400` no `SKILL.md` found in the root directory
- `400` `SKILL.md` has no valid YAML frontmatter, or `name` is empty
- `400` `name` or `instructions` is empty (validation failure)
- `400` a skill with the same name already exists
- `400` the file count or size limit was exceeded

---

## Import a skill (update)

Updates an existing skill from an archive. The skill is located by ID and fully replaced: files present in the archive overwrite the old files of the same name, and old files absent from the archive are deleted. If the `name` in SKILL.md collides with another skill, the request is rejected.

```
PUT /api/n9e/ai-skill/:id/import
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Skill ID |

### Request format

Same as "Import a skill (create)".

### Limits

Same as "Import a skill (create)".

### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the ID of the updated skill.

### Errors

- `404` the skill does not exist
- `400` only `.zip` and `.tar.gz`/`.tgz` files are supported
- `400` `SKILL.md` has no valid YAML frontmatter, or `name` is empty
- `400` `name` collides with another skill

---

## Get resource file details

Returns the full content of a single resource file.

```
GET /api/n9e/ai-skill-file/:fileId
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| fileId | int64 | File ID |

### Response

```json
{
  "dat": {
    "id": 10,
    "skill_id": 1,
    "name": "references/common/llm.md",
    "content": "# LLM Reference\nfull file content...",
    "size": 1024,
    "created_at": 1710000000,
    "created_by": "admin",
    "updated_at": 1710000000,
    "updated_by": "admin"
  },
  "err": ""
}
```

### Errors

- `404` the file does not exist

---

## Delete a resource file

```
DELETE /api/n9e/ai-skill-file/:fileId
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| fileId | int64 | File ID |

### Response

```json
{
  "dat": "",
  "err": ""
}
```

### Errors

- `404` the file does not exist

---

## Service API (v1)

The following endpoints are for other services and use BasicAuth (`APIForService` must be enabled). For write endpoints, `created_by` / `updated_by` is always `system`.

---

### List skills

```
GET /v1/n9e/ai-skills
```

Behaves the same as the UI endpoint `GET /api/n9e/ai-skills`.

#### Query parameters

| Parameter | Type | Description |
|------|------|------|
| search | string | Optional; fuzzy search on name or description |

#### Response

```json
{
  "dat": [
    {
      "id": 1,
      "name": "firemap-skill",
      "description": "Firemap analysis skill",
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

> The list endpoint does not return the `files` field.

---

### Get skill details (including file content)

```
GET /v1/n9e/ai-skill/:id
```

Returns the complete skill and all of its resource files (**including the `content` field**), so a service can fetch everything in one request.

> How it differs from the UI endpoint `GET /api/n9e/ai-skill/:id`: the UI endpoint's files omit `content` (the frontend loads it on demand), while the service endpoint's files include `content` (everything at once).

#### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Skill ID |

#### Response

```json
{
  "dat": {
    "id": 1,
    "name": "firemap-skill",
    "description": "Firemap analysis skill",
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
        "content": "# LLM Reference\nHow to call the model...",
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

#### Errors

- `404` the skill does not exist

---

### Create or update a skill (upsert)

```
POST /v1/n9e/ai-skills
```

Upserts by `name`: if a skill with the same name exists it is updated, otherwise it is created.

#### Request body

```json
{
  "name": "query-generator",
  "description": "Generates PromQL/SQL queries",
  "instructions": "# Query Generator\nGenerate a query from the user's input...",
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

#### Validation rules

- `name` is required (trimmed automatically)
- `instructions` is required (trimmed automatically)
- The request is treated as a Git source only when `source_type=git`: the server pulls the repository first and uses the `SKILL.md` and resource files from it as the final content; the `name` in the request body is only used to find an existing record.
- The service endpoint may write the Git information of a built-in skill, but no read endpoint ever exposes a built-in skill's Git configuration.

Example of a Git-source request:

```json
{
  "source_type": "git",
  "git_info": {
    "url": "https://github.com/example/my-skill.git",
    "ref_type": "branch",
    "ref": "main",
    "auth_type": "none",
    "subdir": "skills/foo"
  },
  "enabled": true
}
```

#### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the skill ID (a new one on creation, the existing one on update).

---

### Import a skill (create)

```
POST /v1/n9e/ai-skills/import
```

Creates a new skill from an archive. Behaves the same as the UI endpoint `POST /api/n9e/ai-skills/import`, with `created_by` / `updated_by` always set to `system`.

#### Request format

`multipart/form-data`

| Field | Type | Description |
|------|------|------|
| file | file | A `.zip` or `.tar.gz`/`.tgz` archive |

#### Limits

Same as the UI endpoint "Import a skill (create)".

#### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the ID of the newly created skill.

#### Errors

Same as the UI endpoint "Import a skill (create)".

---

### Import a skill (update)

```
PUT /v1/n9e/ai-skill/:id/import
```

Updates an existing skill from an archive. Behaves the same as the UI endpoint `PUT /api/n9e/ai-skill/:id/import`, fully replacing the resource files, with `updated_by` always set to `system`.

#### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Skill ID |

#### Request format

`multipart/form-data`

| Field | Type | Description |
|------|------|------|
| file | file | A `.zip` or `.tar.gz`/`.tgz` archive |

#### Limits

Same as the UI endpoint "Import a skill (update)".

#### Response

```json
{
  "dat": 1,
  "err": ""
}
```

Returns the ID of the updated skill.

#### Errors

- `404` the skill does not exist
- Everything else is the same as the UI endpoint "Import a skill (update)"
