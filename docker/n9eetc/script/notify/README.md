通过go plugin模式处理告警通知
---

相比于调用py脚本方式，该方式一般无需考虑依赖问题

### (1) 编写动态链接库逻辑

```go
package main

type inter interface {
	Descript() string
	Notify([]byte)
}

// 0、Descript 可用于该插件在 server 中的描述
// 1、在 Notify 方法中实现要处理的自定义逻辑
```

实现以上接口的 `struct` 实例即为合法 `plugin`

### (2) 构建链接库

参考 `notify.go` 实现方式，执行 `make` 后可以看到生成一个 `notify.so` 链接文件，放到 n9e 对应项目位置即可

### (3) 更新 n9e 配置

```text
[Alerting.CallPlugin]
Enable = false
PluginPath = "./etc/script/notify.so"
# 注意此处caller必须在notify.so中作为变量暴露,首字母必须大写才能暴露
Caller = "N9eCaller"
```

