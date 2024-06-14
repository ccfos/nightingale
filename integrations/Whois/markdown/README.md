# whois

域名探测插件，用于探测域名的注册时间和到期时间，值为UTC0时间戳


## Configuration

最核心的配置就是 domain 配置，配置目标地址，比如想要监控一个地址：
默认保持注释状态，注释状态下，插件默认不启用

```toml
# [[instances]]
## Used to collect domain name information.
# domain = "baidu.com"
```
请注意这里配置的是域名不是URL

## 指标解释

whois_domain_createddate 域名创建时间戳
whois_domain_updateddate 域名更新时间戳
whois_domain_expirationdate 域名到期时间戳

## 注意事项
请不要将interval设置过短，会导致频繁请求timeout，没太大必要性，请尽量放长请求周期