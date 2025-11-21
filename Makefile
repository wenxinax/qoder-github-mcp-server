# Makefile for qoder-github-mcp-server packaging & release

SHELL := /bin/bash
.DEFAULT_GOAL := build

# -------------------------------------------------------------------
# Project metadata
# -------------------------------------------------------------------
BINARY_NAME        ?= qoder-github-mcp-server
CMD_DIR            ?= ./cmd/qoder-github-mcp-server
BUILD_DIR          ?= build
DIST_DIR           ?= dist
EXTRA_PACKAGE_FILES?= README.md LICENSE
OS_ARCHES          ?= darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64
CGO_ENABLED        ?= 0
GO_BUILD_TAGS      ?=
GO                 ?= go

# Version metadata (VERSION can be overridden: `make package VERSION=1.2.3`)
VERSION ?= $(shell git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//')
ifeq ($(strip $(VERSION)),)
  VERSION := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
endif
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE   ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS       := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"
GO_TAGS_FLAG  := $(if $(strip $(GO_BUILD_TAGS)),-tags "$(strip $(GO_BUILD_TAGS))",)

CHECKSUM_TOOL ?= $(shell if command -v shasum >/dev/null 2>&1; then echo "shasum -a 256"; elif command -v sha256sum >/dev/null 2>&1; then echo "sha256sum"; else echo ""; fi)

# Docker metadata
DOCKER_REGISTRY ?= ghcr.io
DOCKER_REPO     ?= qoder/qoder-github-mcp-server
DOCKER_TAG      ?= $(VERSION)
DOCKER_IMAGE     = $(DOCKER_REGISTRY)/$(DOCKER_REPO):$(DOCKER_TAG)

.PHONY: \
	all build clean distclean fmt lint test install deps package checksums \
	verify-version verify-clean-tree release docker-build docker-push \
	docker-run docker-shell help

# -------------------------------------------------------------------
# Developer utilities
# -------------------------------------------------------------------
all: build ## 默认目标：构建本地二进制

build: ## 在本机生成未压缩的二进制
	@mkdir -p $(BUILD_DIR)
	@echo "==> Building $(BINARY_NAME) ($(GOOS)/$(GOARCH))"
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GO_TAGS_FLAG) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)

deps: ## 安装/更新依赖
	$(GO) mod tidy
	$(GO) mod download

fmt: ## gofmt 项目源码
	gofmt -w $$(find . -name '*.go' -not -path "./vendor/*")

lint: ## go vet 静态检查
	$(GO) vet ./...

test: ## 运行单元测试
	$(GO) test ./...

install: ## 安装到 GOPATH/bin
	CGO_ENABLED=$(CGO_ENABLED) $(GO) install $(GO_TAGS_FLAG) $(LDFLAGS) $(CMD_DIR)

clean: ## 清理本地构建产物
	rm -rf $(BUILD_DIR)
	$(GO) clean

distclean: clean ## 清空 dist 目录
	rm -rf $(DIST_DIR)

# -------------------------------------------------------------------
# Packaging & release
# -------------------------------------------------------------------
package: ## 构建所有平台的发布包（tar.gz / zip）
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@set -euo pipefail; \
	for target in $(OS_ARCHES); do \
		os="$${target%/*}"; arch="$${target#*/}"; \
		basename="$(BINARY_NAME)-$(VERSION)-$${os}-$${arch}"; \
		tmp_dir="$(DIST_DIR)/$${basename}"; \
		exe_ext=""; archive_ext="tar.gz"; \
		if [ "$${os}" = "windows" ]; then exe_ext=".exe"; archive_ext="zip"; fi; \
		echo "==> Packaging $${basename}"; \
		mkdir -p "$${tmp_dir}"; \
		GOOS=$${os} GOARCH=$${arch} CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GO_TAGS_FLAG) $(LDFLAGS) -o "$${tmp_dir}/$(BINARY_NAME)$${exe_ext}" $(CMD_DIR); \
		for extra in $(EXTRA_PACKAGE_FILES); do \
			[ -f "$${extra}" ] && cp "$${extra}" "$${tmp_dir}/"; \
		done; \
		if [ "$${archive_ext}" = "zip" ]; then \
			( cd "$(DIST_DIR)" && zip -rq "$${basename}.zip" "$${basename}" ); \
		else \
			( cd "$(DIST_DIR)" && tar -czf "$${basename}.tar.gz" "$${basename}" ); \
		fi; \
		rm -rf "$${tmp_dir}"; \
	done
	@$(MAKE) checksums

checksums: ## 生成 SHA256 校验文件
	@if [ -z "$(CHECKSUM_TOOL)" ]; then \
		echo "未找到 shasum/sha256sum，无法生成校验文件" >&2; \
		exit 1; \
	fi
	@if [ ! -d "$(DIST_DIR)" ]; then \
		echo "$(DIST_DIR) 不存在，请先运行 make package" >&2; \
		exit 1; \
	fi
	@cd $(DIST_DIR) && \
	files=$$(find . -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.zip' \) | sed 's|^\./||' | sort); \
	if [ -z "$$files" ]; then \
		echo "未找到归档文件，跳过校验。" >&2; \
		exit 1; \
	fi; \
	echo "$$files" | xargs $(CHECKSUM_TOOL) > SHA256SUMS
	@echo "==> SHA256SUMS 生成完成：$(DIST_DIR)/SHA256SUMS"

verify-version: ## 确认 VERSION 已设置
	@if [ -z "$(strip $(VERSION))" ] || [ "$(strip $(VERSION))" = "dev" ]; then \
		echo "请通过 VERSION=x.y.z 指定正式版本号，再执行发布打包流程。" >&2; \
		exit 1; \
	fi

verify-clean-tree: ## 确认 git 工作区干净
	@if ! git diff --quiet || ! git diff --cached --quiet; then \
		echo "工作区存在未提交的改动，请先提交或暂存。" >&2; \
		exit 1; \
	fi

release: verify-version verify-clean-tree test package ## 完整发布流程（校验+测试+打包）
	@echo "==> Release artifacts 位于 $(DIST_DIR)"

# -------------------------------------------------------------------
# Docker helpers
# -------------------------------------------------------------------
docker-build: ## 构建 Docker 镜像
	DOCKER_BUILDKIT=1 docker build \
		--build-arg VERSION=$(VERSION) \
		-t $(DOCKER_IMAGE) \
		.

docker-push: docker-build ## 推送 Docker 镜像
	docker push $(DOCKER_IMAGE)

docker-run: ## 以交互方式运行镜像
	docker run --rm -it \
		-e GITHUB_TOKEN \
		-e GITHUB_OWNER \
		-e GITHUB_REPO \
		$(DOCKER_IMAGE)

docker-shell: ## 启动交互式 shell
	docker run --rm -it \
		-e GITHUB_TOKEN \
		-e GITHUB_OWNER \
		-e GITHUB_REPO \
		--entrypoint /bin/sh \
		$(DOCKER_IMAGE)

# -------------------------------------------------------------------
# Help
# -------------------------------------------------------------------
help: ## 显示可用目标
	@echo "可用目标："
	@grep -E '^[a-zA-Z0-9_-]+:.*## ' $(MAKEFILE_LIST) | sort | while read -r line; do \
		target=$$(echo $$line | cut -d':' -f1); \
		desc=$$(echo $$line | sed -e 's/^[^#]*## //'); \
		printf "  %-18s %s\n" $$target "$$desc"; \
	done
