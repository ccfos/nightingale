# Embedded Product API

嵌入式产品（Embedded Product）用于在夜莺首页/侧边栏集成第三方页面链接。所有接口均需登录（`auth` + `user`），写接口额外需要对应的权限点。

## 数据结构

### EmbeddedProduct

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | int64 | - | 主键，自增 |
| name | string | 是 | 产品名称 |
| url | string | 是 | 跳转地址 |
| is_private | bool | 否 | 是否私有。为 `true` 时必须指定 `team_ids` |
| team_ids | int64[] | 条件必填 | 可见团队 ID 列表，`is_private=true` 时不能为空 |
| weight | int | 否 | 排序权重，升序展示，默认 `0` |
| create_at | int64 | - | 创建时间（Unix 时间戳） |
| create_by | string | - | 创建人 username |
| update_at | int64 | - | 更新时间（Unix 时间戳） |
| update_by | string | - | 更新人 username |
| update_by_nickname | string | - | 更新人昵称（仅列表/详情接口返回，由服务端 join 填充） |

列表展示顺序为 `ORDER BY weight ASC, id ASC`；`weight` 相同时按 `id` 升序稳定排序。

---

## 获取 Embedded Product 列表

```
GET /api/n9e/embedded-product
```

非管理员用户只会返回 `is_private=false` 或 `team_ids` 命中当前用户团队的条目；管理员返回全部。

### 响应

```json
{
  "dat": [
    {
      "id": 1,
      "name": "Grafana",
      "url": "https://grafana.example.com",
      "is_private": false,
      "team_ids": [],
      "weight": 0,
      "create_at": 1710000000,
      "create_by": "admin",
      "update_at": 1710000000,
      "update_by": "admin",
      "update_by_nickname": "管理员"
    }
  ],
  "err": ""
}
```

---

## 获取 Embedded Product 详情

```
GET /api/n9e/embedded-product/:id
```

### 路径参数

| 参数 | 类型 | 说明 |
|------|------|------|
| id | int64 | Embedded Product ID |

### 错误

- `400` id 非法
- `403` 当前用户对该私有条目无访问权限

---

## 创建 Embedded Product

```
POST /api/n9e/embedded-product
```

需要权限点 `/embedded-product/add`。请求体为数组，支持一次创建多条；若主键冲突则走 `UPSERT`（全字段覆盖）。服务端自动填充 `create_by` / `update_by` 为当前用户昵称，`create_at` / `update_at` 为当前时间。

### 请求体

```json
[
  {
    "name": "Grafana",
    "url": "https://grafana.example.com",
    "is_private": false,
    "team_ids": [],
    "weight": 0
  },
  {
    "name": "内部管理台",
    "url": "https://admin.example.com",
    "is_private": true,
    "team_ids": [1, 2],
    "weight": 1
  }
]
```

### 校验规则

- `name` 不能为空，且不能包含危险字符
- `url` 不能为空
- `is_private=true` 时 `team_ids` 不能为空

### 响应

```json
{ "dat": null, "err": "" }
```

---

## 更新 Embedded Product

```
PUT /api/n9e/embedded-product/:id
```

需要权限点 `/embedded-product/put`。会覆盖 `name` / `url` / `is_private` / `team_ids` / `weight` 字段，并刷新 `update_by` / `update_at`。

### 请求体

```json
{
  "name": "Grafana Prod",
  "url": "https://grafana-prod.example.com",
  "is_private": true,
  "team_ids": [1, 2],
  "weight": 3
}
```

### 错误

- `400` id 非法
- `404` 条目不存在

---

## 批量更新 Embedded Product 权重

```
PUT /api/n9e/embedded-products/weights
```

需要权限点 `/embedded-product/put`。**专用于前端拖拽排序场景**，在单个事务内只更新 `weight` / `update_at` / `update_by` 三个字段，不会触碰 `name` / `url` / `is_private` / `team_ids` 等业务字段；任何一条更新失败则整体回滚。

> 路径用复数 `embedded-products`，与单条 `PUT /embedded-product/:id` 区分，避免与 `:id` 路由参数歧义。

### 请求体

```json
[
  { "id": 3, "weight": 0 },
  { "id": 1, "weight": 1 },
  { "id": 2, "weight": 2 }
]
```

### 校验规则

- 请求体为空数组时直接返回成功，不会产生任何写操作
- 单次最多提交 **1000** 条，超出返回 `400 too many items`
- 任一 `id <= 0` 直接返回 `400 invalid id`
- 同一请求内重复的 `id` 以最后一次出现的 `weight` 为准

### 响应

```json
{ "dat": null, "err": "" }
```

### 使用示例

前端拖拽结束后，把当前列表按新顺序重新赋 `weight`（通常 `0..N-1`），一次性提交：

```bash
curl -X PUT 'https://n9e.example.com/api/n9e/embedded-products/weights' \
  -H 'Content-Type: application/json' \
  -H 'Cookie: <session>' \
  -d '[{"id":3,"weight":0},{"id":1,"weight":1},{"id":2,"weight":2}]'
```

---

## 删除 Embedded Product

```
DELETE /api/n9e/embedded-product/:id
```

需要权限点 `/embedded-product/delete`。

### 错误

- `400` id 非法
