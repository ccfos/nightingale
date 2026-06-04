# Stage 1: Build the backend
FROM golang:1.25-alpine AS builder

# 【修复核心】在安装依赖前，先将 Alpine 软件源替换为国内阿里云镜像源
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk add --no-cache git gcc musl-dev

WORKDIR /workspace

# 可选：配置 Go 模块代理以加快依赖下载
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY

# 复制 Go 依赖定义文件以缓存依赖层
COPY go.mod go.sum ./
RUN go mod download

# 复制项目所有源码（包含已预嵌入静态资源的 front/statik 目录）
COPY . .

# 编译生成 center 服务主程序 (n9e)
RUN GIT_COMMIT=$(git rev-parse 'HEAD^{commit}' 2>/dev/null || echo "unknown") && \
    RELEASE_VERSION="v6-docker-$GIT_COMMIT" && \
    go build -ldflags "-w -s -X github.com/ccfos/nightingale/v6/pkg/version.Version=${RELEASE_VERSION}" -o n9e ./cmd/center/main.go

# Stage 2: Final minimal runtime image
FROM alpine:3.18
# 安装运行时所需的 ca 证书、时区数据和常用网络命令
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk add --no-cache ca-certificates tzdata bash curl && \
    ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app

# 从 builder 阶段拷贝编译出的二进制可执行文件
COPY --from=builder /workspace/n9e /app/n9e
# 拷贝默认配置及集成目录
COPY --from=builder /workspace/etc /app/etc
COPY --from=builder /workspace/integrations /app/integrations

# 声明默认服务端口
EXPOSE 17000

# 默认容器启动命令，指定配置文件路径
CMD ["/app/n9e", "-configs", "etc"]