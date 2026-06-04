# 💡 Nightingale Docker 部署使用指南 (example)

本目录提供了将夜莺监控系统 (Nightingale) 打包为 Docker 镜像并运行的完整示例，支持一键构建与部署。

---

## 📂 目录结构

* `Dockerfile`: 多阶段构建定义文件，自动打包前端静态文件并编译 Go 后端服务，最终构建出最小化运行时镜像。
* `start.ps1`: Windows PowerShell 一键构建启动脚本。
* `start.sh`: Linux / macOS / Git Bash 一键构建启动脚本。
* `README.md`: 本指南。

---

## 🛠️ 构建与启动

### 方式一：使用一键脚本启动 (推荐)

针对不同的系统环境，我们提供了一键打包并启动的脚本，脚本会自动检测上级目录中的 `etc/config.toml` 配置并将其挂载到容器中运行。

#### 💻 Windows (PowerShell)
在 PowerShell 终端中进入此目录，直接运行：
```powershell
.\start.ps1
```

#### 🐧 Linux / macOS / Git Bash
在终端中进入此目录，赋予脚本执行权限并运行：
```bash
chmod +x start.sh
./start.sh
```

---

### 方式二：手动执行原生 Docker 命令

如果您倾向于手动执行命令，请根据以下步骤进行构建和运行。

> [!WARNING]
> **注意构建上下文 (Build Context)**
> 由于 Dockerfile 中需要复制主项目的 `go.mod`、`go.sum` 等文件，执行 `docker build` 时必须以 **Nightingale 项目根目录** 作为构建上下文。

#### 1. 构建 Docker 镜像
在 **Nightingale 项目根目录** (即本目录的上一级) 下执行：
```bash
docker build -t nightingale:latest -f example/Dockerfile .
```
*(如需通过自定义 Go 代理构建，可加上 `--build-arg GOPROXY=https://goproxy.cn,direct` 参数)*

#### 2. 启动容器
- **基础启动 (使用容器内自带的默认配置)：**
  ```bash
  docker run -d --name nightingale -p 17000:17000 nightingale:latest
  ```

- **挂载本地配置文件启动 (推荐，便于修改配置)：**
  将本地 `etc/config.toml` 挂载至容器内部，使其生效。请将下述 `$(pwd)` 替换为您在当前终端所在的夜莺根目录绝对路径：
  
  **Linux / macOS (Bash)：**
  ```bash
  docker run -d --name nightingale \
    -p 17000:17000 \
    -v $(pwd)/etc/config.toml:/app/etc/config.toml \
    nightingale:latest
  ```

  **Windows (PowerShell)：**
  ```powershell
  docker run -d --name nightingale `
    -p 17000:17000 `
    -v "$((Get-Item .).FullName)/etc/config.toml:/app/etc/config.toml" `
    nightingale:latest
  ```

---

## 🔍 服务验证与排错

### 1. 检查容器状态
```bash
docker ps -f name=nightingale
```

### 2. 实时查看日志
若夜莺未正常启动，可以通过以下命令查看容器内后端日志：
```bash
docker logs -f nightingale
```

### 3. 访问 Web 页面
容器启动成功后，您可以在浏览器中访问以下地址以使用夜莺监控系统：
* **URL:** `http://localhost:17000`
