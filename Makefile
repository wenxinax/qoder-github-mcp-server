# Makefile for qoder-github-mcp-server

# Variables
BINARY_NAME=qoder-github-mcp-server
BUILD_DIR=.
CMD_DIR=./cmd/qoder-github-mcp-server

# Build information
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Build flags
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

# Docker variables
DOCKER_REGISTRY ?= ghcr.io
DOCKER_REPO ?= qoder/qoder-github-mcp-server
DOCKER_TAG ?= latest
DOCKER_IMAGE = $(DOCKER_REGISTRY)/$(DOCKER_REPO):$(DOCKER_TAG)

.PHONY: all build clean test help install deps docker-build docker-push docker-run

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# Run tests
test:
	@echo "Running Go tests..."
	go test ./...
	@echo "Running integration tests..."
	./test_server.sh

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	go clean

# Install the binary to GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME) to GOPATH/bin..."
	go install $(LDFLAGS) $(CMD_DIR)

# Build for multiple platforms
build-all:
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)
	@echo "Multi-platform build complete"

# Docker commands
docker-build:
	@echo "Building Docker image: $(DOCKER_IMAGE)"
	DOCKER_BUILDKIT=1 docker build \
		--build-arg VERSION=$(VERSION) \
		-t $(DOCKER_IMAGE) .

docker-push: docker-build
	@echo "Pushing Docker image: $(DOCKER_IMAGE)"
	docker push $(DOCKER_IMAGE)

docker-run:
	@echo "Running Docker container: $(DOCKER_IMAGE)"
	docker run --rm -it \
		-e GITHUB_TOKEN \
		-e GITHUB_OWNER \
		-e GITHUB_REPO \
		-e QODER_COMMENT_ID \
		-e QODER_COMMENT_TYPE \
		$(DOCKER_IMAGE)

docker-shell:
	@echo "Running Docker container shell: $(DOCKER_IMAGE)"
	docker run --rm -it \
		-e GITHUB_TOKEN \
		-e GITHUB_OWNER \
		-e GITHUB_REPO \
		-e QODER_COMMENT_ID \
		-e QODER_COMMENT_TYPE \
		--entrypoint /bin/sh \
		$(DOCKER_IMAGE)

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  deps         - Install dependencies"
	@echo "  test         - Run tests"
	@echo "  clean        - Clean build artifacts"
	@echo "  install      - Install binary to GOPATH/bin"
	@echo "  build-all    - Build for multiple platforms"
	@echo "  docker-build - Build Docker image"
	@echo "  docker-push  - Build and push Docker image"
	@echo "  docker-run   - Run Docker container"
	@echo "  docker-shell - Run Docker container with shell"
	@echo "  help         - Show this help message"
