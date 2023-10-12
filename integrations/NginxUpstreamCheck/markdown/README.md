# 应用场景
一般用于业务系统做对外或对外路由映射时使用代理服务，是运维最常见且最重要的代理工具。

# 部署场景
需要在装有nginx服务的虚拟机启用此插件。

# 采集原理

- 该采集插件是读取 [nginx_upstream_check](https://github.com/yaoweibin/nginx_upstream_check_module) 的状态输出。[nginx_upstream_check](https://github.com/yaoweibin/nginx_upstream_check_module) 可以周期性检查 upstream 中的各个 server 是否存活，如果检查失败，就会标记为 `down`，如果检查成功，就标记为 `up`。

# 注意事项
- 由于 TSDB 通常无法处理字符串，所以 Categraf 会做转换，将 `down` 转换为 2， `up` 转换为 1，其他状态转换为 0，使用 `nginx_upstream_check_status_code` 这个指标来表示，所以，我们可能需要这样的告警规则：

# 前置条件
## 条件1：nginx服务需要启用nginx_upstream_check_module模块
```
推荐源码编译方式安装模块，如不清楚要安装哪些模块，可参考：
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

# 根据cpu核数
make -j2
make install

注意：第三方模块nginx_upstream_check_module lua-nginx-module nginx-module-vts 都是相关插件所必备的依赖。
```

## 条件2：nginx启用check_status配置
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
浏览器访问https://nginx-upstream.domains.com?format=json出现：
![image](https://user-images.githubusercontent.com/12181410/220912157-57f485de-6b4e-4ca4-869d-871244aabde1.png)

浏览器访问https://nginx-upstream.domains.com出现：
![image](https://user-images.githubusercontent.com/12181410/220909354-fc8ba53d-2384-41d3-8def-4447a104fb3c.png)

## 条件3：在需要启用upstream监控的域名配置下进行配置
例如：
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

# 配置场景
```
本配置启用或数据定义如下功能：
增加自定义标签，可通过自定义标签筛选数据及更加精确的告警推送。
响应超时时间为5秒。
urls字段填写条件2所定义好的域名。
```

# 修改nginx.toml文件配置
```
[root@aliyun conf]# cat input.nginx_upstream_check/nginx_upstream_check.toml

# # collect interval
# interval = 15

[[instances]]
# 这个配置最关键，是要给出获取 status 信息的接口地址
targets = [
    "https://nginx-upstream.domains.com/?format=json"
]

# 标签这个配置请注意
# 如果 Categraf 和 Nginx 是在一台机器上，target 可能配置的是 127.0.0.1
# 如果 Nginx 有多台机器，每台机器都有 Categraf 来采集本机的 Nginx 的 Status 信息
# 可能会导致时序数据标签相同，不易区分，当然，Categraf 会自带 ident 标签，该标签标识本机机器名
# 如果大家觉得 ident 标签不够用，可以用下面 labels 配置，附加 instance、region 之类的标签

# # append some labels for series
labels = { cloud="my-cloud", region="my-region",azone="az1", product="my-product" }

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

# 测试配置
```
./categraf --test --inputs nginx_upstream_check

```
# 重启服务
```
重启categraf服务生效
systemctl daemon-reload && systemctl restart categraf && systemctl status categraf

查看启动日志是否有错误
journalctl -f -n 500 -u categraf | grep "E\!" | grep "W\!"
```

# 检查数据呈现
等待1-2分钟后数据就会在图表中展示出来，如图：
![image](https://user-images.githubusercontent.com/12181410/220914337-f97f6fd5-4763-4174-b64c-131aecf6664f.png)


# 监控告警规则配置
```
一般查看后端是否异常为关键检查对象，nginx_upstream_check_status_code返回1代表正常，返回2代表异常（实际测试可从上图看出）。
nginx_upstream_check_status_code!=1则视为异常需立即告警，级别为一级告警，执行频率为60秒，持续时长为60秒，留观时长2分钟，重复发送频率5分钟，最大发送次数0次，使用企业微信应用及电话语音通道将告警内容发送给系统运维组，此规则运用到周一到周日全天。
```

# 监控图表配置
https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx_upstream_check/dashboards.json

# 故障自愈配置
```
先略过
```
