# Nginx

There are several ways to monitor Nginx; the most recommended one is the vts plugin:

**[http_stub_status_module](https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx/README.md)**

Sample configuration:

```toml
[[instances]]
## An array of Nginx stub_status URI to gather stats.
urls = [
#    "http://192.168.0.216:8000/nginx_status",
#    "https://www.baidu.com/ngx_status"
]

## append some labels for series
# labels = { region="cloud", product="n9e" }

## interval = global.interval * interval_times
# interval_times = 1

## Set response_timeout (default 5 seconds)
response_timeout = "5s"

## Whether to follow redirects from the server (defaults to false)
# follow_redirects = false

## Optional HTTP Basic Auth Credentials
#username = "admin"
#password = "admin"

## Optional headers
# headers = ["X-From", "categraf", "X-Xyz", "abc"]

## Optional TLS Config
# use_tls = false
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = false
```

**[nginx_upstream_check](https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx_upstream_check/README.md)**

Sample configuration:

```toml
[[instances]]
targets = [
    # "http://127.0.0.1/status?format=json",
    # "http://10.2.3.56/status?format=json"
]

# # append some labels for series
# labels = { region="cloud", product="n9e" }

# # interval = global.interval * interval_times
# interval_times = 1

## Set http_proxy (categraf uses the system wide proxy settings if it's is not set)
# http_proxy = "http://localhost:8888"

## Interface to use when dialing an address
# interface = "eth0"

## HTTP Request Method
# method = "GET"

## Set timeout (default 5 seconds)
# timeout = "5s"

## Whether to follow redirects from the server (defaults to false)
# follow_redirects = false

## Optional HTTP Basic Auth Credentials
# username = "username"
# password = "pa$$word"

## Optional headers
# headers = ["X-From", "categraf", "X-Xyz", "abc"]

## Optional TLS Config
# use_tls = false
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
## Use TLS but skip chain & host verification
# insecure_skip_verify = false
```

**[nginx vts](https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx_vts/README.md)**

nginx_vts already supports exposing data in Prometheus format, so this collection plugin is actually no longer needed. Simply use categraf's prometheus collection plugin to read the Prometheus data exposed by nginx_vts. Sample configuration:

```toml
[[instances]]
urls = [
  "http://IP:PORT/vts/format/prometheus"
]
labels = {job="nginx-vts"}
```

# nginx_upstream_check plugin
### Use Cases
Typically used when business systems rely on a proxy service for external routing and mapping; it is one of the most common and most important proxy tools in operations.

### Deployment
This plugin should be enabled on the virtual machine where the nginx service is running.

### How It Works

- This collection plugin reads the status output of [nginx_upstream_check](https://github.com/yaoweibin/nginx_upstream_check_module). [nginx_upstream_check](https://github.com/yaoweibin/nginx_upstream_check_module) periodically checks whether each server in an upstream is alive. If the check fails, the server is marked as `down`; if it succeeds, it is marked as `up`.

### Notes
- Since TSDBs generally cannot handle strings, Categraf converts the values: `down` becomes 2, `up` becomes 1, and other states become 0, represented by the metric `nginx_upstream_check_status_code`. Therefore, we may need an alert rule like the following:

### Prerequisites
#### Prerequisite 1: the nginx service must be built with the nginx_upstream_check_module module
```
Building the module from source is recommended. If you are not sure which modules to install, you can refer to:
cd /opt/nginx-1.20.1 && ./configure \
--prefix=/usr/share/nginx \
--sbin-path=/usr/sbin/nginx \
--modules-path=/usr/lib64/nginx/modules \
--conf-path=/etc/nginx/nginx.conf \
--error-log-path=/var/log/nginx/error.log \
--http-log-path=/var/log/nginx/access.log \
--http-client-body-temp-path=/var/lib/nginx/tmp/client_body \
--http-proxy-temp-path=/var/lib/nginx/tmp/proxy \
--http-fastcgi-temp-path=/var/lib/nginx/tmp/fastcgi \
--http-uwsgi-temp-path=/var/lib/nginx/tmp/uwsgi \
--http-scgi-temp-path=/var/lib/nginx/tmp/scgi \
--pid-path=/var/run/nginx.pid \
--lock-path=/run/lock/subsys/nginx \
--user=nginx \
--group=nginx \
--with-compat \
--with-threads \
--with-http_addition_module \
--with-http_auth_request_module \
--with-http_dav_module \
--with-http_flv_module \
--with-http_gunzip_module \
--with-http_gzip_static_module \
--with-http_mp4_module \
--with-http_random_index_module \
--with-http_realip_module \
--with-http_secure_link_module \
--with-http_slice_module \
--with-http_ssl_module \
--with-http_stub_status_module \
--with-http_sub_module \
--with-http_v2_module \
--with-mail \
--with-mail_ssl_module \
--with-stream \
--with-stream_realip_module \
--with-stream_ssl_module \
--with-stream_ssl_preread_module \
--with-select_module \
--with-poll_module \
--with-file-aio \
--with-http_xslt_module=dynamic \
--with-http_image_filter_module=dynamic \
--with-http_perl_module=dynamic \
--with-stream=dynamic \
--with-mail=dynamic \
--with-http_xslt_module=dynamic \
--add-module=/etc/nginx/third-modules/nginx_upstream_check_module \
--add-module=/etc/nginx/third-modules/ngx_devel_kit-0.3.0 \
--add-module=/etc/nginx/third-modules/lua-nginx-module-0.10.13 \
--add-module=/etc/nginx/third-modules/nginx-module-vts \
--add-module=/etc/nginx/third-modules/ngx-fancyindex-0.5.2

# adjust according to the number of CPU cores
make -j2
make install

Note: the third-party modules nginx_upstream_check_module, lua-nginx-module and nginx-module-vts are required dependencies for the related plugins.
```

#### Prerequisite 2: enable the check_status configuration in nginx
```
[root@aliyun categraf]# cat /etc/nginx/conf.d/nginx-upstream.domains.com.conf
server {
    listen 80;
    listen 443 ssl;
    server_name nginx-upstream.domains.com;
    include /etc/nginx/ssl_conf/domains.com.conf;

    location / {
        check_status;
        include /etc/nginx/ip_whitelist.conf;
    }

    access_log /var/log/nginx/nginx-upstream.domains.com.access.log main;
    error_log /var/log/nginx/nginx-upstream.domains.com.error.log warn;
}
```
Visiting https://nginx-upstream.domains.com?format=json in a browser shows:
![image](https://user-images.githubusercontent.com/12181410/220912157-57f485de-6b4e-4ca4-869d-871244aabde1.png)

Visiting https://nginx-upstream.domains.com in a browser shows:
![image](https://user-images.githubusercontent.com/12181410/220909354-fc8ba53d-2384-41d3-8def-4447a104fb3c.png)

#### Prerequisite 3: add the check directives to each domain configuration that needs upstream monitoring
For example:
```
[root@aliyun upstream_conf]# cat upstream_n9e.conf
upstream n9e {
    server 127.0.0.1:18000 weight=10 max_fails=2 fail_timeout=5s;

    check interval=3000 rise=2 fall=5 timeout=1000 type=tcp default_down=false port=18000;
    check_http_send "HEAD / HTTP/1.0\r\n\r\n";
    check_http_expect_alive http_2xx http_3xx;
}

[root@aliyun upstream_conf]# cat upstream_n9e_server_api.conf
upstream n9e-server-api {
    server 127.0.0.1:19000 weight=10 max_fails=2 fail_timeout=5s;

    check interval=3000 rise=2 fall=5 timeout=1000 type=tcp default_down=false port=19000;
    check_http_send "HEAD / HTTP/1.0\r\n\r\n";
    check_http_expect_alive http_2xx http_3xx;
}

[root@aliyun upstream_conf]# cat upstream_vm.conf
upstream vm {
    server 127.0.0.1:8428 weight=10 max_fails=2 fail_timeout=5s;
    keepalive 20;

    check interval=3000 rise=2 fall=5 timeout=1000 type=tcp default_down=false port=8428;
    check_http_send "HEAD / HTTP/1.0\r\n\r\n";
    check_http_expect_alive http_2xx http_3xx;
}

```

### Configuration Scenario
```
This configuration enables or defines the following features:
Add custom labels, so that data can be filtered by custom labels and alerts can be pushed more precisely.
The response timeout is 5 seconds.
Fill the urls field with the domain name defined in Prerequisite 2.
```

### Modify the nginx.toml configuration file
```
[root@aliyun conf]# cat input.nginx_upstream_check/nginx_upstream_check.toml

# # collect interval
# interval = 15

[[instances]]
# This is the most critical setting: the endpoint URL that exposes the status information
targets = [
    "https://nginx-upstream.domains.com/?format=json"
]

# Please pay attention to the labels configuration
# If Categraf and Nginx run on the same machine, the target may be configured as 127.0.0.1
# If Nginx runs on multiple machines, each with its own Categraf collecting the local Nginx status,
# the time series data may end up with identical labels and be hard to distinguish. Of course, Categraf
# automatically attaches the ident label, which identifies the local hostname
# If the ident label is not enough, you can use the labels configuration below to attach labels such as instance, region, etc.

# # append some labels for series
labels = { cloud="my-cloud", region="my-region",azone="az1", product="my-product" }

# # interval = global.interval * interval_times
# interval_times = 1

### Set http_proxy (categraf uses the system wide proxy settings if it's is not set)
# http_proxy = "http://localhost:8888"

### Interface to use when dialing an address
# interface = "eth0"

### HTTP Request Method
# method = "GET"

### Set timeout (default 5 seconds)
# timeout = "5s"

### Whether to follow redirects from the server (defaults to false)
# follow_redirects = false

### Optional HTTP Basic Auth Credentials
# username = "username"
# password = "pa$$word"

### Optional headers
# headers = ["X-From", "categraf", "X-Xyz", "abc"]

### Optional TLS Config
# use_tls = false
# tls_ca = "/etc/categraf/ca.pem"
# tls_cert = "/etc/categraf/cert.pem"
# tls_key = "/etc/categraf/key.pem"
### Use TLS but skip chain & host verification
# insecure_skip_verify = false
```

### Test the configuration
```
./categraf --test --inputs nginx_upstream_check

```
### Restart the service
```
Restart the categraf service for the changes to take effect
systemctl daemon-reload && systemctl restart categraf && systemctl status categraf

Check the startup logs for errors
journalctl -f -n 500 -u categraf | grep "E\!" | grep "W\!"
```

### Verify the data
After 1-2 minutes, the data will show up in the charts, as shown below:
![image](https://user-images.githubusercontent.com/12181410/220914337-f97f6fd5-4763-4174-b64c-131aecf6664f.png)


### Alert Rule Configuration
```
The key thing to check is whether the backend is abnormal. nginx_upstream_check_status_code returning 1 means healthy and returning 2 means abnormal (as can be seen from the chart above in actual testing).
nginx_upstream_check_status_code!=1 should be treated as abnormal and trigger an alert immediately: severity level 1, evaluation frequency 60 seconds, duration 60 seconds, recovery observation period 2 minutes, repeat notification interval 5 minutes, maximum number of notifications 0 (unlimited), sending the alert via the WeCom app and phone voice channels to the system operations team. This rule applies all day, Monday through Sunday.
```
