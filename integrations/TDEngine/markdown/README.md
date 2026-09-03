# TDEngine

TDEngine 通过 taosKeeper 暴露 Prometheus 指标，默认 HTTP 端口为 `6043`。

参考：[taosKeeper 官方说明](https://docs.tdengine.com/reference/components/taoskeeper/)。

当前目录的 `TaosKeeper 3.x Prometheus Dashboard` 使用 `taos_*` 指标名，与 taosKeeper 的旧 `/metrics` 端点匹配。本次使用 TDengine Enterprise 3.4.2.3 和 `http://taoskeeper:6043/metrics` 完成验证。

## 采集配置

新建 `conf/input.prometheus/taoskeeper.toml`：

```toml
interval = 15

[[instances]]
urls = ["http://127.0.0.1:6043/metrics"]
url_label_key = "instance"
url_label_value = "{{.Host}}"
labels = { job = "taoskeeper" }
```

先确认指标端点：

```bash
curl -fsS http://127.0.0.1:6043/metrics \
  | grep -E 'taos_cluster_info|taos_dn_cpu_taosd' \
  | head
```

## `/metrics/v2` 兼容性

新版 taosKeeper 推荐 `/metrics/v2`，但它主要输出 `taosd_*` 指标，不能直接套用当前仍查询 `taos_*` 的模板。如果改用 `/metrics/v2`，需要同步升级仪表盘 PromQL；多 taosKeeper 实例时还必须抓取所有实例的 `/metrics/v2`，否则数据不完整。

当前测试拓扑没有覆盖所有 dnode、mnode、vgroup 和授权场景，因此相关面板可能为空。
