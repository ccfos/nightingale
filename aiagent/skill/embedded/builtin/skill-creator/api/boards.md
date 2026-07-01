# Dashboards (boards)

In n9e a monitoring dashboard is called a **board**. A board carries its metadata (name, group, tags, note, public flags) in the `board` table, while its panels/layout/variables are stored as one large JSON blob in a separate `board_payload` table. Use these endpoints to list dashboards across your business groups, look up specific boards by id, or fetch a single board together with its full panel configuration.

> Gateway call: GET, `path` relative to `/api/n9e`. Response `{"ok":true,"status":200,"data":{"dat":<payload>,"err":""}}` — read `data["dat"]`. Protocol: see `../n9e-api.md`.

## Endpoints
| Path | Purpose | `dat` shape |
|---|---|---|
| `/boards` | Boards selected by explicit ids | Bare array of **reduced** objects (NOT full `Board`) — see note below |
| `/busi-groups/boards` | Boards across your business groups | Bare array of `Board` (with `bgids` filled; `configs` empty) |
| `/busi-group/:id/boards` | Boards of one business group (`:id` in path) | Bare array of `Board` (`configs` empty) |
| `/board/:bid` | One board WITH its panel config (`:bid` in path) | Single `Board` object (`configs` populated) |

Notes:
- `:bid` for `/board/:bid` may be either the numeric board `id` or the board's string `ident` (the handler tries `ident` first, then `id`).
- The list endpoints do NOT include panel data; `configs` is empty on them. Only `/board/:bid` fills `configs`.

## Query parameters
| Param | Type | Required | Default | Meaning | Endpoint |
|---|---|---|---|---|---|
| `bids` | string (csv of int64) | no | empty (→ empty result) | Comma-separated board ids to fetch | `/boards` |
| `gids` | string (csv of int64) | no | your groups | Comma-separated business-group ids to filter by; if empty, defaults to all groups you belong to | `/busi-groups/boards` |
| `query` | string | no | empty | Space-separated fuzzy filter over board `name` + `tags`. A term prefixed with `-` excludes matches (e.g. `web -test`) | `/busi-groups/boards`, `/busi-group/:id/boards` |

`/board/:bid` takes no query params (all input is in the path).

## Response — `dat` payload
List endpoints return a bare array (Pattern B); `/board/:bid` returns a single object.

For `/busi-groups/boards`, `/busi-group/:id/boards`, and `/board/:bid`, each element is a `Board` (`models/board.go`):

| Field (`json`) | Type | Meaning |
|---|---|---|
| `id` | int64 | Board id (primary key) |
| `group_id` | int64 | Owning business-group id |
| `name` | string | Dashboard name |
| `ident` | string | Optional unique string identifier (used for shareable/public URLs; may be empty) |
| `tags` | string | Space-separated tags |
| `note` | string | Free-text description |
| `create_at` | int64 | Creation time (Unix seconds) |
| `create_by` | string | Creator username |
| `update_at` | int64 | Last-update time (Unix seconds) |
| `update_by` | string | Last-updater username |
| `update_by_nickname` | string | Display nickname of last updater (computed; filled on list endpoints) |
| `configs` | string | (computed) The board's full panel/layout config as a JSON-encoded **string**. Empty on list endpoints; populated only by `/board/:bid`. See below. |
| `public` | int | Whether the board is public — 0: false, 1: true |
| `public_cate` | int | Public visibility category — 0: anonymous, 1: any logged-in user, 2: business-group members |
| `bgids` | []int64 | (computed) Extra business-group ids the board is shared to; filled only by `/busi-groups/boards` |
| `built_in` | int | Whether this is a built-in board — 0: false, 1: true |
| `hide` | int | Whether the board is hidden from the list — 0: false, 1: true |

### `/boards` returns a reduced shape (not `Board`)
The `/boards` endpoint does NOT return `Board` objects. Its handler (`boardGetsByBids`) returns a bare array of small objects, one per found board, with only these keys:

| Field (`json`) | Type | Meaning |
|---|---|---|
| `board_id` | int64 | Board id |
| `board_name` | string | Dashboard name |
| `busi_group_id` | int64 | Owning business-group id |
| `busi_group_name` | string | Owning business-group name |

(Boards whose business group can't be resolved are silently skipped.) If you need full fields or panels for these ids, call `/board/:bid` per id.

### Getting panel configs
Panels are NOT inline fields of `Board`. They live in the separate `board_payload` table (`models/board_payload.go`, column `payload`). The `configs` field is marked `gorm:"-"`, so it is empty in every list response. Only `/board/:bid` populates it: the `boardGet` handler calls `BoardGet`, which loads the board row and then sets `board.Configs = BoardPayloadGet(id)`.

`configs` is itself a JSON-encoded **string** (the whole dashboard: panels, layout/grid positions, template variables, etc.), so consumers must parse `dat["configs"]` a second time to inspect individual panels and their queries. It may be an empty string if the board has no saved payload.

## Example
Request:
```json
{"method":"GET","path":"/busi-groups/boards","query":{}}
```
Response (trimmed):
```json
{
  "ok": true,
  "status": 200,
  "data": {
    "dat": [
      {
        "id": 12,
        "group_id": 2,
        "name": "Host Overview",
        "ident": "",
        "tags": "host linux",
        "note": "",
        "create_at": 1700000000,
        "create_by": "root",
        "update_at": 1700100000,
        "update_by": "root",
        "update_by_nickname": "Administrator",
        "configs": "",
        "public": 0,
        "public_cate": 0,
        "bgids": [3, 4],
        "built_in": 0,
        "hide": 0
      }
    ],
    "err": ""
  }
}
```

Fetch one board with its panels:
```json
{"method":"GET","path":"/board/12","query":{}}
```
Response (trimmed) — `configs` is now a JSON string you must parse again:
```json
{
  "ok": true,
  "status": 200,
  "data": {
    "dat": {
      "id": 12,
      "name": "Host Overview",
      "group_id": 2,
      "configs": "{\"version\":\"3.0.0\",\"panels\":[{\"type\":\"timeseries\",\"targets\":[{\"expr\":\"cpu_usage_active\"}]}]}",
      "public": 0,
      "public_cate": 0,
      "built_in": 0,
      "hide": 0
    },
    "err": ""
  }
}
```
