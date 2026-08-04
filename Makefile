# icloud_distribution 常用命令
#
# 开发:   make dev                — 一条命令同时启动后端 (:8081) 和前端热更新 (:5173)
#         make dev TOKEN=xxx      — 指定 UI 访问口令 (默认 dev123)
# 构建:   make build    — 前端构建 + Go 单二进制
# 测试:   make test     — go vet + go test -race
# 部署:   make docker   — Docker 一键构建并启动

# UI 访问口令: make dev TOKEN=xxx 或 HME_UI_TOKEN=xxx make dev
TOKEN ?= $(or $(HME_UI_TOKEN),dev123)

.PHONY: dev build test docker clean

dev: ## 开发模式: 后端 + Vite 热更新 (Ctrl+C 同时停止)
	@trap 'kill 0' EXIT INT; \
	if [ -n "$(TOKEN)" ]; then go run . -debug -token $(TOKEN) & else go run . -debug & fi; \
	cd web && npm run dev

build: ## 构建单二进制 (含内嵌前端)
	cd web && npm run build
	go build -ldflags='-s -w' -o icloud_distribution .

test: ## 静态检查 + 竞态测试
	go vet ./...
	go test -race ./...

docker: ## Docker 一键构建并启动 (HME_UI_TOKEN=xxx make docker)
	docker compose up -d --build

clean: ## 清理构建产物
	rm -f icloud_distribution
	rm -rf internal/server/web/dist/assets
