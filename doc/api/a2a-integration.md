# 开放 A2A 接口给第三方接入（配置说明）

n9e 内置的 AI 助手可以作为一个标准 [A2A（Agent-to-Agent Protocol）](https://a2a-protocol.org/)
智能体被外部系统调用——第三方用一句自然语言就能驱动 n9e 查告警、查数据、建规则、跑诊断。

本文只讲**管理员要做哪些配置**，以及要把什么信息交给第三方。协议细节由对方的 A2A
客户端负责，不需要你关心。

## 一、前置条件

1. **配置好大模型**：页面 `集成中心 → AI 配置`，确保 AI 助手在页面上能正常对话。
   A2A 走的是同一个助手，页面上不通，A2A 也不会通。
2. **确认 TokenAuth 已开启**（`etc/config.toml`，默认就是开的）：

   ```toml
   [HTTP.TokenAuth]
   Enable = true
   HeaderUserTokenKey = "X-User-Token"
   ```

   这一项关掉的话，A2A 端点会拒绝所有请求。

## 二、A2A 配置项

A2A 端点**默认开启，不需要任何配置**。要调整时改 `etc/config.toml`：

```toml
[HTTP.A2A]
# Disable = false     # 改成 true 可关闭 A2A + MCP 全部对外端点
# DisableMCP = false  # 改成 true 只关 MCP，保留 A2A
# BaseURL = ""        # 对外公布的绝对地址，如 https://n9e.example.com
```

| 配置项 | 默认 | 说明 |
|---|---|---|
| `Disable` | `false` | 关闭 A2A 与 MCP 的全部对外端点 |
| `DisableMCP` | `false` | 只关闭 MCP 端点，A2A 不受影响 |
| `BaseURL` | 空 | 第三方发现 n9e 时拿到的地址。留空时由请求的 `Host` 头推断；**部署在反向代理/负载均衡后面时建议显式配置**，否则第三方可能拿到内网地址 |

改完重启 center 生效。

## 三、反向代理放行

A2A 的端点挂在**根路径**下，不在 `/api/n9e` 前缀里。如果你的 nginx 只转发了
`/api/n9e/*`，需要额外放行两个路径：

```nginx
# 发现地址，第三方用它找到 n9e
location /.well-known/ {
    proxy_pass http://n9e_center;
}

# A2A 端点
location /a2a/ {
    proxy_pass http://n9e_center;
    proxy_http_version 1.1;

    proxy_buffering off;        # 必须：流式回答依赖它，不关会一直没输出
    proxy_read_timeout 3600s;   # 必须：一次提问可能跑几分钟
    proxy_send_timeout 3600s;
}
```

这两项超时和缓冲设置是最容易踩的坑：默认的 60s 超时会让稍复杂的提问在回答途中断开。

## 四、创建接入用的 API Token

A2A 请求以 Token 对应的用户身份执行，**n9e 的业务组权限和角色权限原样生效**。

1. 登录 n9e，右上角进入个人中心 → **API Token** → 新建，复制生成的字符串；
2. 建议为每个接入方**单独建一个账号**并按最小权限授权，不要直接用管理员账号的 Token；
3. Token 泄露等同于账号泄露，可随时在同一页面删除以吊销。

## 五、交给第三方的信息

对接时把这三样给对方即可：

| 项 | 值 |
|---|---|
| 发现地址（Agent Card） | `https://n9e.example.com/.well-known/agent-card.json` |
| 鉴权方式 | 请求头 `X-User-Token: <token>` |
| Token | 上一步生成的字符串 |

标准 A2A 客户端只要拿到发现地址，就能自动读出端点、能力清单和鉴权方式。

## 六、验证是否通了

```bash
# 1) 发现地址应返回一段 JSON，含 name、skills 等字段（这一步不需要 Token）
curl -s https://n9e.example.com/.well-known/agent-card.json

# 2) 带 Token 问一句，能返回一段回答即接入成功
curl -s -X POST https://n9e.example.com/a2a/message:send \
  -H 'Content-Type: application/json' \
  -H 'X-User-Token: <你的token>' \
  -d '{"message":{"messageId":"test-1","role":"ROLE_USER",
       "parts":[{"text":"现在有哪些正在告警的事件？"}]},
       "metadata":{"lang":"zh_CN"}}'
```

第 2 步可能要等几十秒（模型在思考和调工具），属正常现象。

| 现象 | 处理 |
|---|---|
| 返回 `unauthorized` | Token 无效/已删除，或 `HTTP.TokenAuth.Enable = false` |
| 发现地址 404 | `[HTTP.A2A] Disable = true`，或反向代理没放行 `/.well-known/` |
| 一直没有输出、60 秒左右断开 | nginx 没配 `proxy_buffering off` 和 `proxy_read_timeout` |
| 回答里说模型不可用 | 大模型没配好，先去页面上和 AI 助手对话验证 |

## 七、企业鉴权（可选）

第四节的 API Token 已经够用。如果你还有下面两类需求，可以改用 OAuth 方式接入——
接入方不再需要你分发 Token，而是用自己的账号登录后授权：

| 你的情况 | 用哪个方案 |
|---|---|
| 公司已有统一登录（Keycloak / Okta / Entra ID / Auth0 等） | **方案 A**：让 n9e 认可公司登录系统签发的凭证 |
| 没有统一登录，但想让 Claude、ChatGPT 这类通用客户端直接连 n9e | **方案 B**：让 n9e 自己充当授权方 |

两个方案可以同时开启，互不影响，API Token 也继续可用。

### 7.1 方案 A：对接公司已有的登录系统

接入方以**员工本人的账号**调用 n9e，权限和操作记录都落到这个人头上；员工离职时在公司
登录系统里停用即可，不用管 n9e 这边。

**第一步：在公司登录系统里给 n9e 加一个标识。**
需要让它签发的凭证上写明"这是给 n9e 用的"，起个名字比如 `n9e-a2a-rs`。
以 Keycloak 为例：`Client scopes → 对应 scope → Mappers → Add → Audience`，
`Included Custom Audience` 填 `n9e-a2a-rs`，勾选 *Add to access token*。
Auth0 / Entra ID 等在各自的"API / 受众"设置里配同样的东西，具体名称各家不同。

**第二步：改 n9e 配置文件 `etc/config.toml`，然后重启 center。**

```toml
[HTTP.RSAuth]
Enable = true
Audience = "n9e-a2a-rs"   # 必须和第一步在登录系统里填的完全一致
Provider = "oidc"         # 公司登录系统是 OIDC 协议时用这个（大多数情况）
```

**第三步：在 n9e 页面配置单点登录。**
`系统设置 → 单点登录 → OIDC`：

- 打开开关；
- 填公司登录系统的地址、ClientId、ClientSecret；
- 用户名字段（Attributes → Username）一般填 `preferred_username`；
- 设置默认角色和默认团队——某个员工第一次通过这种方式调用时，n9e 会自动为他创建账号，
  用的就是这里的默认值。

保存后约 10 秒生效，这一步不用重启。

> 补充说明：
> - n9e 所在服务器必须能访问公司登录系统的地址；如果环境配了 HTTP 代理，
>   记得把该地址加进 `no_proxy`。
> - 如果公司用的不是 OIDC 而是普通 OAuth2，把 `Provider` 改成 `"oauth2"`，
>   并去 `系统设置 → 单点登录 → OAuth2` 配置。这种模式默认不校验第一步的标识，
>   安全要求较高时需要额外设置，详见 [a2a-oauth-rs.md](./a2a-oauth-rs.md)。

配好以后，交给第三方的仍然只有第五节那个发现地址——对方的客户端会自动发现该走哪个
登录系统，不需要你再告诉他什么。

### 7.2 方案 B：让 n9e 自己充当授权方

适合没有统一登录、又希望 Claude / ChatGPT 这类通用客户端"填个地址就能连"的场景。
不用事先给对方发 Token，使用者自己在客户端里点一下就完成授权。

**第一步：改 `etc/config.toml`，然后重启 center。**

```toml
[HTTP.MCPAuth]
Enable = true
# 填用户浏览器实际访问 n9e 的地址（下面第三步要填的也是它）
Issuer = "https://n9e.example.com"
```

**第二步：反向代理再放行一个路径**（在第三节两条 location 之外补上）：

```nginx
location /oauth/ {
    proxy_pass http://n9e_center;
}
```

如果对接的是 Claude / ChatGPT 这类 MCP 客户端，还要放行 `/mcp`，配置与第三节的
`/a2a/` 完全一样（同样要关缓冲、调大超时）：

```nginx
location /mcp {
    proxy_pass http://n9e_center;
    proxy_http_version 1.1;

    proxy_buffering off;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

**第三步：告诉使用者在客户端里填什么地址。**

按客户端类型二选一，`n9e.example.com` 换成你自己的域名：

| 客户端类型 | 填这个地址 |
|---|---|
| Claude、ChatGPT 等 MCP 客户端（界面上通常叫"添加自定义连接器 / Add custom connector"，要求填一个 MCP 服务器地址） | `https://n9e.example.com/mcp` |
| 标准 A2A 客户端 | `https://n9e.example.com/.well-known/agent-card.json`；部分客户端只要根地址 `https://n9e.example.com`，会自己补后面的路径 |

这个地址要满足三个条件，否则授权走不通：

1. **是使用者的浏览器能直接打开的地址**——授权过程要在浏览器里登录 n9e 并点确认，
   所以不能填只有服务器内网能访问的 IP、容器名或 `127.0.0.1`；
2. **域名、协议、端口与第一步的 `Issuer` 完全一致**——`Issuer` 填 `https://n9e.example.com`，
   这里就不能填 `http://` 或带端口的写法；
3. **用 HTTPS**——多数客户端出于安全限制不接受 `http://` 的远程服务器地址
   （仅本机调试的 `localhost` 例外）。

填完之后使用者会经历：客户端自动发现 n9e 是授权方 → 浏览器弹出 n9e 登录页
（已登录则跳过）→ 显示授权确认页，点"允许" → 回到客户端，连接完成。
此后该客户端就以这个人的账号身份调用 n9e，权限与他登录页面时一致。

> 多实例部署（多个 center 前面挂负载均衡）时，`Issuer` 一定要**显式填写且各实例保持一致**，
> 不能留空——留空时每个实例按收到的请求各自推断，可能不一致，导致在 A 实例上完成的
> 授权到 B 实例就不认。签名密钥由各实例共享的数据库统一管理，不用额外配置。

