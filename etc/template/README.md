# 告警消息模版文件

模版中可以使用的变量参考`AlertCurEvent`对象
模版语法如何使用可以参考[html/template](https://pkg.go.dev/html/template)

## 如何在告警模版中添加监控详情url

假设web的地址是http://127.0.0.1:18000/, 实际使用时用web地址替换该地址

在监控模版中添加以下行:

* dingtalk / wecom / feishu
```markdown
[监控详情](http://127.0.0.1:18000/metric/explorer?promql={{ .PromQl | escape }})
```

* mailbody

```html
<tr>
  <th>监控详情：</th>
  <td>
    <a href="http://127.0.0.1:18000/metric/explorer?promql={{ .PromQl | escape }}" target="_blank">点击查看</a>
  </td>
</tr>
```