# icloud_distribution 常用命令
#
# 开发:   make dev                — 一条命令同时启动后端 (:6981) 和前端热更新 (:5173)
#         make dev HME_UI_PASSWORD=xxx — 指定 UI 登录密码
# 构建:   make build    — 前端构建 + Go 单二进制
# 测试:   make test     — go vet + go test -race
# 部署:   make docker   — Docker 一键构建并启动

# UI 登录密码必须通过环境变量提供
PASSWORD ?= $(HME_UI_PASSWORD)

.PHONY: dev build test docker clean

dev: ## 开发模式: 后端 + Vite 热更新 (Ctrl+C 同时停止)
	@if [ -z "$(PASSWORD)" ]; then echo "请设置 HME_UI_PASSWORD"; exit 1; fi; \
	trap 'kill 0' EXIT INT; \
	HME_UI_PASSWORD="$(PASSWORD)" go run . -debug & \
	cd web && npm run dev

build: ## 构建单二进制 (含内嵌前端)
	cd web && npm run build
	go build -ldflags='-s -w' -o icloud_distribution .

test: ## 静态检查 + 竞态测试
	go vet ./...
	go test -race ./...

docker: ## Docker 一键构建并启动 (HME_UI_PASSWORD=xxx make docker)
	docker compose up -d --build

clean: ## 清理构建产物
	rm -f icloud_distribution
	rm -rf internal/server/web/dist/assets
