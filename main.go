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
//	HME_SUPERADMIN_USERNAME  超级管理员用户名 (必填)
//	HME_SUPERADMIN_PASSWORD  超级管理员密码 (必填)
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

	superUsername := os.Getenv("HME_SUPERADMIN_USERNAME")
	superPassword := os.Getenv("HME_SUPERADMIN_PASSWORD")
	if superUsername == "" || superPassword == "" {
		log.Fatal("缺少环境变量 HME_SUPERADMIN_USERNAME 或 HME_SUPERADMIN_PASSWORD，服务拒绝启动")
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
	if err := mgr.AssignMissingOwners(auth.SuperadminID); err != nil {
		log.Fatalf("迁移账号归属失败: %v", err)
	}
	log.Printf("账号加载完成 count=%d data_dir=%s", len(mgr.ListAccounts()), abs)

	logins := auth.NewLoginStore()
	defer logins.Close()

	shares, err := share.NewStore(abs)
	if err != nil {
		log.Fatalf("初始化分享存储失败: %v", err)
	}
	if err := shares.AssignMissingOwners(func(accountID string) string {
		owner, _ := mgr.Owner(accountID)
		if owner == "" {
			return auth.SuperadminID
		}
		return owner
	}); err != nil {
		log.Fatalf("迁移分享归属失败: %v", err)
	}

	users, err := auth.NewUserStore(filepath.Join(abs, "users.db"))
	if err != nil {
		log.Fatalf("初始化用户数据库失败: %v", err)
	}
	defer users.Close()
	ui, err := auth.NewUIAuth(users, superUsername, superPassword)
	if err != nil {
		log.Fatalf("初始化 UI 鉴权失败: %v", err)
	}
	log.Printf("鉴权模式: 多用户，超级管理员=%s", superUsername)

	srv := server.New(mgr, logins, ui, shares, server.StaticFS(), *debug)

	log.Printf("HTTP 服务就绪 addr=%s", *addr)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
