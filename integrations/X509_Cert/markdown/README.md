# X509_Cert

证书探测插件，用于监控 TLS/SSL 证书的剩余有效期和校验状态，避免证书悄无声息地过期。

探测方式有两类，可以混着用：

- 网络探测：与目标地址握手，取回服务端出示的证书链，适合监控对外提供 HTTPS/SMTPS 服务的域名
- 本地文件：直接读取机器上的证书文件（支持通配符），适合监控 Nginx、网关等自己维护的证书

## 前置条件

- 目标机器上的 categraf 需要能访问被探测的地址（网络可达、没有被防火墙拦住）；如果只能走代理，配置 `http_proxy`
- 探测本地证书文件时，categraf 进程需要有读取该文件的权限
- 探测自签证书时打开 `insecure_skip_verify`，否则校验状态会一直是 invalid

## Configuration

categraf 的 `conf/input.x509_cert/x509_cert.toml`，核心配置就是 targets：

```toml
[[instances]]
targets = ["tcp://example.org:443", "https://www.baidu.com", "/etc/nginx/ssl/*.pem"]
labels = { env = "prod" }
```

targets 支持的写法：

| 写法 | 说明 |
| --- | --- |
| `tcp://host:port` | 与端口直接握手取证书，最通用 |
| `https://host` | HTTPS 站点，端口默认 443 |
| `smtp://host:port` | SMTP STARTTLS |
| `udp://host:port` | DTLS |
| `/path/to/cert.pem` | 本地证书文件，支持通配符 `*` |
| `file:///path/to/*.pem` | 相对路径要加 `file://` 前缀 |

其他常用配置项：

- `timeout`：建立 SSL 连接的超时时间，默认 5s
- `server_name`：SNI，一个 IP 上放了多个域名证书时需要指定
- `exclude_root_certs`：只上报叶子证书，忽略中间证书和根证书，可以显著减少指标量
- `use_tls` / `tls_cert` / `tls_key`：目标开启双向认证、要求客户端出示证书时才需要

## 指标解释

| 指标 | 说明 |
| --- | --- |
| x509_cert_expiry | 距离证书过期还有多少秒，负数表示已经过期，告警一般就看这个指标 |
| x509_cert_enddate | 证书过期时间戳（秒） |
| x509_cert_startdate | 证书生效时间戳（秒） |
| x509_cert_age | 证书已经签发了多少秒 |
| x509_cert_verification_code | 证书校验结果，0 为通过，1 为不通过（过期、域名不匹配、链不完整等） |
| x509_cert_ocsp_status_code | OCSP 吊销状态，0=good 1=revoked 2=unknown，目标返回了 OCSP stapling 时才有 |

指标上附带的标签：`target`（探测目标）、`common_name`（证书域名）、`issuer_common_name`（签发机构）、`verification`（valid/invalid）、`type`（leaf/intermediate/root）、`serial_number`、`signature_algorithm`、`public_key_algorithm` 等。

告警可以直接对 `x509_cert_expiry` 设阈值，比如剩余有效期小于 14 天（1209600 秒）就告警；也可以对 `x509_cert_verification_code == 1` 告警，覆盖证书链配置错误的场景。
