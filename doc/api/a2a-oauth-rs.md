# A2A / MCP 对接企业 IdP（OAuth 2.1 Resource Server）

让 n9e 的 a2a / mcp agent 端点接受**外置企业 IdP（Keycloak / Entra ID / Okta / Auth0 等）签发的 OAuth access token**，
作为「按用户」的凭证。各类 Agent 编排平台代表某个具体用户调用 n9e 时，携带该用户的 access token，
n9e 验 token 后把调用落到对应的本地用户——审计按人留痕、权限与该用户一致，不再共享一个机器人身份。

该档与现有 `X-User-Token`（PAT）、自签 session JWT **并列**，满足任一即放行；默认关闭，开启后不影响原有两条鉴权路径。

## 一、工作原理

- **两种 provider**（`[HTTP.RSAuth].Provider`，复用对应的 SSO 登录配置，不必再配第二个授权服务器）：
  - `oidc`（默认）：复用 **OIDC 登录**配置指向的 IdP，借其 issuer 与 JWKS 在**本地验签 JWT** access token。
  - `oauth2`：复用 **OAuth2 登录**配置指向的 IdP，校验 **opaque** access token（校验方式见 2.4 的 `RSVerifyMethod`）。
- **只校验、不签发**：n9e 只做 Resource Server（验别人发的 token），AS（授权服务器）始终是外置的 IdP。
- **如何识别 OAuth token**：请求带 `Authorization: Bearer <token>`。n9e 自签 session JWT **不带 `iss`**，且本身是 JWT：
  - `oidc` provider：只有**携带 `iss`** 的 JWT 才走 RS 校验；
  - `oauth2` provider：外部 token 是 opaque，故**非 JWT** 的 Bearer token 才走 RS 校验。
  两种情况下自签 session JWT 都仍走原路径，**回归不破**。
- **校验内容（`oidc` provider，任一不过即 401）**：
  1. 用 IdP 公钥（JWKS）验**签名**；
  2. 校验 **issuer**（须等于 OIDC 配置 IdP 的 issuer）；
  3. 校验 **audience**：token 的 `aud` 必须包含配置的 `Audience`（绑定本服务，防止 IdP 发给其它应用的 token 被重放）；
  4. 校验**过期时间** `exp`。
  （`oauth2` provider 的校验内容与是否校验 `aud` 取决于 `RSVerifyMethod`，见 2.4。）
- **用户映射**：从 IdP 响应取用户名 claim（复用对应登录配置的 `Attributes.Username`，默认 `sub`，可改 `preferred_username`），映射到本地用户。
- **自动建用户（JIT）**：查无此人时按对应登录配置的同款规则自动建用户。`oidc`：`Belong=oidc`、角色用 OIDC `DefaultRoles`、团队用 OIDC `DefaultTeams`；`oauth2`：`Belong=oauth2`、角色用 OAuth2 `DefaultRoles`（OAuth2 配置无默认团队）。已存在的用户不会重复创建。
- **权限**：沿用该用户在 n9e 自身的角色，本期不引入基于 OAuth scope 的额外授权。

## 二、配置（两处）

对接 IdP 需要改两处：① n9e 配置文件里的 `[HTTP.RSAuth]` 开关；② OIDC 登录配置（指定受信 IdP）。

### 2.1 配置文件 `etc/config.toml`：`[HTTP.RSAuth]`

```toml
[HTTP.RSAuth]
# 总开关。true 时 a2a/mcp 等走 tokenAuth 的端点开始接受外置 IdP 的 OAuth access token
Enable = true
# 本服务的资源标识；access token 的 aud 必须包含它。Enable=true 时必填，留空则 RS 校验不生效
Audience = "n9e-a2a-rs"
# 受信 IdP 协议：oidc（默认，JWT 经 JWKS 本地验签）或 oauth2（opaque token，校验方式见 2.4）
Provider = "oidc"
```

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| Enable | bool | false | RS 校验总开关。关闭时整条分支跳过，行为与现状完全一致 |
| Audience | string | "" | 本服务资源标识。**空值时 RS 不生效**。注意：仅 `oidc` 与 `oauth2`+`introspect` 真正校验 `aud`；`oauth2`+`userinfo`（oauth2 默认）**不校验 aud**（见 2.4） |
| Provider | string | "oidc" | 受信 IdP 协议。`oidc`=复用 OIDC 登录、本地 JWKS 验签 JWT；`oauth2`=复用 OAuth2 登录、校验 opaque token（见 2.4） |

配置结构定义见 `pkg/httpx/httpx.go` 的 `RSAuth`。改 `config.toml` 后需**重启** center 生效。

### 2.2 OIDC 登录配置（`Provider=oidc`）：指定受信 IdP

RS 复用 OIDC 配置里的 IdP，所以必须先把 OIDC 配好且 **`Enable = true`**（否则 RS 拿不到 provider/JWKS，不生效）。
OIDC 配置存在数据库 `sso_config` 表，通过 **Web UI（系统设置 → 单点登录 → OIDC）** 或接口 `PUT /api/n9e/sso-config` 维护，**不在 config.toml 里**。

与 RS 相关的关键字段：

```toml
Enable = true
# IdP 的 issuer 根地址；n9e 据此拉 <SsoAddr>/.well-known/openid-configuration 与 JWKS
SsoAddr = 'https://idp.example.com/realms/yourrealm'
ClientId = '<oidc-client-id>'
ClientSecret = '<oidc-client-secret>'
DefaultRoles = ['Standard']      # JIT 建用户时赋的默认角色
DefaultTeams = [2]               # JIT 建用户时加入的默认团队 id（可空）

[Attributes]
# RS 从 access token 取哪个 claim 当用户名；Keycloak 常用 preferred_username
Username = 'preferred_username'
Nickname = 'name'
Email = 'email'
```

> 说明：RS 校验 audience 用的是 `[HTTP.RSAuth].Audience`，**不是** OIDC 的 `ClientId`——两者通常不同。
> OIDC 的 `ClientId/Secret` 仅用于交互式登录流程；RS 只需要 `SsoAddr`（拿 issuer/JWKS）、`Attributes.Username`、`DefaultRoles`、`DefaultTeams`。

### 2.3 IdP 侧：让 access token 带上 `aud`

多数 IdP 默认不会把你的资源标识写进 access token 的 `aud`，需要显式配置一个 audience：

- **Keycloak**：给 client 加一个 *Audience* 协议映射器（Client scopes → 你的 scope → Mappers → Add → Audience），
  `Included Custom Audience` 填 `n9e-a2a-rs`，勾选 *Add to access token*。
- **Auth0**：请求 token 时带 `audience=n9e-a2a-rs`（在 API 中注册该 Identifier）。
- **Entra ID**：*Expose an API* 配置 Application ID URI / scope，使 access token 的 `aud` 为该值，`Audience` 填成对应值。

另外确保：

- （`Provider=oidc` 时）IdP 签发的须是 **JWT access token**；若 IdP 只发 opaque token，请改用 `Provider=oauth2`（见 2.4）。
- n9e 进程能访问 IdP 的 discovery 与 JWKS 地址。**若环境有 HTTP 代理**，需把 IdP 地址加进 `NO_PROXY`/`no_proxy`，否则拉 JWKS/discovery 会失败。

### 2.4 OAuth2 登录配置（`Provider=oauth2`）：校验 opaque token

IdP 只签发 opaque（非 JWT）access token 时用此 provider。RS 复用 **OAuth2 登录**配置（系统设置 → 单点登录 → OAuth2，存 `sso_config` 表），所以须先把 OAuth2 配好且 **`Enable = true`**。与 RS 相关的字段：

```toml
Enable = true
SsoAddr = 'https://sso.example.com/oauth2/authorize'   # 作为 RFC 9728 的 authorization_servers 广告出去
UserInfoAddr = 'https://api.example.com/api/v1/user/info'
ClientId = '<client-id>'
ClientSecret = '<client-secret>'                       # introspect 模式向内省端点做 Basic Auth 用
DefaultRoles = ['Standard']                            # JIT 建用户的默认角色（OAuth2 无默认团队）
# 校验方式：留空（默认）/userinfo，或 introspect
RSVerifyMethod = ''
IntrospectAddr = ''                                    # RSVerifyMethod=introspect 时必填（RFC 7662 内省端点）
IntrospectCacheSeconds = 60                            # 正向结果按 token 哈希缓存秒数（introspect 再以 token exp 封顶），0 不缓存

[Attributes]
Username = 'sub'
```

`RSVerifyMethod` 两种校验：

| 取值 | 校验方式 | 是否校验 aud | 适用 |
|---|---|---|---|
| `''`（默认）/ `userinfo` | 拿 token 调 `UserInfoAddr`，**成功即视为 token 有效** | **否** —— UserInfo 响应不含 `aud`，同一 IdP 下任意有效 token 都被接受 | 对接最省事（多数 OAuth2 server 都有 UserInfo）；安全要求不高时用 |
| `introspect` | RFC 7662 内省（`IntrospectAddr`，带 ClientId/Secret Basic Auth），校验 `active` 与 `aud` | **是** —— `aud` 须含 `Audience`，否则 401 | 有安全要求时用 |

> ⚠️ **安全提示**：`userinfo` 是 oauth2 的**默认**模式，它**不校验 audience**——即使你配了 `[HTTP.RSAuth].Audience`，该值在此模式下仅用于 RFC 9728 元数据广告，**不参与放行判定**。若同一 IdP 还给别的应用发 token，这些 token 也会被 n9e 接受。**需要 audience 绑定时务必把 `RSVerifyMethod` 切到 `introspect`。** 启动日志会对 userinfo 模式打印对应 warning。

## 三、对接步骤（以 Keycloak 为例）

1. **Keycloak**：建/选一个 realm 与 client；加 Audience 映射器，使 access token 的 `aud` 含 `n9e-a2a-rs`；确认用户名落在 `preferred_username`。
2. **n9e config.toml**：设 `[HTTP.RSAuth] Enable = true`、`Audience = "n9e-a2a-rs"`，重启 center。
3. **n9e OIDC**：Web UI 系统设置 → 单点登录 → OIDC：`Enable=true`，`SsoAddr` 指向 Keycloak realm，填 `ClientId/Secret`，`Attributes.Username = preferred_username`，按需设 `DefaultRoles` / `DefaultTeams`。保存（约 9s 内热加载生效）。
4. **取 token 自测**：从 Keycloak 取一个用户的 access token，调 a2a/mcp（见下）。
5. **核验**：调用返回非 401（MCP 返回工具列表 / A2A 进入协议处理器）；n9e 日志出现 `[A2A] done ... user=<该用户>` / `[MCP] done ... user=<该用户>`；若该用户原本不存在，用户管理页会新出现该用户（默认角色/团队）。

## 四、自测（curl）

```bash
# 1) 从 IdP 取该用户的 access token（Keycloak 密码模式示例）
TOKEN=$(curl -s --noproxy '*' \
  -d 'grant_type=password' -d 'client_id=<client>' -d 'client_secret=<secret>' \
  -d 'username=carol' -d 'password=<pwd>' -d 'scope=openid' \
  'https://idp.example.com/realms/yourrealm/protocol/openid-connect/token' | jq -r .access_token)

# 2) 调 MCP（合法 token → 200，返回 result.tools）
curl -s --noproxy '*' -X POST http://127.0.0.1:17000/mcp \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# 3) 反例：aud 不符 / 过期 / 改坏签名 / iss 不符 → 一律 401 unauthorized
```

## 五、行为与边界

- **开关启停**：`Enable=false` 时 RS 分支整体跳过，OAuth token 不再被接受，其余鉴权与现状完全一致。
- **并列不互斥**：开启后 `X-User-Token`、自签 JWT 行为不变；三档满足任一即放行。
- **作用范围**：RS 校验挂在共享的 `tokenAuth()` 中间件上，所以除 `/a2a` `/mcp` 外，其它走 `tokenAuth` 的接口（`/api/n9e/*`）同样接受该 token——这是预期行为（凭证即用户身份）。
- **发现链路（见第六节）**：RS 启用时会发布 `/.well-known/oauth-protected-resource` 并在 AgentCard 增加 oidc 档;但 401 暂不带 `WWW-Authenticate` 头（MCP 的自动发现入口仍待补）。

## 六、发现链路（OAuth 自动发现）

为减少调用方手工配置，RS 启用时（`rsAuthEnabled`）会主动暴露两处「发现」信息，让支持 OAuth 的客户端自动找到受信 IdP：

- **AgentCard 增加 `oidc` 档**（A2A 客户端用，**仅 `Provider=oidc` 时**）：`GET /.well-known/agent-card.json` 的 `securitySchemes` 在原有 `x-user-token` 之外增加一档 `oidc`（`type=openIdConnect`，`openIdConnectUrl` 指向 IdP 的 `…/.well-known/openid-configuration`），并加入 `security` 数组——两档**满足任一**即可，A2A 客户端据此自动选择走 OAuth。`Provider=oauth2` 时纯 OAuth2 IdP 无 OIDC discovery 文档，AgentCard **不**增加该档（只保留 `x-user-token`）。
- **RFC 9728 资源元数据端点**（OAuth/MCP 客户端用）：`GET /.well-known/oauth-protected-resource`（公开、无需鉴权）返回：

  ```json
  {
    "resource": "n9e-a2a-rs",
    "authorization_servers": ["https://idp.example.com/realms/yourrealm"],
    "bearer_methods_supported": ["header"]
  }
  ```

  `resource` = `[HTTP.RSAuth].Audience`（建议配成 https URL 以完全契合 RFC 9728），`authorization_servers` = 受信 provider 的 `SsoAddr`（`oidc` 取 OIDC、`oauth2` 取 OAuth2）。RS 未启用时该端点返回 404，不广告任何东西。

**尚未做的一环（②）**：401 响应**暂不**带 `WWW-Authenticate: Bearer resource_metadata="…"` 头。影响：**A2A 客户端不受影响**（靠 AgentCard 自发现）;**MCP 客户端**（ChatGPT/Claude connector）的标准自动发现入口正是这个 401 头，缺它则需**手动配置** IdP 与 audience（功能可用，只是非零配置）。日后补 ② 时，上面两处元数据已就绪、直接指向即可。

> 说明：AgentCard 的 `oidc` 档与资源元数据端点一样**每次请求实时计算**——运行时启用 RS/OIDC 或更换 IdP 后，下一次拉取 AgentCard 即生效，**无需重启 center**。

## 七、排错

| 现象 | 可能原因 |
|---|---|
| OAuth token 一律 401 | `RSAuth.Enable=false` / `Audience` 空 / 对应 provider 未启用（OIDC 或 OAuth2 `Enable=false`）/ `oauth2`+`introspect` 缺 `IntrospectAddr` / `oauth2`+`userinfo` 缺 `UserInfoAddr`；启动日志会打印对应 warning |
| 合法 token 仍 401 | （oidc）`aud` 不含 `Audience`、`iss` 与 `SsoAddr` issuer 不一致、token 过期、拉不到 JWKS；（oauth2 introspect）`active=false`、`aud` 不含 `Audience`、内省端点不通或 Basic Auth 失败；（oauth2 userinfo）UserInfo 返回非 200 |
| 建了用户但用户名不对 | 对应登录配置 `Attributes.Username` 取错 claim（如该用 `preferred_username` 却配了 `sub`） |
| 未自动建用户/没进团队 | `DefaultRoles` / `DefaultTeams` 未配（OAuth2 无默认团队） |

开启 debug 日志可看到校验失败原因：`[RS] verify access token failed: <err>`。

## 八、涉及代码

- `pkg/httpx/httpx.go` — `RSAuth` 配置（`Enable` / `Audience` / `Provider`）
- `pkg/oidcx/oidc.go` — `VerifyAccessToken`（`oidc` provider：复用 provider 的 JWKS 验签 + issuer/audience/过期，映射 claim）
- `pkg/oauth2x/oauth2x.go` — `VerifyAccessToken`（`oauth2` provider：introspect/userinfo 两种校验 + 按 token 哈希缓存）
- `center/router/router_rsauth.go` — `rsAuthProvider` / `rsAuthEnabled` / `shouldVerifyAsRS`（按 provider 区分 token）/ `authByIdPAccessToken`（JIT 建用户）/ `oidcDiscoveryURL` / `rsAuthServerAddr` / `oauthProtectedResource`（RFC 9728 元数据）
- `center/router/router_mw.go` — `tokenAuth()` 中的 RS 分支
- `center/router/router_a2a.go` — 注册 `/.well-known/oauth-protected-resource`，并把 OIDC 发现 URL 传入 AgentCard
- `aiagent/a2a/agent_card.go` — AgentCard 的 `oidc` securityScheme 档
