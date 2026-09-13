// Command icloud_distribution 启动 iCloud Hide My Email 多账号管理平台 (含 Web UI)。
//
// 用法:
//
//	./icloud_distribution                       # 默认 :6981
//	./icloud_distribution -addr :9000           # 指定端口
//	./icloud_distribution -data ./data          # 指定数据目录
//	./icloud_distribution -debug                # 调试模式 (Gin 请求日志)
//
// 环境变量:
//
//	HME_UI_PASSWORD  UI 登录密码 (必填)
package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"icloud_distribution/internal/account"
	"icloud_distribution/internal/auth"
	"icloud_distribution/internal/server"
	"icloud_distribution/internal/share"
)

func main() {
	addr := flag.String("addr", ":6981", "HTTP 监听地址")
	dataDir := flag.String("data", "./data", "数据目录 (accounts.json 存放位置)")
	debug := flag.Bool("debug", false, "调试模式 (启用 Gin 调试日志)")
	flag.Parse()

	uiPassword := os.Getenv("HME_UI_PASSWORD")
	if uiPassword == "" {
		log.Fatal("缺少环境变量 HME_UI_PASSWORD，服务拒绝启动")
	}

	log.Printf("iCloud Distribution 启动 addr=%s", *addr)

	abs, err := filepath.Abs(*dataDir)
	if err != nil {
		log.Fatalf("数据目录路径错误: %v", err)
	}

	mgr, err := account.NewManager(abs)
	if err != nil {
		log.Fatalf("初始化账号管理器失败: %v", err)
	}
	log.Printf("账号加载完成 count=%d data_dir=%s", len(mgr.ListAccounts()), abs)

	logins := auth.NewLoginStore()
	defer logins.Close()

	shares, err := share.NewStore(abs)
	if err != nil {
		log.Fatalf("初始化分享存储失败: %v", err)
	}

	ui := auth.NewUIAuth(uiPassword)
	log.Printf("鉴权模式: 环境变量密码")

	srv := server.New(mgr, logins, ui, shares, server.StaticFS(), *debug)

	log.Printf("HTTP 服务就绪 addr=%s", *addr)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
