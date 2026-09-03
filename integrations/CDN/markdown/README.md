
## 配置
```shell
[[instances]]
  targets = [
   "https://www.baidu.com"
  ]

# # 自定义DNS
# # 自定义dns 地址
# dns = ""
# # 自定义dns使用的协议
# dns_Protocol = "udp"
# # 自定义dns 超时时间
# dns_timeout_ms = 1000

# # 连接相关配置
   #  连接超时时间
   connect_timeout_ms = 3000
   # tls 握手超时时间
   tls_handshake_timeout_ms = 1000

# # http 相关配置
   # http 请求方法 HEAD GET POST 
   method = "GET"
# # 响应内容编码，支持GBK GB2312 HZGB2312 GBK18030 BIG5 ,default: UTF8
   encode = "UTF8"
   # # http 请求头部配置
#  headers = { Authorization="", X-Forwarded-For="", Host=""}
   headers = { User-Agent="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36"}
   ## body 配置 
   paylaod = ""
   ## basic auth配置 ,也可以直接在header中配置
# username = ""
# password = ""

# # 响应部分配置
   # 匹配方式， 完全匹配还是部分匹配, 支持complete or substring
   # 不配置 则使用substring
    match_pattern = "substring"
   # 期望返回的响应内容
   expect_response_string = "ok"
   # 期望返回的响应码
   expect_response_status_code = 200
```

## 指标说明
- cdn_dns_request 请求资源链接时DNS解析花费的时间
- cdn_tcp_connect 请求资源链接时，建立TCP连接花费的时间
- cdn_tls_handshake 请求资源链接时，TLS握手花费的时间
- cdn_first_byte 请求资源链接时，首包响应时间（从请求到首包响应的时间）
- cdn_total_cost 请求资源连接，总共花费的时间
- cdn_response_status_code 请求资源连接，响应码
- cdn_probe_result 探测结果

### cdn_probe_result 响应码说明
```
	Success          0  探测成功
	ConnectionFailed 1  连接失败
	Timeout          2  超时
	DNSError         3  DNS解析失败
	AddressError     4  地址错误
	BodyMismatch     5  响应内容不匹配
	CodeMismatch     6  响应码不匹配
```