---
name: categraf-deploy-guide
description: 解答"如何部署 categraf 采集器"。触发场景：用户问"怎么装 categraf / categraf 怎么部署 / 用 Docker 跑 categraf / Windows 装 categraf / categraf 怎么注册成系统服务 / categraf 上报到夜莺 / categraf config.toml 怎么写 / 怎么验证 categraf 采集到数据"。覆盖二进制+systemd、Docker、Windows、K8s 提示、关键配置、常见验证命令。本 skill 是教学/指引型，不调任何工具，输出可粘贴执行的命令与配置片段。
---

# Categraf 部署指南

## 适用范围

**进本 skill**：
- "categraf 怎么装 / 怎么部署 / 怎么启动"
- "Docker / Kubernetes / Windows / Linux 上跑 categraf"
- "categraf 怎么注册成 systemd 服务 / 开机自启"
- "categraf 上报到夜莺 / config.toml 怎么写 / writer 地址写啥"
- "怎么验证 categraf 采集到了数据"
- "categraf 装好了但夜莺看不到机器" → **不进本 skill**，转 `n9e-host-onboard-diagnose`

**不进本 skill**：
- 已装但接入失败的诊断 → `n9e-host-onboard-diagnose`
- 已接入但指标异常 → `n9e-host-health-diagnose`
- 配置某个具体 input 插件（mysql / redis / nginx / snmp 等）的采集 → 引导用户查 `conf/input.<name>/` 下的注释，本 skill 只做"装起来 + 通起来"

## 一句话原则

**categraf 部署 = 三个动作**：拿到二进制 → 写对 `config.toml` 的 `[[writers]]` → 启动并验证。其他都是这三步的变体（容器化、systemd 托管、Windows 服务等）。

## 部署方式速查

| 场景 | 推荐方式 | 说明 |
|---|---|---|
| Linux 物理机 / VM | **二进制 + systemd** | v0.3.35+ 内置 `--install` 一行注册 |
| Linux 但无 root | 二进制 + `--user --install` | v0.4.5+ 支持 |
| 容器/编排环境 | **Docker** 或 K8s DaemonSet | 采集宿主机指标需挂 `/proc /sys` |
| Windows | `categraf.exe --win-service-install` | 或 `win_run.bat` 后台运行 |
| macOS（开发/调试） | 二进制直跑 | 生产不建议 |

操作系统支持：Linux 内核 2.6.32+ / Windows 10+ 或 Server 2008+ / macOS 10.15+。

## 第一步：拿到二进制

下载地址（任选其一）：
- 官方下载中心：https://flashcat.cloud/download/categraf/
- GitHub releases：https://github.com/flashcatcloud/categraf/releases

包命名规则：`categraf-{version}-{os}-{arch}.tar.gz`，例如 `categraf-v0.3.35-linux-amd64.tar.gz`。**先确认架构**（`uname -m`：x86_64 选 amd64，aarch64 选 arm64）。

解压后目录结构：

```
categraf/
├── categraf              # 主程序
└── conf/
    ├── config.toml       # 主配置（hostname / writers / heartbeat）
    └── input.*/          # 各类采集插件的子配置目录
```

## 第二步：写对 `conf/config.toml`

最小可用配置（上报到夜莺 n9e）：

```toml
[global]
hostname = ""                  # 留空让 categraf 自动取主机名；多机重名时显式指定
interval = 15                  # 全局采集频率（秒）
providers = ["local"]

[global.labels]
# region = "shanghai"          # 全局附加标签，可选
# env = "prod"

[[writers]]
url = "http://N9E_HOST:17000/prometheus/v1/write"
basic_auth_user = ""
basic_auth_pass = ""
timeout = 5000
dial_timeout = 2500
max_idle_conns_per_host = 100

[heartbeat]
enable = true                  # 必须开，否则夜莺机器列表看不到这台机器
url = "http://N9E_HOST:17000/api/n9e/heartbeat"
interval = 10
```

**关键字段**：
- `[[writers]].url`：n9e 的 remote write 接收端点，路径固定 `/prometheus/v1/write`，端口默认 17000。
- `[heartbeat].enable=true` + `[heartbeat].url`：心跳上报，让"机器列表"能看到这台机器；**漏配 → 装了也看不到**。
- `[global].hostname`：留空走系统 hostname；如果 `hostname` 命令在容器/克隆机里取出来全是 `localhost.localdomain` 这种重名，**必须显式指定**，否则不同机器会互相覆盖。
- `[global].labels`：全局打 tag，写好区分维度（region / env / idc）后面建大盘和告警很省心。

⚠️ **不要把 `omit_hostname` 设成 true**——它会去掉 `ident` 标签，结果是机器列表 OS/agent_version 全是 unknown（社区 FAQ 高频翻车）。

## 第三步：启动与托管

### 方案 A：Linux + systemd（v0.3.35 及以上，推荐）

```bash
cd /path/to/categraf
sudo ./categraf --install      # 注册成系统服务
sudo ./categraf --start        # 启动
sudo ./categraf --status       # 查看状态
sudo ./categraf --stop         # 停止
```

非 root 用户（v0.4.5+）：

```bash
./categraf --user --install
./categraf --user --start
```

systemd 资源限制（编辑 `/etc/systemd/system/categraf.service`）：

```ini
[Service]
CPUQuota=200%
MemoryLimit=1G
```

改完 `systemctl daemon-reload && systemctl restart categraf`。

### 方案 B：Docker

最小化（只采容器内/网络指标）：

```bash
docker run -td \
  --name categraf \
  --restart=always \
  -e TZ=Asia/Shanghai \
  -v /home/flashcat/categraf/conf/:/etc/categraf/conf \
  flashcatcloud/categraf:latest
```

**采集宿主机 CPU/内存/磁盘**（最常用，必须挂宿主机的 `/proc` `/sys` 并用 host 网络）：

```bash
docker run -td \
  --name categraf \
  --restart=always \
  --network=host \
  -e TZ=Asia/Shanghai \
  -e HOST_PROC=/hostfs/proc \
  -e HOST_SYS=/hostfs/sys \
  -e HOST_MOUNT_PREFIX=/hostfs \
  -v /home/flashcat/categraf/conf/:/etc/categraf/conf \
  -v /proc:/hostfs/proc:ro \
  -v /sys:/hostfs/sys:ro \
  flashcatcloud/categraf:latest
```

资源限制：追加 `--cpus=2 --memory=1g`。

### 方案 C：Windows

注册成服务：

```cmd
categraf.exe --win-service-install
categraf.exe --win-service-start
categraf.exe --win-service-stop
categraf.exe --win-service-uninstall
```

或者用脚本后台跑：

```cmd
win_run.bat start
win_run.bat stop
```

Windows 不支持 `kill -HUP` 重载，改配置后用 `--win-service-stop` + `--win-service-start`。

### 方案 D：Kubernetes

K8s 一般用 DaemonSet（每个 Node 一份采宿主机指标）+ ConfigMap（统一下发 `config.toml`），官方 release 包里有 `k8s/` 目录给的 YAML 样例，直接 `kubectl apply -f k8s/`。**重点改两处**：
1. ConfigMap 里的 `[[writers]].url` 和 `[heartbeat].url` 指向你的 n9e；
2. 如果用了多集群，给 `[global.labels]` 加 `cluster = "xxx"` 区分。

## 第四步：验证

### 4.1 单插件调试（不会真正上报，只打印一次）

```bash
./categraf --test --inputs cpu              # 只测一个
./categraf --test --inputs cpu:mem:disk     # 多个用冒号分隔
```

看到 stdout 出现 metric 行就说明本机采集 OK；如果报错（连接 mysql 失败 / 权限不足等），先把这一步过了再启服务。

### 4.2 重载配置（不重启进程）

```bash
kill -HUP `pidof categraf`   # Linux 专用，Windows 不支持
```

### 4.3 在夜莺侧验证

- 浏览器打开 n9e → **基础设施 / 机器列表**，应能看到这台 `ident`，且 OS/CPU/agent_version 都不是 unknown。
- 没出现 → 转 `n9e-host-onboard-diagnose` 走 5 段排障，不要在这里硬猜。

### 4.4 自升级（v0.3.36+）

```bash
./categraf --update --update_url https://download.flashcat.cloud/categraf-vX.Y.Z-linux-amd64.tar.gz
```

## 常见踩坑（一句话版）

| 现象 | 原因 | 修复 |
|---|---|---|
| 机器列表看不到 | `[heartbeat].enable=false` 或 `url` 没改 | 改 toml，重启 |
| 机器列表 OS=unknown | `omit_hostname=true` 或 categraf 版本 < v0.2.35 | 关掉 omit_hostname，升级版本 |
| 多台机器互相覆盖 | hostname 重名（localhost / VM 克隆） | 显式设 `[global].hostname` 或 ident shell |
| Docker 里采不到宿主机 CPU | 没挂 `/proc /sys` 也没设 `HOST_*` env | 用上面"采宿主机"那段命令 |
| 写入 prom 报 499 / queue full | n9e 后端 ingest 队列满 | 调 n9e 写入并发，或减少 categraf interval |
| TLS x509 unknown authority | 用了自签证书没装 CA | writer 段加 `insecure_skip_verify = true`（仅测试），或装 CA |
| BasicAuth 401 | n9e 开了 BasicAuth 但 categraf 没配 | 填 `basic_auth_user/pass` |

## 输出风格

- **先问清楚再给方案**：用户没说 OS / 部署形态时，先问"你是要装在 Linux 物理机、Docker 还是 K8s 上？n9e 的访问地址是什么？"
- **每条建议都给可粘贴的命令**——不要写"修改一下配置"，要写"把 `config.toml` 里 `[[writers]].url` 改成 `http://your-n9e:17000/prometheus/v1/write`"。
- **配置示例必须带占位符替换说明**（`N9E_HOST` 之类），让用户知道哪里要改。
- 用户语言回答（中文用户中文，英文用户英文）。
- 部署完后**必须提醒做一次 `--test --inputs cpu`** 和"去夜莺机器列表看一眼"，否则当下不知道有没有装通。
- 如果用户说"装完了看不到机器" → 不要再给部署指令，直接引导到 `n9e-host-onboard-diagnose`。
