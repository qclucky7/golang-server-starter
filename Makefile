# gin-quick-start 开发命令
# Windows 用户可直接使用 make.bat（命令与 target 同名）

APP_NAME    := gin-quick-start
MAIN_PKG    := ./cmd/server
BIN_DIR     := bin
SWAG        := swag
GO          := go

# 运行环境（dev / test / prod），可用 make run ENV=prod 覆盖
ENV         ?= dev

.PHONY: help
help: ## 显示全部可用命令
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

.PHONY: run
run: ## 本地启动服务（ENV=xxx 指定环境，默认 dev）
	$(GO) run $(MAIN_PKG) -e $(ENV)

.PHONY: build
build: ## 编译当前平台可执行文件到 bin/
	$(GO) build -trimpath -ldflags "-s -w" -o $(BIN_DIR)/$(APP_NAME) $(MAIN_PKG)

.PHONY: build-linux
build-linux: ## 交叉编译 Linux amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o $(BIN_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PKG)

.PHONY: build-windows
build-windows: ## 交叉编译 Windows amd64
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o $(BIN_DIR)/$(APP_NAME).exe $(MAIN_PKG)

.PHONY: docs
docs: ## 生成 Swagger 文档到 docs/
	$(SWAG) init -g cmd/server/main.go -o docs --parseInternal

.PHONY: gqlgen
gqlgen: ## 根据 internal/graphql/schema.graphqls 生成 GraphQL 代码
	$(GO) run github.com/99designs/gqlgen generate

.PHONY: fmt
fmt: ## 格式化代码
	$(GO) fmt ./...

.PHONY: vet
vet: ## 静态检查
	$(GO) vet ./...

.PHONY: tidy
tidy: ## 整理依赖
	$(GO) mod tidy

.PHONY: test
test: ## 运行单元测试
	$(GO) test ./... -count=1

.PHONY: cover
cover: ## 运行测试并输出覆盖率
	$(GO) test ./... -count=1 -coverprofile=coverage.out
	$(GO) tool cover -func=coverage.out

.PHONY: check
check: fmt vet test ## 格式化 + 静态检查 + 测试

.PHONY: clean
clean: ## 清理构建产物
	rm -rf $(BIN_DIR) coverage.out

.PHONY: docker
docker: ## 构建 Docker 镜像
	docker build -t $(APP_NAME):latest .
