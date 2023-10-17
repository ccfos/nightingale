# 应用场景
```
应用于input插件库exec目录之外的特殊或自定义实现指定业务的监控。
监控脚本采集到监控数据之后通过相应的格式输出到stdout，categraf截获stdout内容，解析之后传给服务端，
脚本的输出格式支持3种：influx、falcon、prometheus，通过 exec.toml 的 `data_format` 配置告诉 Categraf。
data_format有3个值，其用法为：
```

## influx

influx 格式的内容规范：
```
mesurement,labelkey1=labelval1,labelkey2=labelval2 field1=1.2,field2=2.3
```
- 首先mesurement，表示一个类别的监控指标，比如 connections；
- mesurement后面是逗号，逗号后面是标签，如果没有标签，则mesurement后面不需要逗号
- 标签是k=v的格式，多个标签用逗号分隔，比如region=beijing,env=test
- 标签后面是空格
- 空格后面是属性字段，多个属性字段用逗号分隔
- 属性字段是字段名=值的格式，在categraf里值只能是数字
  最终，mesurement和各个属性字段名称拼接成metric名字

## falcon
Open-Falcon的格式如下，举例：

```json
[
    {
        "endpoint": "test-endpoint",
        "metric": "test-metric",
        "timestamp": 1658490609,
        "step": 60,
        "value": 1,
        "counterType": "GAUGE",
        "tags": "idc=lg,loc=beijing",
    },
    {
        "endpoint": "test-endpoint",
        "metric": "test-metric2",
        "timestamp": 1658490609,
        "step": 60,
        "value": 2,
        "counterType": "GAUGE",
        "tags": "idc=lg,loc=beijing",
    }
]
```
timestamp、step、counterType，这三个字段在categraf处理的时候会直接忽略掉，endpoint会放到labels里上报。

## prometheus
prometheus 格式大家不陌生了，比如我这里准备一个监控脚本，输出 prometheus 的格式数据：
```shell
#!/bin/sh

echo '# HELP demo_http_requests_total Total number of http api requests'
echo '# TYPE demo_http_requests_total counter'
echo 'demo_http_requests_total{api="add_product"} 4633433'
```
其中 `#` 注释的部分，其实会被 categraf 忽略，不要也罢，prometheus 协议的数据具体的格式，请大家参考 prometheus 官方文档


# 部署场景
一般在复合型用途或独立的虚拟机启用此插件。

# 前置条件
```
1.需使用人解读每个脚本或程序的逻辑，其脚本或程序顶部有大概作用的描述。
```

# 配置场景
本配置启用或数据定义如下功能：
增加自定义标签，可通过自定义标签筛选数据及更加精确的告警推送。
响应超时时间为5秒。
commands字段正确应用脚本所在位置。

# 修改exec.toml文件配置
```
[root@aliyun input.exec]# vi exec.toml

# # collect interval
# interval = 15

[[instances]]
# # commands, support glob
commands = [
     "/opt/categraf/scripts/*/collect_*.sh"
     #"/opt/categraf/scripts/*/collect_*.py"
     #"/opt/categraf/scripts/*/collect_*.go"
     #"/opt/categraf/scripts/*/collect_*.lua"
     #"/opt/categraf/scripts/*/collect_*.java"
     #"/opt/categraf/scripts/*/collect_*.bat"
     #"/opt/categraf/scripts/*/collect_*.cmd"
     #"/opt/categraf/scripts/*/collect_*.ps1"
]

# # timeout for each command to complete
# timeout = 5

# # interval = global.interval * interval_times
# interval_times = 1

# # mesurement,labelkey1=labelval1,labelkey2=labelval2 field1=1.2,field2=2.3
data_format = "influx"
```

#  测试配置
```
以cert/collect_cert_expiretime.sh为例：
sh /opt/categraf/cert/collect_cert_expiretime.sh 出现：
cert,cloud=huaweicloud,region=huabei-beijing-4,azone=az1,product=cert,domain_name=www.baidu.com expire_days=163
cert,cloud=huaweicloud,region=huabei-beijing-4,azone=az1,product=cert,domain_name=www.weibo.com expire_days=85
cert,cloud=huaweicloud,region=huabei-beijing-4,azone=az1,product=cert,domain_name=www.csdn.net expire_days=281
```

# 重启服务
```
重启categraf服务生效
systemctl daemon-reload && systemctl restart categraf && systemctl status categraf

查看启动日志是否有错误
journalctl -f -n 500 -u categraf | grep "E\!" | grep "W\!"
```

# 检查数据呈现
如图：
![image](https://user-images.githubusercontent.com/12181410/220940504-04c47faa-790a-42c1-b3dd-1510ae55c217.png)

# 告警规则
```
脚本作用不同，规则就不同，先略过。
```

# 监控图表
```
脚本作用不同，规则就不同，先略过。
```

# 故障自愈
```
脚本作用不同，规则就不同，先略过。
```
