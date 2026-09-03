# Use cases
```
Used for special or custom monitoring of specific business scenarios not covered by the scripts in the exec directory of the input plugin library.
After the monitoring script collects data, it prints the data to stdout in the corresponding format. categraf captures the stdout content, parses it, and sends it to the server.
Three script output formats are supported: influx, falcon, and prometheus. Tell Categraf which one to use via the `data_format` setting in exec.toml.
data_format has 3 possible values, used as follows:
```

## influx

Specification for the influx format:
```
measurement,labelkey1=labelval1,labelkey2=labelval2 field1=1.2,field2=2.3
```
- First comes the measurement, which represents a category of monitoring metrics, e.g. connections;
- The measurement is followed by a comma, then the labels; if there are no labels, no comma is needed after the measurement
- Labels are in k=v format; multiple labels are separated by commas, e.g. region=beijing,env=test
- The labels are followed by a space
- After the space come the fields; multiple fields are separated by commas
- Fields are in name=value format; in categraf the value can only be numeric
  Finally, the measurement and each field name are concatenated to form the metric name

## falcon
The Open-Falcon format looks like this, for example:

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
The timestamp, step, and counterType fields are simply ignored when categraf processes the data; endpoint is reported as part of the labels.

## prometheus
Everyone is familiar with the prometheus format. For example, here is a monitoring script that outputs data in prometheus format:
```shell
#!/bin/sh

echo '# HELP demo_http_requests_total Total number of http api requests'
echo '# TYPE demo_http_requests_total counter'
echo 'demo_http_requests_total{api="add_product"} 4633433'
```
The lines starting with `#` are comments and are actually ignored by categraf, so they can be omitted. For details on the prometheus protocol data format, please refer to the official prometheus documentation.


# Deployment scenario
This plugin is generally enabled on multi-purpose or standalone virtual machines.

# Prerequisites
```
1. The user needs to understand the logic of each script or program; a brief description of its purpose is provided at the top of each script or program.
```

# Configuration scenario
This configuration enables or defines the following:
Add custom labels, which can be used to filter data and enable more precise alert delivery.
The response timeout is 5 seconds.
The commands field should correctly point to the location of the scripts.

# Edit the exec.toml configuration file
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

# # measurement,labelkey1=labelval1,labelkey2=labelval2 field1=1.2,field2=2.3
data_format = "influx"
```

#  Testing the configuration
```
Taking cert/collect_cert_expiretime.sh as an example:
running sh /opt/categraf/cert/collect_cert_expiretime.sh prints:
cert,cloud=huaweicloud,region=huabei-beijing-4,azone=az1,product=cert,domain_name=www.baidu.com expire_days=163
cert,cloud=huaweicloud,region=huabei-beijing-4,azone=az1,product=cert,domain_name=www.weibo.com expire_days=85
cert,cloud=huaweicloud,region=huabei-beijing-4,azone=az1,product=cert,domain_name=www.csdn.net expire_days=281
```

# Restart the service
```
Restart the categraf service to apply the changes
systemctl daemon-reload && systemctl restart categraf && systemctl status categraf

Check the startup log for errors
journalctl -f -n 500 -u categraf | grep "E\!" | grep "W\!"
```

# Verify the data
As shown below:
![image](https://user-images.githubusercontent.com/12181410/220940504-04c47faa-790a-42c1-b3dd-1510ae55c217.png)
