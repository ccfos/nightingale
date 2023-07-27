# mongodb

mongodb 监控采集插件，由mongodb-exporter（https://github.com/percona/mongodb_exporter）封装而来。

## Configuration

- 配置权限，至少授予以下权限给配置文件中用于连接 MongoDB 的 user 才能收集指标：
    ```
    {
         "role":"clusterMonitor",
         "db":"admin"
      },
      {
         "role":"read",
         "db":"local"
      }
 
    ```
    更详细的权限配置请参考[官方文档](https://www.mongodb.com/docs/manual/reference/built-in-roles/#mongodb-authrole-clusterMonitor)

## 监控大盘和告警规则

在内置仪表盘和内置告警规则页面，可以直接导入使用。