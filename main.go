// Command icloud_distribution 启动 iCloud Hide My Email 多账号管理平台 (含 Web UI)。
//
// 用法:
//
//	./icloud_distribution                       # 默认 :8081
//	./icloud_distribution -addr :9000           # 指定端口
//	./icloud_distribution -data ./data          # 指定数据目录
//	./icloud_distribution -token <口令>          # 启用 UI 访问鉴权
//	./icloud_distribution -debug                # 调试模式 (Gin 请求日志)
//
// 环境变量:
//
//	HME_UI_TOKEN  UI 访问口令 (与 -token 等价,命令行优先)
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
	addr := flag.String("addr", ":8081", "HTTP 监听地址")
	dataDir := flag.String("data", "./data", "数据目录 (accounts.json 存放位置)")
	token := flag.String("token", "", "UI 访问口令 (也可用环境变量 HME_UI_TOKEN)")
	debug := flag.Bool("debug", false, "调试模式 (启用 Gin 调试日志)")
	flag.Parse()

	uiToken := *token
	if uiToken == "" {
		uiToken = os.Getenv("HME_UI_TOKEN")
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

	// 鉴权: -token/HME_UI_TOKEN 为令牌模式; 否则为管理员账号模式 (首跑创建)
	var ui *auth.UIAuth
	var creds *auth.CredentialStore
	if uiToken != "" {
		ui = auth.NewUIAuth(uiToken)
		log.Printf("鉴权模式: 启动令牌")
	} else {
		creds, err = auth.NewCredentialStore(abs)
		if err != nil {
			log.Fatalf("初始化凭证存储失败: %v", err)
		}
		ui = auth.NewUIAuthFromCredentials(creds)
		log.Printf("鉴权模式: 管理员账号 (initialized=%v)", creds.Initialized())
	}

	srv := server.New(mgr, logins, ui, creds, shares, server.StaticFS(), *debug)

	log.Printf("HTTP 服务就绪 addr=%s", *addr)
	if err := srv.Run(*addr); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
