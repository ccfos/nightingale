# tomcat

tomcat 采集器，是读取 tomcat 的管理侧接口 `/manager/status/all` 这个接口需要鉴权。修改 `tomcat-users.xml` ，增加下面的内容：

```xml
<role rolename="admin-gui" />
<user username="tomcat" password="s3cret" roles="manager-gui" />
```

此外，还需要注释文件**webapps/manager/META-INF/context.xml**的以下内容，
```xml
  <Valve className="org.apache.catalina.valves.RemoteAddrValve"
         allow="127\.\d+\.\d+\.\d+|::1|0:0:0:0:0:0:0:1" />
```

否则 tomcat 会报以下错误，导致 tomcat 采集器无法采集到数据。

```html
403 Access Denied
You are not authorized to view this page.

By default the Manager is only accessible from a browser running on the same machine as Tomcat. If you wish to modify this restriction, you'll need to edit the Manager's context.xml file.
```

## Configuration

配置文件在 `conf/input.tomcat/tomcat.toml`

```toml
[[instances]]
## URL of the Tomcat server status
url = "http://127.0.0.1:8080/manager/status/all?XML=true"

## HTTP Basic Auth Credentials
username = "tomcat"
password = "s3cret"

## Request timeout
# timeout = "5s"

# # interval = global.interval * interval_times
# interval_times = 1

# important! use global unique string to specify instance
# labels = { instance="192.168.1.2:8080", url="-" }

## Optional TLS Config
# use_tls = false
# tls_min_version = "1.2"
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = true
```

## 监控大盘

夜莺内置了 tomcat 仪表盘，克隆到自己的业务组下使用即可。
