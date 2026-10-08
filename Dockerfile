# ---------- 构建阶段 ----------
FROM golang:1.26-alpine AS builder

WORKDIR /src

# 依赖单独一层，充分利用构建缓存
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 全部依赖都是纯 Go 实现（sqlite 用 glebarez/sqlite），无需 CGO
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags "-s -w" -o /out/app ./cmd/server

# ---------- 运行阶段 ----------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime

WORKDIR /app

COPY --from=builder /out/app /app/app
COPY configs /app/configs

RUN mkdir -p /app/data /app/logs

# 生效的环境名，对应 configs/config-prod.yaml
ENV APP_ENV=prod \
    APP_SERVER_HOST=0.0.0.0 \
    APP_SERVER_PORT=8080 \
    APP_LOG_DIR=/app/logs

# 以下三项没有可用默认值，必须在部署时注入（docker run -e / k8s Secret）：
#   APP_JWT_SECRET        令牌签名密钥，长度 >= 32
#   APP_DATABASE_DSN      MySQL DSN，必须带 parseTime=True（默认驱动；换 PostgreSQL 用 sslmode=disable）
#   APP_NODE_ID           雪花算法节点 ID，多实例部署时逐实例配置且互不相同
#
# 例：
#   docker run -p 8080:8080 \
#     -e APP_JWT_SECRET=xxxx \
#     -e APP_DATABASE_DSN='app:xxx@tcp(mysql:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local' \
#     -e APP_NODE_ID=1 \
#     -v $PWD/logs:/app/logs gin-quick-start:latest

EXPOSE 8080

VOLUME ["/app/logs"]

ENTRYPOINT ["/app/app"]
CMD ["-c", "/app/configs/config.yaml"]
