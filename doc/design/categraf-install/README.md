# 发布包内置 Categraf 与机器列表一键安装 设计方案

解决新用户接入监控的首个断点：装第一台 categraf 必须手动下载安装包、手改上报地址、且装完后页面无实时反馈。

对应问题（来自走查）：

- **C1**：categraf 安装文档硬编码 `127.0.0.1:17000`，装第一台机器必手改（`fe: public/n9e-docs/categraf/zh_CN.md:17,22`，DocumentDrawer 无变量替换能力）。
- **C2**：无「等待首台机器上报」的实时确认，只能手动刷新（机器列表页无任何轮询）。
- 官方 categraf 无一键安装脚本，只有「手动下载 → 改 conf/config.toml → `sudo ./categraf --install --start`」流程；内网/离线环境连下载这一步都走不通。

## 1. 目标

1. n9e 发布 tar.gz（及 Docker 镜像）内置 categraf 安装包（linux amd64 + arm64 双架构），离线/内网环境可用。
2. 机器列表页提供一条可复制的一键安装命令，目标机执行后自动完成：下载 → 解压 → 改上报地址 → 注册 systemd 服务 → 启动。
3. 上报地址零手改：脚本由服务端动态渲染，默认地址取「目标机实际用来下载脚本的地址」。
4. 前端实时确认首台机器上报，闭环 C2。

非目标（二期展望见 §8）：Windows 一键安装、安装时自动挂载业务组、categraf 独立于 n9e 发版的版本管理。

## 2. 现状关键事实

### 2.1 后端（ccfos/nightingale）

| 事实 | 位置 |
|------|------|
| 发布 tar.gz 内容 = 3 个二进制 + `docker/ etc/ integrations/ cli/ n9e.sql`，约 167MB | `.goreleaser.yaml:81-86` |
| 前端 pub 经 statik embed 进 `n9e` 二进制，不在 tar 文件列表 | `.goreleaser.yaml:22-24`、`fe.sh`、`center/router/router.go:252` |
| 已有「从磁盘目录匿名发文件」先例：integrations icon | `center/router/router.go:437`、`center/router/router_builtin.go:308-317` |
| 已有免鉴权端点先例：`/pub`、`/api/n9e/versions`、`/api/n9e/site-info` | `center/router/router.go:258,625,751` |
| categraf 心跳入口 `POST /v1/n9e/heartbeat`，默认匿名（BasicAuth 注释掉） | `center/router/router.go:898-905`、`etc/config.toml:47-50` |
| ident 由心跳 INSERT 进 `target` 表，前端可轮询 `/api/n9e/targets` 确认 | `pushgw/idents/idents.go:137-150`、`center/router/router.go:422` |
| `site_url` 存 DB configs 表，默认初始化为 `http://<心跳IP>:17000`，`/api/n9e/site-info` 匿名可查 | `center/center.go:173-234`、`center/router/router_config.go:66-69` |
| 无任何 categraf 安装端点/脚本/版本 pin | 全局 grep 无命中 |

### 2.2 前端（n9e/fe）

| 事实 | 位置 |
|------|------|
| 机器列表空态引导已存在，主按钮仅打开静态文档抽屉 | `src/pages/hosts/pages/List/List.tsx:397-413` |
| DocumentDrawer 纯 fetch md 渲染，无变量替换 | `src/components/DocumentDrawer/index.tsx:42-64` |
| 机器列表无自动轮询，空态需手动刷新 | `src/pages/hosts/` 全目录无 interval |
| 现成「命令 + 复制按钮」组件 `Code`、`copy2ClipBoard` 工具 | `src/components/Code/index.tsx`、`src/utils/index.ts:77-103` |
| `siteInfo` 已在应用初始化时拉取入 Context，但 `site_url` 前端从未消费 | `src/App.tsx:247-254` |
| `en_US.md` 文档内容错位为企业版（http_provider / categraf_ent） | `public/n9e-docs/categraf/en_US.md:6-18` |

### 2.3 categraf（外部约束）

- 单架构 tar.gz 约 49–55MB（slim 与完整版体积几乎相同，选完整版）。
- 目标机需改 `conf/config.toml` 两处：`[[writers]] url`（remote write）与 `[heartbeat] url`（心跳，机器出现在列表页靠它，默认 `enable = true`）。
- systemd 安装：`sudo ./categraf --install`，启动 `--start`；普通用户模式 `--user --install`（v0.4.5+）。

## 3. 方案选型

### 3.1 categraf 包放在哪里

| 方案 | 做法 | 结论 |
|------|------|------|
| **A. 发布包内置目录（选定）** | tar.gz 增加 `agents/categraf/*.tar.gz`，后端从磁盘目录发文件 | 离线可用；goreleaser `files` 加一行 + 一个下载端点；不膨胀二进制与内存 |
| B. go:embed 进 n9e 二进制 | 仿 sandbox 资产链路（`.github/workflows/n9e.yml:17-53`） | 否。二进制 +100MB 且常驻内存，无收益 |
| C. 不内置，脚本从公网下载 | install.sh 里 wget GitHub releases | 否（作为 A 的降级保留）。离线/内网不可用，违背需求本意 |

**选定 A + C 降级**：install.sh 先从 n9e 本机下载，失败（老版本包、agents 目录被删）时回落 GitHub releases。

公网回退源只用 GitHub，不用 flashcat CDN：pin 的版本正是 `scripts/download_categraf.sh` 从 GitHub 取的，GitHub 必定有该版本；而 CDN 会滞后（v0.5.15 已发布时其下载页仍停在 v0.5.9），回退到它反而会 404。

### 3.2 架构矩阵（已确认）

**双架构全量内置**：每个发布包同时内置 linux-amd64 + linux-arm64 两个 categraf 包（被监控机器架构 ≠ n9e 服务器架构）。代价：发布包 ~167MB → **~270MB**，已确认接受。Windows 不进一期，文档链接兜底。

### 3.3 上报地址如何免手改（核心设计）

一键命令形态：

```bash
curl -fsSL http://<n9e地址>:17000/api/n9e/agents/categraf/install.sh | sudo bash
```

install.sh 由服务端 **动态渲染**，默认上报地址取本次 HTTP 请求的 Host（处理 `X-Forwarded-Proto` / `X-Forwarded-Host`），兜底 `site_url`。

逻辑闭环：**目标机既然能用这个地址 curl 到脚本，这个地址就必然可作为上报地址**——比读 site_url 更可靠（site_url 是浏览器视角，不保证目标机可达）。脚本内以 `N9E_HOST` 环境变量暴露，允许覆盖：

```bash
curl -fsSL http://10.1.2.3:17000/api/n9e/agents/categraf/install.sh | sudo N9E_HOST=http://other:17000 bash
```

## 4. 后端设计

### 4.1 打包链路

1. 新增 `scripts/download_categraf.sh`：
   - 版本 pin 在脚本内变量 `CATEGRAF_VERSION`（如 `v0.5.15`），升级 = 改一行，随 n9e 发版节奏走。
   - 从 GitHub release 下载 `categraf-${VER}-linux-amd64.tar.gz` 与 `categraf-${VER}-linux-arm64.tar.gz` 到 `agents/categraf/`，用 `checksums.txt` 校验。
   - 已存在且校验通过则跳过（本地重复构建友好）。
2. `.goreleaser.yaml`：
   - `before.hooks` 增加执行该脚本；
   - `archives.files` 追加 `agents/*`。
3. `docker/Dockerfile.goreleaser`：增加 `ADD agents`。**不可漏**——docker/compose 部署用户占比高，漏掉则镜像用户拿不到此功能。
4. `agents/` 加入 `.gitignore`（构建期产物，同 `pub/` 处理方式）。

### 4.2 HTTP API（新增 `center/router/router_agent.go`）

三个端点全部免鉴权，挂 `pages` 组（先例：`site-info`、`integrations/icon`）：

| 端点 | 行为 |
|------|------|
| `GET /api/n9e/agents/categraf/install.sh` | `text/template` 渲染安装脚本（模板 go:embed，几 KB）。注入：默认服务地址（§3.3）、categraf 版本、可用架构列表。`Content-Type: text/x-shellscript` |
| `GET /api/n9e/agents/categraf/download?arch=amd64\|arm64` | 从 `AgentsDir` 发对应 tar.gz。**arch 参数白名单校验**（只接受 `amd64`/`arm64`，杜绝路径穿越——现有 `builtinIcon` 的裸拼路径不照抄）。文件不存在返回 404（触发脚本降级到 GitHub） |
| `GET /api/n9e/agents/categraf/meta` | 返回 `{"bundled": bool, "version": "v0.5.15", "arches": ["amd64","arm64"], "basic_auth": bool}`。供前端决定展示一键安装还是退回文档模式；`basic_auth` 反映 `APIForAgent.BasicAuth` 是否启用（只返回布尔，不泄露凭据） |

新增配置项：`[Center] AgentsDir`，默认 `./agents/categraf`（同 `BuiltinIntegrationsDir` 的默认值模式，`center/cconf/conf.go:14`）。

### 4.3 install.sh 行为

```
1. 前置检查：root 或 sudo；curl/wget 二选一存在
2. uname -m 探测架构（x86_64→amd64，aarch64→arm64；其余报错退出并给出文档链接）
3. 下载 ${N9E_HOST}/api/n9e/agents/categraf/download?arch=${ARCH}
   ├─ 失败 → 降级 GitHub releases（无外网则报错退出）
4. 解压到 /opt/categraf（已存在则中止并提示 --force，避免静默覆盖用户已改配置；
   --force 时先 ./categraf --stop 再覆盖，保留已有 conf 的自定义文件）
5. sed 改写 conf/config.toml：
   ├─ [[writers]] url = "${N9E_HOST}/prometheus/v1/write"
   └─ [heartbeat] url = "${N9E_HOST}/v1/n9e/heartbeat"
6. 有 systemd → ./categraf --install && ./categraf --start
   无 systemd → nohup 启动并提示
7. 输出：安装完成；机器将在约 10 秒内出现在 n9e 机器列表页；提示挂载业务组
```

支持的覆盖变量/参数：`N9E_HOST`（上报地址）、`--dir`（安装目录，默认 `/opt/categraf`）、`--force`、`--auth user:pass`（`APIForAgent.BasicAuth` 场景，写入 writers/heartbeat 的 basic auth 字段）。

## 5. 前端设计（n9e/fe，另仓库实施）

1. 新增 `InstallCategrafModal`：
   - 打开时调 meta 接口；`bundled=false` 或接口 404（老后端）→ 降级为现有文档抽屉。
   - n9e 地址输入框，默认 `siteInfo.site_url || window.location.origin`，可编辑，命令随之重新生成。
   - 用现成 `Code` 组件展示一行命令 + 复制按钮；附「先下载再审阅」备选命令。
   - `basic_auth=true` 时提示追加 `--auth user:pass`。
2. `List.tsx` 入口：
   - 空态 `EmptyGuide` 主按钮从「打开文档」改为打开此弹窗，文档链接降为次级；
   - 工具栏刷新按钮旁增加常驻「安装采集器」入口（列表非空时也能加新机器）。
3. **C2 闭环（轮询确认）**：
   - 弹窗打开期间每 5s 轮询 `GET /api/n9e/targets`，对比打开前 ident 集合，检测到新机器 → 显示「✓ 机器 xxx 已上报」，自动刷新列表并提示挂载业务组；
   - 空态 + 无筛选时，列表本身每 10s 自动轮询（用户不开弹窗直接跑命令也能自动出现）。
4. **C1 文档收尾**：
   - `DocumentDrawer` 增加可选 `variables` prop 做 `{{N9E_HOST}}` 占位替换；
   - `zh_CN.md` 更新为引用一键安装方式；
   - 顺手修复 `en_US.md` 内容错位为企业版的问题。

## 6. 兼容性与降级矩阵

| 组合 | 表现 |
|------|------|
| 新后端 + 新前端 | 完整一键安装 + 轮询确认 |
| 老后端 + 新前端 | meta 404 → 前端降级为文档抽屉，行为同现状 |
| 新后端 + 老前端 | 端点闲置，无副作用；命令可从文档手动获得 |
| agents 目录缺失（用户自删/裁剪包） | download 404 → 脚本降级 GitHub；meta `bundled=false` → 前端提示 |
| 服务端在代理/LB 后 | install.sh 渲染读 `X-Forwarded-*`；仍不对则用户改弹窗内地址输入框 |

## 7. 风险与安全

- **匿名端点面**：install.sh 与 download 均不含敏感信息（地址来自请求方自己的 Host；categraf 是公开软件），与现有 `/pub`、`site-info` 匿名策略一致。
- **路径安全**：download 端点 arch 白名单，不接受任意文件名。
- **curl|bash 观感**：业界通行（Datadog 等同型），且脚本从用户自己的 n9e 下发而非第三方；提供「先下载审阅」备选。
- **APIForAgent.BasicAuth 启用场景**：凭据不经匿名端点下发，由用户显式 `--auth` 传入；meta 只返回布尔提示。
- **发布包体积**：~167MB → ~270MB（已确认接受）。
- **categraf 版本滞后**：pin 版本随 n9e 发版更新；脚本的 GitHub 降级路径始终能拿到 pin 的那个版本。

## 8. 落地改动清单

### 后端（本仓库）

- [ ] `scripts/download_categraf.sh`：按 pin 版本下载双架构包 + checksum 校验
- [ ] `.goreleaser.yaml`：before hook + `archives.files` 追加 `agents/*`
- [ ] `docker/Dockerfile.goreleaser`：`ADD agents`
- [ ] `.gitignore`：追加 `agents/`
- [ ] `center/cconf/conf.go`：`Center.AgentsDir` 配置项（默认 `./agents/categraf`）
- [ ] `center/router/router_agent.go`：三个端点（install.sh 渲染 / download / meta）
- [ ] `center/router/templates/install-categraf.sh.tmpl`（go:embed）：§4.3 脚本模板
- [ ] `center/router/router.go`：注册路由（pages 组，免鉴权）
- [ ] `etc/config.toml`：AgentsDir 示例注释

### 前端（n9e/fe 仓库）

- [ ] `src/pages/hosts/pages/List/InstallCategrafModal.tsx`：一键安装弹窗（含轮询确认）
- [ ] `src/pages/hosts/pages/List/List.tsx`：空态/工具栏入口 + 空态自动轮询
- [ ] `src/pages/hosts/services.ts`：meta 接口封装
- [ ] `src/components/DocumentDrawer/index.tsx`：可选 `variables` 占位替换
- [ ] `public/n9e-docs/categraf/zh_CN.md`：更新为一键安装引导；`en_US.md` 修复内容错位
- [ ] `src/pages/hosts/locale/*`：新增文案

两仓库可并行开发，靠 meta 接口解耦发布顺序。

### 二期展望

- 安装时自动挂载业务组（heartbeat 协议扩展 host_tags / 服务端自动挂载规则）
- Windows 一键安装（PowerShell 脚本）
- 服务端惰性下载 categraf（包体积回落，联网环境自动补齐 agents 目录）
