# Qoder GitHub MCP 服务器

专为 Qoder 操作设计的 GitHub Model Context Protocol (MCP) 服务器。

## 功能特性

- **qoder_update_comment**: 更新 GitHub 评论（支持 issue 评论和 pull request review 评论）。并非完全覆盖更新整个 comment 内容，而是替换 Qoder 标记之间的内容，达到部分更新的目的。

## 安装

### 前置要求

- Go 1.21 或更高版本
- 具有适当权限的 GitHub Personal Access Token

### 从源码构建

```bash
git clone <repository-url>
cd qoder-github-mcp-server
go mod tidy
go build -o qoder-github-mcp-server ./cmd/qoder-github-mcp-server
```

### 使用 Docker

#### 拉取预构建镜像

```bash
docker pull ghcr.io/your-username/qoder-github-mcp-server:latest
```

#### 本地构建镜像

```bash
make docker-build
```

#### 使用 Docker Compose

```bash
# 设置环境变量
cp .env.example .env
# 编辑 .env 文件设置你的配置

# 启动服务
docker-compose up qoder-github-mcp-server
```

### Docker 镜像特性

- **轻量级**: 基于 Google 的 distroless 镜像，只有 ~41MB
- **安全**: 无 shell 和包管理器，减少攻击面
- **多架构**: 支持 linux/amd64 和 linux/arm64
- **缓存优化**: 使用 BuildKit 缓存加速构建

## 配置

服务器需要以下环境变量：

- `GITHUB_TOKEN`: 你的 GitHub Personal Access Token
- `GITHUB_OWNER`: 仓库所有者（用户名或组织名）
- `GITHUB_REPO`: 仓库名称
- `QODER_COMMENT_ID`: 要更新的评论 ID
- `QODER_COMMENT_TYPE`: 评论类型（"issue" 或 "review"）

### 环境变量设置示例

```bash
export GITHUB_TOKEN="ghp_your_token_here"
export GITHUB_OWNER="your-username"
export GITHUB_REPO="your-repo"
export QODER_COMMENT_ID="123456789"
export QODER_COMMENT_TYPE="issue"  # 或 "review"
```

你也可以创建一个 `.env` 文件（参考 `.env.example`）。

## 使用方法

### 启动 MCP 服务器

#### 使用二进制文件

```bash
# 设置必需的环境变量
export GITHUB_TOKEN="ghp_your_token_here"
export GITHUB_OWNER="your-username"
export GITHUB_REPO="your-repo"
export QODER_COMMENT_ID="123456789"
export QODER_COMMENT_TYPE="issue"

# 启动服务器
./qoder-github-mcp-server stdio
```

#### 使用 Docker

```bash
# 使用环境变量运行
docker run --rm -it \
  -e GITHUB_TOKEN="ghp_your_token_here" \
  -e GITHUB_OWNER="your-username" \
  -e GITHUB_REPO="your-repo" \
  -e QODER_COMMENT_ID="123456789" \
  -e QODER_COMMENT_TYPE="issue" \
  ghcr.io/qoder/qoder-github-mcp-server:latest

# 或使用 Makefile
make docker-run

# 测试 Docker 镜像
./test_docker.sh
```

服务器通过标准输入/输出使用 JSON-RPC 消息进行通信，遵循 MCP 协议。

### 测试服务器

运行包含的测试脚本来验证基本功能：

```bash
./test_server.sh
```

### 可用工具

#### qoder_update_comment

更新评论（issue 评论或 pull request review 评论），替换 `<!-- QODER_BODY_START -->` 和 `<!-- QODER_BODY_END -->` 标记之间的内容。

**参数：**
- `new_content` (必需): 要替换到 Qoder 标记之间的新内容

**配置（通过环境变量）：**
- 仓库所有者、仓库名、评论 ID 和评论类型通过环境变量配置

**使用示例：**
```json
{
  "method": "tools/call",
  "params": {
    "name": "qoder_update_comment",
    "arguments": {
      "new_content": "这是来自 Qoder 的更新内容"
    }
  }
}
```

## 评论格式

工具期望评论包含 Qoder 标记：

```markdown
一些现有内容...

<!-- QODER_BODY_START -->
这部分内容将被替换
<!-- QODER_BODY_END -->

更多现有内容...
```

运行工具后，标记之间的内容将被新内容替换。

## 错误处理

服务器将为以下情况返回适当的错误消息：
- 缺失或无效的环境变量
- GitHub API 错误（认证、速率限制等）
- 评论中缺失 Qoder 标记
- 无效参数

## 扩展服务器

添加新工具：

1. 在 `pkg/qoder/tools.go` 中创建新的工具函数
2. 在 `pkg/qoder/server.go` 的 `registerTools` 函数中注册工具
3. 遵循现有的参数验证和错误处理模式

## 开发

### 运行测试

```bash
go test ./...
```

### 为不同平台构建

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o qoder-github-mcp-server-linux ./cmd/qoder-github-mcp-server

# macOS
GOOS=darwin GOARCH=amd64 go build -o qoder-github-mcp-server-macos ./cmd/qoder-github-mcp-server

# Windows
GOOS=windows GOARCH=amd64 go build -o qoder-github-mcp-server.exe ./cmd/qoder-github-mcp-server
```

或者使用 Makefile：

```bash
make build-all
```
