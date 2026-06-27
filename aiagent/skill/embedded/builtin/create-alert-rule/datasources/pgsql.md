# PostgreSQL alert rules

- `prod`: `"metric"`
- `cate`: `"pgsql"`
- `recover_config.judge_type`: `1` (metric type)
- **Required** `database`: the name of the database the query runs against
- **Required** `keys.valueKey`: the alias of the numeric column in the SELECT statement

## Key constraint: SQL must use 3-part naming `db.schema.table`

Just like MySQL uses `db.table`, PostgreSQL also requires **writing the database name into the SQL**.

The PostgreSQL plugin requires the SQL to use the **`database.schema.table`** three-part naming format (e.g. `testdb.public.events`), and internally the plugin will:
1. Extract the database name (the first segment) from the SQL via regex
2. Switch the connection to that database
3. Format the three-part name as `"db"."schema"."table"` before executing

**If the SQL only writes `FROM events` or `FROM public.events` (missing the database name), it reports the error `no valid table name in format database.schema.table found`**.

## OSS edition limitation

**The OSS edition of n9e's PostgreSQL data source does not support time variables such as `$from`/`$to`/`$__timeFilter`**; the variables are not substituted.

**Correct approach**: use PostgreSQL's native time functions:
- Last N minutes: `WHERE created_at >= NOW() - INTERVAL '5 minutes'`
- Last N hours: `WHERE created_at >= NOW() - INTERVAL '1 hour'`
- From the start of today until now: `WHERE DATE(created_at) = CURRENT_DATE`

## triggers hard rules (must read)

- `exp` is **required** and is the only field the alert engine evaluates (a rule without exp will never fire once created, with no error whatsoever)
- Variable syntax for this data source: `$<ref>.<valueKey alias>`, e.g. `$A.value > 5`; with only one valueKey you may omit the alias and write `$A` directly, but with multiple valueKeys you **must include the alias** (a bare `$A` has an undefined value)
- `mode` is fixed at `1` (expression mode; the frontend displays exp as-is); join multiple conditions with `&&` / `||`, e.g. `"$A.value > 10 && $B.value < 5"`

## rule_config structure

```json
{
  "rule_config": {
    "queries": [
      {
        "ref": "A",
        "sql": "SELECT count(*) AS value FROM testdb.public.events WHERE created_at >= NOW() - INTERVAL '5 minutes' AND severity = 'critical'",
        "keys": {
          "valueKey": "value",
          "labelKey": ""
        },
        "interval": 60
      }
    ],
    "triggers": [
      {
        "mode": 1,
        "exp": "$A.value > 5",
        "severity": 1,
        "recover_config": {"judge_type": 1}
      }
    ]
  }
}
```

## query field reference

| Field | Required | Description |
|---|---|---|
| `ref` | ✅ | Query reference name |
| `sql` | ✅ | PostgreSQL SQL. **Must use the `FROM db.schema.table` three-part naming** (e.g. `FROM testdb.public.events`) |
| `keys.valueKey` | ✅ | **Required**, the alias of the numeric column |
| `keys.labelKey` | ❌ | Label column alias(es), multiple separated by spaces |
| `interval` | ❌ | Query execution interval, **unit: total seconds** (60=1 minute, 300=5 minutes, 3600=1 hour). **Do not write `interval_unit`** |

## Multi-schema example

PostgreSQL's default schema is `public`, but there may be other schemas:

```json
{
  "ref": "A",
  "sql": "SELECT count(*) AS value FROM testdb.monitoring.events WHERE created_at >= NOW() - INTERVAL '5 minutes'",
  "keys": {"valueKey": "value"},
  "interval": 60
}
```

## Three-part naming quick reference

| Description | SQL syntax |
|---|---|
| Default public schema | `FROM testdb.public.events` |
| Other schema | `FROM testdb.monitoring.events` |
| Multi-table JOIN | `FROM testdb.public.orders o JOIN testdb.public.items i ON o.id = i.order_id` |
