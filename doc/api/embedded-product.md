# Embedded Product API

Embedded products are used to integrate third-party page links into the Nightingale home page and sidebar. Every endpoint requires login (`auth` + `user`); write endpoints additionally require the corresponding permission point.

## Data structures

### EmbeddedProduct

| Field | Type | Required | Description |
|------|------|------|------|
| id | int64 | - | Primary key, auto-increment |
| name | string | Yes | Product name |
| url | string | Yes | Target URL |
| is_private | bool | No | Whether the entry is private. When `true`, `team_ids` must be specified |
| hide | bool | No | Whether the entry is hidden. Defaults to `false`; when `true` the frontend does not show it in the entry points, but the API still returns it |
| team_ids | int64[] | Conditional | IDs of the teams that can see the entry; must not be empty when `is_private=true` |
| weight | int | No | Sort weight, displayed in ascending order, default `0` |
| create_at | int64 | - | Creation time (Unix timestamp) |
| create_by | string | - | Username of the creator |
| update_at | int64 | - | Last update time (Unix timestamp) |
| update_by | string | - | Username of the last updater |
| update_by_nickname | string | - | Nickname of the last updater (returned by the list/detail endpoints only, filled in by a server-side join) |

The list is ordered by `ORDER BY weight ASC, id ASC`; entries with the same `weight` are stably ordered by ascending `id`.

---

## List embedded products

```
GET /api/n9e/embedded-product
```

Non-administrator users only get entries where `is_private=false` or where `team_ids` matches one of the current user's teams; administrators get everything.

### Response

```json
{
  "dat": [
    {
      "id": 1,
      "name": "Grafana",
      "url": "https://grafana.example.com",
      "is_private": false,
      "hide": false,
      "team_ids": [],
      "weight": 0,
      "create_at": 1710000000,
      "create_by": "admin",
      "update_at": 1710000000,
      "update_by": "admin",
      "update_by_nickname": "Administrator"
    }
  ],
  "err": ""
}
```

---

## Get embedded product details

```
GET /api/n9e/embedded-product/:id
```

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Embedded product ID |

### Errors

- `400` invalid id
- `403` the current user has no access to this private entry

---

## Create embedded products

```
POST /api/n9e/embedded-product
```

Requires the `/embedded-product/add` permission point. The request body is an array, so several entries can be created at once; on a primary-key conflict it performs an `UPSERT` (overwriting all fields). The server fills in `create_by` / `update_by` with the current user's nickname and `create_at` / `update_at` with the current time.

### Request body

```json
[
  {
    "name": "Grafana",
    "url": "https://grafana.example.com",
    "is_private": false,
    "hide": false,
    "team_ids": [],
    "weight": 0
  },
  {
    "name": "Internal Admin Console",
    "url": "https://admin.example.com",
    "is_private": true,
    "hide": false,
    "team_ids": [1, 2],
    "weight": 1
  }
]
```

### Validation rules

- `name` must not be empty and must not contain dangerous characters
- `url` must not be empty
- `team_ids` must not be empty when `is_private=true`

### Response

```json
{ "dat": null, "err": "" }
```

---

## Update an embedded product

```
PUT /api/n9e/embedded-product/:id
```

Requires the `/embedded-product/put` permission point. It overwrites the `name` / `url` / `is_private` / `hide` / `team_ids` / `weight` fields and refreshes `update_by` / `update_at`.

### Request body

```json
{
  "name": "Grafana Prod",
  "url": "https://grafana-prod.example.com",
  "is_private": true,
  "hide": false,
  "team_ids": [1, 2],
  "weight": 3
}
```

### Errors

- `400` invalid id
- `404` the entry does not exist

---

## Update the hidden state only

```
PUT /api/n9e/embedded-product/:id/hide
```

Requires the `/embedded-product/put` permission point. **Dedicated to the "show / hide" toggle**: it only updates the `hide` / `update_at` / `update_by` fields and never touches business fields such as `name` / `url` / `is_private` / `team_ids` / `weight`.

### Path parameters

| Parameter | Type | Description |
|------|------|------|
| id | int64 | Embedded product ID |

### Request body

```json
{ "hide": true }
```

| Field | Type | Required | Description |
|------|------|------|------|
| hide | bool | Yes | The desired hidden state |

### Errors

- `400` invalid id

### Response

```json
{ "dat": null, "err": "" }
```

---

## Batch update embedded product weights

```
PUT /api/n9e/embedded-products/weights
```

Requires the `/embedded-product/put` permission point. **Dedicated to the frontend drag-and-drop ordering scenario**: within a single transaction it only updates the `weight` / `update_at` / `update_by` fields and never touches business fields such as `name` / `url` / `is_private` / `team_ids`. If any single update fails, the whole batch is rolled back.

> The path uses the plural `embedded-products` to distinguish it from the single-entry `PUT /embedded-product/:id` and avoid ambiguity with the `:id` route parameter.

### Request body

```json
[
  { "id": 3, "weight": 0 },
  { "id": 1, "weight": 1 },
  { "id": 2, "weight": 2 }
]
```

### Validation rules

- An empty array returns success immediately without performing any write
- At most **1000** entries per request; more returns `400 too many items`
- Any `id <= 0` returns `400 invalid id` immediately
- If the same `id` appears more than once in a request, the last `weight` wins

### Response

```json
{ "dat": null, "err": "" }
```

### Example

When a drag ends, the frontend reassigns `weight` across the list in the new order (usually `0..N-1`) and submits everything in one call:

```bash
curl -X PUT 'https://n9e.example.com/api/n9e/embedded-products/weights' \
  -H 'Content-Type: application/json' \
  -H 'Cookie: <session>' \
  -d '[{"id":3,"weight":0},{"id":1,"weight":1},{"id":2,"weight":2}]'
```

---

## Delete an embedded product

```
DELETE /api/n9e/embedded-product/:id
```

Requires the `/embedded-product/delete` permission point.

### Errors

- `400` invalid id
