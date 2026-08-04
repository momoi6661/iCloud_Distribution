# ============================================================
# icloud_distribution 多阶段构建: 前端 → 后端 → 极简运行时
#
# 一条命令部署:
#   docker compose up -d --build
# 或:
#   docker build -t icloud_distribution .
#   docker run -d -p 8081:8081 -e HME_UI_TOKEN=你的口令 \
#     -v $(pwd)/data:/app/data icloud_distribution
# ============================================================

# ---- 阶段 1: 构建前端 (产物输出到 internal/server/web/dist) ----
FROM node:20-alpine AS web
WORKDIR /app
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci --no-audit --no-fund
COPY web/ ./web/
RUN cd web && npm run build

# ---- 阶段 2: 构建后端 (embed 内嵌前端产物) ----
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 用阶段 1 的前端产物覆盖占位目录
COPY --from=web /app/internal/server/web/dist ./internal/server/web/dist
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o icloud_distribution .

# ---- 阶段 3: 运行时 (仅二进制 + CA 证书) ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=backend /app/icloud_distribution .

# 数据目录: accounts.json 持久化 (含敏感凭证,务必挂载卷并保护)
VOLUME ["/app/data"]
EXPOSE 8081

# UI 访问口令在运行时通过 -e HME_UI_TOKEN=xxx 注入 (程序直接读环境变量,无需在此声明)

ENTRYPOINT ["/app/icloud_distribution"]
CMD ["-addr", ":8081", "-data", "/app/data"]
