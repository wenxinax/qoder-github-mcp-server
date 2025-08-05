# 多阶段构建 Dockerfile for qoder-github-mcp-server
FROM golang:1.23-alpine AS build
ARG VERSION="dev"

# 设置工作目录
WORKDIR /build

# 安装 git
RUN --mount=type=cache,target=/var/cache/apk \
    apk add git

# 构建应用
# go build 会自动下载所需的模块依赖到 /go/pkg/mod
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION} -X main.commit=$(git rev-parse HEAD 2>/dev/null || echo unknown) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /bin/qoder-github-mcp-server cmd/qoder-github-mcp-server/main.go

# 运行阶段
FROM gcr.io/distroless/base-debian12

# 设置工作目录
WORKDIR /server

# 从构建阶段复制二进制文件
COPY --from=build /bin/qoder-github-mcp-server .

# 设置入口
ENTRYPOINT ["/server/qoder-github-mcp-server"]

# 默认命令
CMD ["stdio"]
