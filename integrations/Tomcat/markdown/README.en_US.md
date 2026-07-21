# tomcat

The tomcat collector reads Tomcat's management endpoint `/manager/status/all`, which requires authentication. Edit `tomcat-users.xml` and add the following content:

```xml
<role rolename="admin-gui" />
<user username="tomcat" password="s3cret" roles="manager-gui" />
```

In addition, you need to comment out the following content in the file **webapps/manager/META-INF/context.xml**:
```xml
  <Valve className="org.apache.catalina.valves.RemoteAddrValve"
         allow="127\.\d+\.\d+\.\d+|::1|0:0:0:0:0:0:0:1" />
```

Otherwise Tomcat will return the following error, and the tomcat collector will not be able to collect any data.

```html
403 Access Denied
You are not authorized to view this page.

By default the Manager is only accessible from a browser running on the same machine as Tomcat. If you wish to modify this restriction, you'll need to edit the Manager's context.xml file.
```

## Configuration

The configuration file is located at `conf/input.tomcat/tomcat.toml`

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
