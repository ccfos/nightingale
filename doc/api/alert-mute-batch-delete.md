# 批量清理过期屏蔽规则 API

用于一次性清理历史遗留的、已经过期的告警屏蔽规则（AlertMute）。常见场景：长期运行后系统中累积了大量临时屏蔽，需要按业务组或全局做一次性清理。

接口为异步执行：服务端会立即返回成功响应，然后在后台分批删除符合条件的数据，直到没有更多匹配为止。

## 接口定义

```
DELETE /api/n9e/alert-mutes
```

- 权限：`auth` + `admin`，仅管理员可调用。
- Content-Type：`application/json`。

## 请求参数

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| timestamp | int64 | 是 | Unix 秒级时间戳。只删除 `create_at < timestamp` 的屏蔽规则。可以理解为"删除这个时间点之前创建的过期屏蔽"。 |
| group_ids | int64[] | 否 | 业务组 ID 列表。传入后只清理这些业务组下的屏蔽规则；不传或空数组表示全部业务组。 |

## 删除条件

后台批处理逻辑只会删除**同时满足**以下条件的记录：

1. `etime > 0`：屏蔽有结束时间（永久屏蔽 `etime = 0` 不会被清理）。
2. `etime < now`：当前时间已经超过结束时间，即已经过期。
3. `create_at < timestamp`：创建时间早于请求中的 `timestamp`。
4. 如果传了 `group_ids`，再追加 `group_id IN (group_ids)`。

换言之：本接口**只清理过期屏蔽**，对永久屏蔽和仍在生效期内的屏蔽都不会触碰。

## 响应

请求被接收并启动后台任务后立即返回：

```json
{
  "dat": "Alert mutes deletion started",
  "err": ""
}
```

后台按 1000 条/批的步长循环删除，每批之间 sleep 100ms，直到某一批返回的行数小于 1000 为止；遇到 DB 错误会中断并写入 server 日志。

参数缺失时同步返回 400：

```json
{
  "err": "timestamp parameter is required"
}
```

## 请求示例

清理某两个业务组下、30 天前创建且已过期的屏蔽：

```bash
NOW=$(date +%s)
THRESHOLD=$((NOW - 30*24*3600))

curl -X DELETE 'http://<n9e-host>/api/n9e/alert-mutes' \
  -H 'Content-Type: application/json' \
  -H 'Cookie: <admin-session>' \
  -d "{\"timestamp\": ${THRESHOLD}, \"group_ids\": [1, 2]}"
```

全局清理所有业务组下 30 天前的过期屏蔽：

```bash
curl -X DELETE 'http://<n9e-host>/api/n9e/alert-mutes' \
  -H 'Content-Type: application/json' \
  -H 'Cookie: <admin-session>' \
  -d "{\"timestamp\": ${THRESHOLD}}"
```

## 注意事项

- 接口是**异步**的，返回 `Alert mutes deletion started` 并不代表清理已经完成。如需确认结果，可在 server 日志中搜索 `Successfully deleted alert mutes` / `Failed to delete alert mutes`，或再次调用列表接口核对剩余数量。
- 清理动作不可逆，删除前请确认 `timestamp` 与 `group_ids` 是预期范围。
- 永久屏蔽（`etime = 0`）和未来才到期的屏蔽都不会被删除，调用方无需额外做兜底过滤。
