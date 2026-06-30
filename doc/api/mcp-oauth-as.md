# A2A / MCP 内建 OAuth 2.1 授权服务器（Authorization Server）

让 n9e **自身成为 OAuth 2.1 授权服务器**（与 Resource Server co-located），使通用 MCP 客户端
（Claude / ChatGPT connector）经 RFC 7591 动态客户端注册(DCR) **零配置**接入 `/a2a` `/mcp`。
这是 [a2a-oauth-rs.md](./a2a-oauth-rs.md)（对接外部企业 IdP 的 Resource Server）的**互补**能力，
面向「**没有外部 IdP** 的自托管」场景。默认关闭，与 `X-User-Token`（PAT）、自签 session JWT、
外部 IdP RS 三条鉴权路径**并列**，开启后互不影响。

## 一、它解决什么、与 RSAuth 的关系

- **RSAuth（已有）**：n9e 当 Resource Server，接受**外部企业 IdP** 签发的 token —— 适合「已有 Keycloak/Entra」。
- **MCPAuth（本档）**：n9e 自己当 Authorization Server，自己签发 token、自己做 DCR —— 适合「没有 IdP，想让 Claude/ChatGPT 直接连」。
- 两者**可同时开启**：`/.well-known/oauth-protected-resource` 的 `authorization_servers` 会**同时列出** n9e 自身与外部 IdP，客户端各取所需（通用客户端走 n9e 的 DCR，企业客户端走 IdP）。

## 二、设计要点（精简版 v2）

- **无状态优先**：client_id、授权请求票据、授权码、access、refresh **全是 HS256 签名 JWT**，靠 `token_use` claim 区分；签名密钥从 `JWTAuth.SigningKey` 经 **HKDF-SHA256 派生**（与 session 密钥密码学隔离 —— MCP token 不能当 session 用，反之亦然）。
- **唯一共享状态**：授权码一次性 —— 兑换时对码的 `jti` 做一次 Redis `SetNX`，重放即拒。**原子、跨实例安全**（n9e center 多实例共享同一 Redis）。
- **refresh 不轮换**：本版 refresh 无状态、不做重用检测；撤销靠短 TTL 或轮换签名密钥。如需公网多租户级安全，后续可加 refresh 轮换。
- **consent 在前端**：n9e 会话是 header/Bearer 无 Cookie，`/oauth/authorize` 校验后 **302 跳转前端 SPA 的 `/oauth-consent` 路由**；SPA（持 token、含 SSO 登录）展示同意页，再调受保护的决策 API 签发授权码。

## 三、协议合规

实现 OAuth 2.1 + RFC 8414(AS 元数据) + RFC 9728(受保护资源元数据) + RFC 7591(DCR) + RFC 7636(PKCE，**强制 S256**) + RFC 8707(resource 绑定 aud) + RFC 7009(revoke)。授权码强制一次性；redirect_uri 精确匹配防开放重定向；access token 的 `aud` 绑定资源、校验时拒绝错配（防 passthrough）。

## 四、配置 `etc/config.toml`

```toml
[HTTP.MCPAuth]
Enable = true
# 本 AS 的 canonical URL（签发 token 的 iss、RFC 8414 元数据的 issuer）。
# 多实例务必显式配置，使各实例广告一致；留空则按请求 Host + X-Forwarded-Proto 推导（仅单机）
Issuer = "https://n9e.example.com"
# MCP 资源标识，绑定进 access token 的 aud（RFC 8707）。留空回退 RSAuth.Audience，
# 再回退 "<base>/mcp"。与 RSAuth 同开时建议设成与 RSAuth.Audience 相同
Resource = "https://n9e.example.com/mcp"
# 留空则从 JWTAuth.SigningKey 经 HKDF 派生（推荐）。多实例必须各实例一致，切勿每进程随机
# SigningKey = ""
# 生命周期（秒），留 0 用默认：access 3600 / refresh 604800 / code 60
# AccessTTL = 3600
# RefreshTTL = 604800
# CodeTTL = 60
```

| 字段 | 默认 | 说明 |
|---|---|---|
| Enable | false | 内建 AS 总开关。关闭时所有 `/oauth/*` 端点 404，行为同现状 |
| Issuer | "" | AS 的 canonical URL。**多实例必填**，否则各实例按请求派生可能发散 |
| Resource | "" | access token 的 aud；空→RSAuth.Audience→`<base>/mcp` |
| SigningKey | "" | 空则 HKDF 自 `JWTAuth.SigningKey`（与 session 隔离、全实例一致） |
| AccessTTL/RefreshTTL/CodeTTL | 3600/604800/60 | 秒 |

> ⚠️ **多实例约束**：① `SigningKey`（或其派生源 `JWTAuth.SigningKey`）必须**全实例逐字节一致**，切勿每进程随机生成，否则 token 只在签发实例上验得过。② 多实例请**显式配置 `Issuer`/`Resource`**，不要依赖请求派生。③ 除授权码一次性（共享 Redis）外全部无状态，**禁止用进程内缓存**存码/票据。

## 五、端点

| 方法 | 路径 | 鉴权 | 说明 |
|---|---|---|---|
| GET | `/.well-known/oauth-authorization-server`(+`/a2a` `/mcp` 别名) | 公开 | RFC 8414 AS 元数据 |
| GET | `/.well-known/oauth-protected-resource`(+别名) | 公开 | RFC 9728，`authorization_servers` 含 n9e 自身 |
| POST | `/oauth/register` | 公开 | RFC 7591 DCR，返回签名 client_id |
| GET | `/oauth/authorize` | 公开 | 校验参数 → 302 跳前端 `/oauth-consent?req=<票据>` |
| POST | `/api/n9e/mcp/oauth/authorize` | **session** | 决策 API：前端同意后 POST `{req, decision}` → 签发授权码 → 返回 `{redirect}` |
| POST | `/oauth/token` | public client + PKCE | `authorization_code`（一次性+PKCE+resource 校验）/ `refresh_token` |
| POST | `/oauth/revoke` | public client | RFC 7009，无状态 token 故 best-effort 200 |

## 六、授权流程

1. MCP 客户端无 token 调 `/mcp` → `401` + `WWW-Authenticate: Bearer resource_metadata=…`。
2. 客户端拉 `/.well-known/oauth-protected-resource` → 找到 AS=n9e → 拉 `/.well-known/oauth-authorization-server` → `POST /oauth/register`（DCR）→ 跳 `/oauth/authorize?...&code_challenge=...&resource=<base>/mcp`。
3. n9e 校验 client_id(验签)/redirect_uri(精确匹配)/`response_type=code`/PKCE S256/resource → 签授权请求票据 → **302 到 `/oauth-consent?req=<票据>`**（前端 SPA 路由）。
4. 前端 SPA：未登录→跳 `/login?redirect=...`（含 SSO）；已登录→展示同意页 → 点「允许」→ `POST /api/n9e/mcp/oauth/authorize {req, decision:"allow"}`（带 session）→ 后端验 session+票据 → 签**授权码**（绑定该用户）→ 返回 `{redirect}` → SPA `window.location` 跳回 `redirect_uri?code=...&state=...`。
5. 客户端 `POST /oauth/token`（code + code_verifier + resource）→ PKCE 校验 + **一次性 SetNX** + resource 一致 → 签发 access(+refresh)。
6. 客户端带 `Authorization: Bearer <access>` 调 `/mcp` → n9e 用 MCP 密钥验签放行（映射到对应用户，agent 面内权限同该用户）。注意：该 access token **仅在 `/a2a` `/mcp` 端点受理**，不能用于其它 `/api/n9e/*` 接口（见 router_mw.go 的 `agentOAuthScope`）。

## 七、自测（curl）

```bash
BASE=http://127.0.0.1:17000

# 1) DCR 拿 client_id
CID=$(curl -s -XPOST $BASE/oauth/register -H 'Content-Type: application/json' \
  -d '{"client_name":"cli","redirect_uris":["http://127.0.0.1:9999/cb"]}' | jq -r .client_id)

# 2) 构造 PKCE
VERIFIER=$(openssl rand -hex 32)
CHALLENGE=$(printf %s "$VERIFIER" | openssl dgst -sha256 -binary | openssl base64 | tr '+/' '-_' | tr -d '=')

# 3) 浏览器打开 authorize（会跳到前端 /oauth-consent 登录+同意，回调拿 code）
echo "$BASE/oauth/authorize?response_type=code&client_id=$CID&redirect_uri=http://127.0.0.1:9999/cb&code_challenge=$CHALLENGE&code_challenge_method=S256&state=xyz"

# 4) 用 code 换 token（CODE 从回调 URL 取）
curl -s -XPOST $BASE/oauth/token -d grant_type=authorization_code -d code=$CODE \
  -d code_verifier=$VERIFIER -d redirect_uri=http://127.0.0.1:9999/cb -d client_id=$CID | jq

# 5) 带 access_token 调 MCP
curl -s -XPOST $BASE/mcp -H "Authorization: Bearer $ACCESS" -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# 反例：重放第 4 步同一个 code → {"error":"invalid_grant", ...}（一次性守卫）
```

## 八、涉及代码

- `pkg/httpx/httpx.go` — `MCPAuth` 配置
- `center/router/router_mcp_oauth.go` — AS 全部端点 + 决策 API + JWT/PKCE/HKDF/一次性码助手
- `center/router/router_rsauth.go` — 发现链开关推广到 `rsAuthEnabled() || mcpAuthEnabled()`；`oauthProtectedResource` 的 `authorization_servers` 含 n9e 自身
- `center/router/router_mw.go` — `tokenAuth()` 的 builtin 分支（MCP 密钥验签，排在外部 IdP RS 之前）；`agentOAuthScope`（把 OAuth 受理限定在 `/a2a` `/mcp`）
- `center/router/router_a2a.go` / `router.go` — 端点注册（公开 `/oauth/*` 于根，决策 API 于 `/api/n9e`）
- 前端 `n9e/fe`：`src/pages/oauthConsent` + `src/routers/index.tsx` 路由 `/oauth-consent`
