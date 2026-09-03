
配置示例
```
interval=60
[[instances]]
#api_key="xxxxx"
#base_url="https://api.meraki.cn/api/v1"
#network_result_per_page=100000
#rate_limit=5

[instances.signal_4g]
url="xxx"
app_key=""
app_secret=""
app_id=""


[instances.extra_info]
url="xxxx"
query='''
probe_success{cluster="hedan-prod-elk",job="icmp",isOpen="True"}
'''
```