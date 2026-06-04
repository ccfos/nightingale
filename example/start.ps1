# 获取当前脚本所在目录
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

# 夜莺项目的根目录是此脚本所在目录的父目录
$N9eRootDir = (Get-Item $ScriptDir).Parent.FullName
$Dockerfile = Join-Path $ScriptDir "Dockerfile"
$LocalConfig = Join-Path $N9eRootDir "etc" "config.toml"

Write-Host ">>> Building Nightingale Docker Image..." -ForegroundColor Cyan
Write-Host "Context: $N9eRootDir" -ForegroundColor Gray
Write-Host "Dockerfile: $Dockerfile" -ForegroundColor Gray

# 执行 Docker 镜像构建，以 nightingale 项目根目录作为上下文
docker build -t nightingale:latest -f $Dockerfile $N9eRootDir

if ($LASTEXITCODE -ne 0) {
    Write-Error "Docker Build Failed!"
    Exit $LASTEXITCODE
}

Write-Host "`n>>> Starting Nightingale Container..." -ForegroundColor Cyan

# 检查本地的 etc/config.toml 是否存在，存在则进行挂载，不存在则使用容器内自带的配置
if (-not (Test-Path $LocalConfig)) {
    Write-Warning "Local config.toml not found at $LocalConfig. Container will use the default packaged config."
    # 不挂载配置启动
    docker run -d --name nightingale -p 17000:17000 nightingale:latest
} else {
    $AbsoluteConfigPath = (Get-Item $LocalConfig).FullName
    Write-Host "Mapping config: $AbsoluteConfigPath -> /app/etc/config.toml" -ForegroundColor Gray
    # 挂载本地配置启动
    docker run -d --name nightingale -p 17000:17000 -v "${AbsoluteConfigPath}:/app/etc/config.toml" nightingale:latest
}

if ($LASTEXITCODE -eq 0) {
    Write-Host "`n>>> Nightingale started successfully!" -ForegroundColor Green
    Write-Host "You can access it at http://localhost:17000" -ForegroundColor Green
    Write-Host "To check logs, run: docker logs -f nightingale" -ForegroundColor Gray
} else {
    Write-Error "Failed to start Docker container!"
}
