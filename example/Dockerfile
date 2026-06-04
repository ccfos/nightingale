# Stage 1: Build the backend and embed frontend
FROM golang:1.25-alpine AS builder

# 安装构建所需的依赖工具
RUN apk add --no-cache git curl tar bash gcc musl-dev

WORKDIR /workspace

# 可选：配置 Go 模块代理以加快国内构建速度
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=$GOPROXY

# 安装静态资源打包工具 statik
RUN go install github.com/rakyll/statik@latest

# 复制 Go 依赖定义文件以缓存依赖层
COPY go.mod go.sum ./
RUN go mod download

# 复制整个仓库源码
COPY . .

# 检查本地是否有 pub 前端资源目录，如无，则自动从 GitHub 接口获取最新发布版解压
RUN if [ ! -d "./pub" ]; then \
        echo "pub directory not found, downloading latest frontend from github..." && \
        TAG=$(curl -sX GET https://api.github.com/repos/n9e/fe/releases/latest | awk '/tag_name/{print $4;exit}' FS='[""]') && \
        echo "Latest frontend version: $TAG" && \
        curl -o n9e-fe-${TAG}.tar.gz -L https://github.com/n9e/fe/releases/download/${TAG}/n9e-fe-${TAG}.tar.gz && \
        tar zxf n9e-fe-${TAG}.tar.gz && \
        rm -f n9e-fe-${TAG}.tar.gz; \
    fi

# 使用 statik 嵌入前端静态文件到 Go 二进制中
RUN $(go env GOPATH)/bin/statik -src=./pub -dest=./front

# 编译生成 center 服务主程序 (n9e)
RUN GIT_COMMIT=$(git rev-parse 'HEAD^{commit}' 2>/dev/null || echo "unknown") && \
    RELEASE_VERSION="v6-docker-$GIT_COMMIT" && \
    go build -ldflags "-w -s -X github.com/ccfos/nightingale/v6/pkg/version.Version=${RELEASE_VERSION}" -o n9e ./cmd/center/main.go

# Stage 2: Final minimal runtime image
FROM alpine:3.18

# 安装运行时所需的 ca 证书、时区数据和常用网络命令
RUN apk add --no-cache ca-certificates tzdata bash curl && \
    ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

WORKDIR /app

# 从 builder 拷贝二进制及配置文件
COPY --from=builder /workspace/n9e /app/n9e
COPY --from=builder /workspace/etc /app/etc
COPY --from=builder /workspace/integrations /app/integrations

# 声明默认服务端口
EXPOSE 17000

# 默认容器启动命令
CMD ["/app/n9e", "--config", "/app/etc/config.toml"]
