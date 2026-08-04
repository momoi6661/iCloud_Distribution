// Command hme-mail 调试指定账号的邮件读取 (Web API 路径)。
//
// 用法:
//
//	go run ./cmd/hme-mail -account acc_xxx [-alias x@icloud.com] [-data ./data] [-limit 10]
//
// 打印每一步的结果: 网关解析、收件箱摘要、thread/get 元数据、按别名过滤。
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"icloud_distribution/internal/account"
)

// runIMAP 通过 IMAP 连接池读取邮件,打印耗时。
func runIMAP(mgr *account.Manager, accountID, alias string, limit int, uid uint, folder string) {
	start := time.Now()
	mc, unlock, err := mgr.AcquireIMAP(accountID)
	if err != nil {
		fmt.Printf("[!] IMAP 连接失败: %v\n", err)
		os.Exit(1)
	}
	defer unlock.Unlock()
	fmt.Printf("[*] 连接就绪 (耗时 %v)\n", time.Since(start).Round(time.Millisecond))

	if uid > 0 {
		start = time.Now()
		full, err := mc.GetFull(uint32(uid), folder)
		if err != nil {
			fmt.Printf("[!] 读取正文失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[+] 正文 (%v):\n主题: %s\n发件人: %s\nContentType: %s\n正文长度: %d\n前 500 字符:\n%.500s\n",
			time.Since(start).Round(time.Millisecond), full.Subject, full.From, full.ContentType, len(full.Body), full.Body)
		return
	}

	start = time.Now()
	var msgs interface {
	}
	_ = msgs
	var list []struct {
		Date, From, To, Subject, Preview, Folder string
	}
	if alias != "" {
		fmt.Printf("[*] IMAP 按别名查找: %s\n", alias)
		found, err := mc.FindByRecipient(alias, limit, 30)
		if err != nil {
			fmt.Printf("[!] 查找失败: %v\n", err)
			os.Exit(1)
		}
		for _, m := range found {
			list = append(list, struct{ Date, From, To, Subject, Preview, Folder string }{m.Date, m.From, m.To, m.Subject, m.Preview, m.Folder})
		}
	} else {
		fmt.Printf("[*] IMAP 列出收件箱 (limit=%d)\n", limit)
		found, err := mc.ListInbox(limit, 0)
		if err != nil {
			fmt.Printf("[!] 读取失败: %v\n", err)
			os.Exit(1)
		}
		for _, m := range found {
			list = append(list, struct{ Date, From, To, Subject, Preview, Folder string }{m.Date, m.From, m.To, m.Subject, m.Preview, m.Folder})
		}
	}
	fmt.Printf("[+] 共 %d 封 (耗时 %v):\n", len(list), time.Since(start).Round(time.Millisecond))
	for _, m := range list {
		fmt.Printf("    [%s] [%s] %s → %s | %s\n", m.Date, m.Folder, m.From, m.To, m.Subject)
		if m.Preview != "" {
			fmt.Printf("        预览: %.80s\n", m.Preview)
		}
	}
}

func main() {
	accountID := flag.String("account", "", "账号 ID (acc_xxx)")
	alias := flag.String("alias", "", "按别名过滤 (可选)")
	dataDir := flag.String("data", "./data", "数据目录")
	limit := flag.Int("limit", 10, "拉取数量")
	listAliases := flag.Bool("aliases", false, "列出 HME 别名及状态")
	useIMAP := flag.Bool("imap", false, "使用 IMAP (连接池) 而非 Web API")
	uid := flag.Uint("uid", 0, "读取指定 uid 邮件正文")
	folder := flag.String("folder", "", "uid 所在文件夹 (默认 INBOX)")
	flag.Parse()

	if *accountID == "" {
		fmt.Println("用法: go run ./cmd/hme-mail -account acc_xxx [-alias x@icloud.com] [-limit 10] [-aliases] [-imap] [-uid N -folder Junk]")
		os.Exit(1)
	}

	mgr, err := account.NewManager(*dataDir)
	if err != nil {
		fmt.Printf("[!] 初始化失败: %v\n", err)
		os.Exit(1)
	}

	if *useIMAP {
		runIMAP(mgr, *accountID, *alias, *limit, *uid, *folder)
		return
	}

	if *listAliases {
		hc, err := mgr.HMEClient(*accountID, false)
		if err != nil {
			fmt.Printf("[!] 创建 HME 客户端失败: %v\n", err)
			os.Exit(1)
		}
		aliases, err := hc.ListAliases()
		if err != nil {
			fmt.Printf("[!] 列出别名失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[+] 共 %d 个别名:\n", len(aliases))
		for _, a := range aliases {
			status := "启用"
			if !a.Active {
				status = "停用"
			}
			fmt.Printf("    [%s] %s (%s)\n", status, a.Email, a.Label)
		}
		return
	}

	wc, err := mgr.WebMailClient(*accountID)
	if err != nil {
		fmt.Printf("[!] 创建 Web 邮件客户端失败: %v\n", err)
		os.Exit(1)
	}
	wc.Verbose = true

	if *alias != "" {
		fmt.Printf("[*] 按别名查找: %s (limit=%d)\n", *alias, *limit)
		msgs, err := wc.FindByAlias(*alias, *limit)
		if err != nil {
			fmt.Printf("[!] 查找失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[+] 匹配 %d 封:\n", len(msgs))
		for _, m := range msgs {
			fmt.Printf("    [%s] %s → %s | %s\n", m.Date, m.From, m.To, m.Subject)
		}
		return
	}

	fmt.Printf("[*] 列出收件箱 (limit=%d)\n", *limit)
	msgs, err := wc.ListInbox(*limit)
	if err != nil {
		fmt.Printf("[!] 读取失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[+] 共 %d 封:\n", len(msgs))
	for _, m := range msgs {
		fmt.Printf("    [%s] %s | %s\n", m.Date, m.From, m.Subject)
	}

	// 顺带检查垃圾邮件文件夹 (HME 转发邮件可能被判垃圾邮件)
	fmt.Println("[*] 检查垃圾邮件文件夹 (Junk) ...")
	junk, err := wc.ListFolder("Junk", *limit)
	if err != nil {
		fmt.Printf("[!] Junk 读取失败: %v\n", err)
		return
	}
	fmt.Printf("[+] Junk 共 %d 封:\n", len(junk))
	for _, m := range junk {
		fmt.Printf("    [%s] %s | %s\n", m.Date, m.From, m.Subject)
	}
}
