# 前端接口

`POST /api/portal/auth/login`

校验用户登录信息的接口，is_ldap=0表示不使用LDAP账号验证，is_ldap=1表示使用LDAP账号验证

```
{
    "username": "",
    "password": "",
    "is_ldap": 0
}
```

---

`GET /api/portal/auth/logout`

退出当前账号，如果请求成功，前端需要跳转到登录页面

---

`GET /api/portal/self/profile`

获取个人信息，可以用此接口校验用户是否登录了

---

`PUT /api/portal/self/profile`

更新个人信息

```
{
    "dispname": "",
    "phone": "",
    "email": "",
    "im": ""
}
```

---

`PUT /api/portal/self/password`

更新个人密码，新密码输入两次做校验，在前端完成

```
{
    "oldpass": "",
    "newpass": ""
}
```

---

`GET /api/portal/user`

获取用户列表，支持搜索，搜索条件参数是query，每页显示条数是limit，页码是p，如果当前用户是root，则展示相关操作按钮，如果不是，则所有按钮不展示，只是查看

---

`POST /api/portal/user`

root账号新增一个用户，is_root字段表示新增的这个用户是否是个root

```
{
    "username": "",
    "password": "",
    "dispname": "",
    "phone": "",
    "email": "",
    "im": "",
    "is_root": 0
}
```

---

`GET /api/portal/user/:id/profile`

获取某个人的信息

---

`PUT /api/portal/user/:id/profile`

root账号修改某人的信息

```
{
    "dispname": "",
    "phone": "",
    "email": "",
    "im": "",
    "is_root": 0
}
```

---

`PUT /api/portal/user/:id/password`

root账号重置某人的密码，输入两次新密码保证一致的校验由前端来做

```
{
    "password": ""
}
```

---

`DELETE /api/portal/user/:id`

root账号来删除某个用户

---

`GET /api/portal/team`

获取团队列表，支持搜索，搜索条件参数是query，每页显示条数是limit，页码是p

---

`POST /api/portal/team`

创建团队，mgmt=0表示成员管理制，mgmt=1表示管理员管理制，admins是团队管理员的id列表，members是团队普通成员的id列表

```
{
    "ident": "",
    "name": "",
    "mgmt: 0,
    "admins": [],
    "members": []
}
```

---

`PUT /api/portal/team/:id`

修改团队信息

```
{
    "ident": "",
    "name": "",
    "mgmt: 0,
    "admins": [],
    "members": []
}
```

---

`DELETE /api/portal/team/:id`

删除团队

---

`GET /api/portal/endpoint`

获取endpoint列表，用于【服务树】-【对象列表】页面，该页展示endpoint列表，搜索条件参数是query，每页显示条数是limit，页码是p，如果要做批量筛选，则同时要指定用哪个字段(参数名字是field)来筛选，只支持ident和alias，批量筛选的内容是batch，即batch和field一般是同时出现的

---

`POST /api/portal/endpoint`

导入endpoint，要求传入列表，每一条是ident::alias拼接在一起

```
{
    "endpoints": []
}
```

---

`PUT /api/portal/endpoint/:id`

修改一个endpoint的alias信息

```
{
    "alias": ""
}
```

---

`DELETE /api/portal/endpoint`

删除多个endpoint，ids参数放到request body里

```
{
    "ids": [10000, 200000]
}
```

---

`GET /api/portal/endpoints/bindings`

查询endpoint的绑定关系，QueryString：idents，逗号分隔多个

---

`GET /api/portal/endpoints/bynodeids`

根据节点id查询挂载了哪些endpoint，QueryString：ids，逗号分隔的多个节点id

---

`GET /api/portal/tree`

查询整颗服务树

---

`GET /api/portal/tree/search`

根据节点路径(path)查询服务树子树

---

`POST /api/portal/node`

创建服务树节点，pid表示父节点id，leaf=0表示非叶子节点，leaf=1表示叶子节点，note是备注信息

```
{
    "pid": 0,
    "name": "",
    "leaf": 0,
    "note": ""
}
```

---

`PUT /api/portal/node/:id/name`

服务树节点改名

```
{
    "name": ""
}
```

---

`DELETE /api/portal/node/:id`

删除服务树节点

---

`GET /api/portal/node/:id/endpoint`

获取节点下面的endpoint列表，查询字符串使用query，每页展示多少条使用limit，页码使用p，如要批量筛选，一行一个，使用batch，同时必须指定field，即使用哪个字段进行批量筛选，有ident和alias可选

---

`POST /api/portal/node/:id/endpoint-bind`

绑定一批endpoint到当前节点，del_old=1表示同时删除老的挂载关系

```
{
    "idents": [],
    "del_old": 0
}
```

---

`POST /api/portal/node/:id/endpoint-unbind`

解绑endpoint和节点的挂载关系

```
{
    "idents": []
}
```

---

`GET /api/portal/nodes/search`

搜索节点，limit表示最多返回多少条，query是搜索条件

---

`GET /api/portal/nodes/leafids`

获取节点对应的叶子节点的id，参数是ids，逗号分隔的多个节点id

---

`GET /api/portal/nodes/pids`

获取节点对应的父、祖节点的id，参数是ids，逗号分隔的多个节点id

---

`GET /api/portal/nodes/byids`

查询节点的信息，参数是ids，逗号分隔的多个节点id，返回这多个节点的信息

---

`GET /api/portal/node/:id/maskconf`

获取报警屏蔽列表，因为已经是某个节点下的了，量比较少，后端不分页

---

`POST /api/portal/maskconf`

创建一个报警屏蔽策略

```
{
    "nid": 0,
    "endpoints": [],
    "metric": "",
    "tags": "",
    "cause": "",
    "btime": 1563838361,
    "etime": 1563838461
}
```

---

`PUT /api/portal/maskconf/:id`

修改一个报警屏蔽策略

```
{
    "endpoints": [],
    "metric": "",
    "tags": "",
    "cause": "",
    "btime": 1563838361,
    "etime": 1563838461
}
```

---

`DELETE /api/portal/maskconf/:id`

删除一个报警屏蔽策略

---

`GET /api/portal/node/:id/screen`

获取screen列表，因为已经是某个节点下的了，量比较少，后端不分页

---

`POST /api/portal/node/:id/screen`

创建screen

```
{
    "name": ""
}
```

---

`PUT /api/portal/screen/:id`

修改screen，其中node_id顺带也可以修改，这样screen相当于直接挪动了挂载节点

```
{
    "name": "",
    "node_id": 0
}
```

---

`DELETE /api/portal/screen/:id`

删除某个screen

---

`GET /api/portal/screen/:id/subclass`

获取screen下面的子类，返回的subclass按照weight字段排序

---

`POST /api/portal/screen/:id/subclass`

创建subclass

```
{
    "name": "",
    "weight": 0
}
```

---

`PUT /api/portal/subclass`

批量修改subclass

```
[
    {
        "id": 1,
        "name": "a",
        "weight": 1
    },
    {
        "id": 2,
        "name": "b",
        "weight": 0
    }
]
```

---

`DELETE /api/portal/subclass/:id`

删除某个subclass

---

`PUT /api/portal/subclasses/loc`

修改subclass的location，即所属的screen

```
[
    {
        "id": 1,
        "screen_id": 1
    },
    {
        "id": 2,
        "screen_id": 1
    }
]
```

---

`GET /api/portal/subclass/:id/chart`

获取chart列表，根据chart的weight排序，不分页

---

`POST /api/portal/subclass/:id/chart`

创建chart

```
{
    "configs": "",
    "weight": 0
}
```

---

`PUT /api/portal/chart/:id`

修改某个chart的信息

```
{
    "subclass_id": 1,
    "configs": ""
}
```

---

`DELETE /api/portal/chart/:id`

删除某个chart

---

`PUT /api/portal/charts/weights`

修改chart的排序权重

```
{
    "id": 1,
    "weight": 9
}
```

---

`GET /api/portal/tmpchart`

获取临时图，参数是QueryString：ids，逗号分隔的多个id

---

`POST /api/portal/tmpchart`

创建一个临时图，返回生成的临时图的id列表

```
[
    {
        "configs": ""
    },
    {
        "configs": ""
    }
]
```

---

`GET /api/portal/event/cur`

获取某个节点下的未恢复报警列表，QueryString：

- 节点路径：nodepath
- 开始时间：stime
- 结束时间：etime
- 每页条数：limit
- 查询条件：query
- 优先级：priorities，逗号分隔的多个
- 发送类型：sendtypes，逗号分隔的多个

---

`GET /api/portal/event/cur/:id`

获取当前某一个未恢复的报警

---

`DELETE /api/portal/event/cur/:id`

删除当前某一个未恢复的报警

---

`POST /api/portal/event/curs/claim`

认领某一些未恢复的告警，避免告警升级到老板那里，id和nodepath只能传入一个，不能同时传入，也不能一个都不传，业务上的语义是：要么认领某一个告警事件，要么认领某个节点下的所有告警事件

```
{
    "id": 1,
    "nodepath": ""
}
```

---

`GET /api/portal/event/his`

获取某个节点下的所有报警列表，QueryString：

- 节点路径：nodepath
- 开始时间：stime
- 结束时间：etime
- 每页条数：limit
- 查询条件：query
- 优先级：priorities，逗号分隔的多个
- 发送类型：sendtypes，逗号分隔的多个
- 事件类型：type

---

`GET /api/portal/event/his/:id`

获取某个历史告警事件

---

`GET /api/portal/collect/list`

获取某个节点下面配置的采集策略，必传QueryString: nid表示节点id，后端不分页，返回全部

---

`GET /api/portal/collect`

获取单个采集配置的详情，QueryString：type和id，都必传，type是port、proc、log之一。

---

`POST /api/portal/collect`

创建一个采集策略

```
{
    "type": "",
    "data": {
        # 端口、进程、日志配置不同，参阅model/collect.go
    }
}
```

---

`PUT /api/portal/collect`

修改一个采集策略

```
{
    "type": "",
    "data": {
        # 端口、进程、日志配置不同，参阅model/collect.go
    }
}
```

---

`DELETE /api/portal/collect`

删除采集策略

```
{
    "type": "",
    "ids": []
}
```

---

`POST /api/portal/collect/check`

校验用户输入的数据是否匹配正则，跟商业版本数据结构一致

---

`POST /api/portal/stra`

新增告警策略，跟商业版本数据结构一致

---

`PUT /api/portal/stra`

修改告警策略，跟商业版本数据结构一致

---

`DELETE /api/portal/stra`

删除告警策略，跟商业版本数据结构一致

---

`GET /api/portal/stra`

获取告警策略列表，跟商业版本数据结构一致

---

`GET /api/portal/stra/:id`

获取单个告警策略，跟商业版本数据结构一致