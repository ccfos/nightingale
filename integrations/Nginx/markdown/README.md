- 该插件依赖**nginx**的 **http_stub_status_module

# 应用场景
一般用于业务系统做对外或对外路由映射时使用代理服务，是运维最常见且最重要的代理工具。

# 部署场景
需要在装有nginx服务的虚拟机启用此插件。


# 前置条件
```
条件1：nginx服务需要启用http_stub_status_module模块

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

```
条件2：nginx启用stub_status配置。

[root@aliyun conf.d]# cat nginx.domains.com.conf
server {
    listen 80;
    listen 443 ssl;
    server_name nginx.domains.com;
    include /etc/nginx/ssl_conf/domains.com.conf;

    location / {
        stub_status on;
	    include /etc/nginx/ip_whitelist.conf;
    }

    access_log /var/log/nginx/nginx.domains.com.access.log main;
    error_log /var/log/nginx/nginx.domains.com.error.log warn;
}

浏览器访问https://nginx.domains.com出现：
Active connections: 5 
server accepts handled requests
 90837 90837   79582
Reading: 0 Writing: 1 Waiting: 4

Nginx状态解释：
Active connections Nginx正处理的活动连接数5个
server Nginx启动到现在共处理了90837个连接。
accepts Nginx启动到现在共成功创建90837次握手。
handled requests Nginx总共处理了79582次请求。
Reading Nginx读取到客户端的 Header 信息数。
Writing Nginx返回给客户端的 Header 信息数。
Waiting Nginx已经处理完正在等候下一次请求指令的驻留链接，Keep-alive启用情况下，这个值等于active-（reading + writing）。
请求丢失数=(握手数-连接数)可以看出,本次状态显示没有丢失请求。

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
[root@aliyun input.nginx]# cat nginx.toml
# # collect interval
# interval = 15

[[instances]]
## An array of Nginx stub_status URI to gather stats.
urls = [
    "https://nginx.domains.com"
]

## append some labels for series
labels = { cloud="my-cloud", region="my-region",azone="az1", product="my-product" }

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

# 测试配置
```
./categraf --test --inputs nginx

21:46:46 nginx_waiting agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud port=443 product=nginx region=huabei-beijing-4 server=nginx.devops.press 0
21:46:46 nginx_active agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud port=443 product=nginx region=huabei-beijing-4 server=nginx.devops.press 1
21:46:46 nginx_accepts agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud port=443 product=nginx region=huabei-beijing-4 server=nginx.devops.press 90794
21:46:46 nginx_handled agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud port=443 product=nginx region=huabei-beijing-4 server=nginx.devops.press 90794
21:46:46 nginx_requests agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud port=443 product=nginx region=huabei-beijing-4 server=nginx.devops.press 79458
21:46:46 nginx_reading agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud port=443 product=nginx region=huabei-beijing-4 server=nginx.devops.press 0
21:46:46 nginx_writing agent_hostname=aliyun.tjf.n9e.001 azone=az1 cloud=huaweicloud port=443 product=nginx region=huabei-beijing-4 server=nginx.devops.press 1

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
![image](https://user-images.githubusercontent.com/12181410/220639442-5d02a9ec-f0ae-48f5-91f0-4c7839b747b5.png)


# 监控告警规则配置
```
```
个人经验仅供参考：
超过2000毫秒，为P2级别，启用企业微信应用推送告警，3分钟内恢复发出恢复告警。
超过5000毫秒，为P1级别，启用电话语音告警&企业微信应用告警，3分钟内恢复发出恢复告警。
```

# 监控图表配置

https://github.com/flashcatcloud/categraf/blob/main/inputs/nginx_vts/dashboards.json

# 故障自愈配置
```
先略过
```
