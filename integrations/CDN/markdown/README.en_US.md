
## Configuration
```shell
[[instances]]
  targets = [
   "https://www.baidu.com"
  ]

# # Custom DNS
# # Custom DNS server address
# dns = ""
# # Protocol used by the custom DNS
# dns_Protocol = "udp"
# # Custom DNS timeout
# dns_timeout_ms = 1000

# # Connection settings
   #  Connection timeout
   connect_timeout_ms = 3000
   # TLS handshake timeout
   tls_handshake_timeout_ms = 1000

# # HTTP settings
   # HTTP request method: HEAD GET POST 
   method = "GET"
# # Response content encoding; supports GBK GB2312 HZGB2312 GBK18030 BIG5, default: UTF8
   encode = "UTF8"
   # # HTTP request headers
#  headers = { Authorization="", X-Forwarded-For="", Host=""}
   headers = { User-Agent="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/104.0.0.0 Safari/537.36"}
   ## Request body 
   paylaod = ""
   ## Basic auth settings; can also be configured directly in the headers
# username = ""
# password = ""

# # Response settings
   # Match mode: full match or partial match; supports complete or substring
   # If not configured, substring is used
    match_pattern = "substring"
   # Expected response content
   expect_response_string = "ok"
   # Expected response status code
   expect_response_status_code = 200
```

## Metric descriptions
- cdn_dns_request: time spent on DNS resolution when requesting the resource URL
- cdn_tcp_connect: time spent establishing the TCP connection when requesting the resource URL
- cdn_tls_handshake: time spent on the TLS handshake when requesting the resource URL
- cdn_first_byte: time to first byte when requesting the resource URL (from request to first-byte response)
- cdn_total_cost: total time spent on the resource request
- cdn_response_status_code: response status code of the resource request
- cdn_probe_result: probe result

### cdn_probe_result codes
```
	Success          0  Probe succeeded
	ConnectionFailed 1  Connection failed
	Timeout          2  Timed out
	DNSError         3  DNS resolution failed
	AddressError     4  Address error
	BodyMismatch     5  Response content mismatch
	CodeMismatch     6  Response status code mismatch
```
