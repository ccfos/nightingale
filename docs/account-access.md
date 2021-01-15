## 登陆相关

#### 来源地址限制

IP地址的获取顺序

- http header "X-Forwarded-For"
- http header "X-Real-Ip"
- http request RemoteAddr

nginx 代理配置客户端地址

```
# https://www.nginx.com/resources/wiki/start/topics/examples/forwarded/
proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
```
