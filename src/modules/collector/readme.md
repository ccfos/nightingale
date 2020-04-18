linux 服务器基础资源采集agent

## 功能
系统指标采集


## 接口

#### 上报数据

```
POST /api/collector/push

request body:
// endpoint 可以填ip或者hostname, 如果ip是在运维平台是唯一表示, 那就填ip, hostname类同
// step 为监控指标的采集周期
// tags 监控指标的额外描述, a=b的形式, 可以填多个, 多个用逗号隔开, 比如 group=devops,module=api
[
    {
        "metric":"qps",
        "endpoint":"hostname",
        "timestamp":1559733442,
        "step":10,
        "value":1,
        "tags":""
    }
]

response body:
{
     
    "dat": "ok"
    "err": "",
}
```

-------
#### 获取已生效的采集策略

```
GET /api/collector/stra

response body:
{
    "dat": [
        {
            "collect_type": "port",
            "comment": "test",
            "created": "2019-06-05T18:52:58+08:00",
            "creator": "root",
            "id": 1,
            "last_updated": "2019-06-17T15:46:06+08:00",
            "last_updator": "root",
            "name": "test",
            "nid": 2,
            "port": 8047,
            "step": 10,
            "tags": "port=8047,service=tsdb",
            "timeout": 3
        }
    ],
    "err": ""
}
```

