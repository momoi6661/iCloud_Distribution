// Command hme-login 是分步调试 iCloud SRP 登录的工具。
//
// 用法:
//
//	HME_EMAIL=you@example.com HME_PASSWORD=xxx go run ./cmd/hme-login
//	HME_EMAIL=you@example.com HME_PASSWORD=xxx HME_OTP=123456 go run ./cmd/hme-login
//
// 也可以用 -proxy 指定代理:
//
//	go run ./cmd/hme-login -proxy http://127.0.0.1:7890
//
// 每一步都会打印结果,失败时显示具体状态码,用于定位授权问题。
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"icloud_distribution/internal/hme"
)

func main() {
	proxy := flag.String("proxy", "", "HTTP/SOCKS5 代理 (可选)")
	host := flag.String("host", "icloud.com", "iCloud 域名 (国区用 icloud.com.cn)")
	flag.Parse()

	email := os.Getenv("HME_EMAIL")
	password := os.Getenv("HME_PASSWORD")
	if email == "" || password == "" {
		fmt.Println("用法: HME_EMAIL=xxx HME_PASSWORD=yyy go run ./cmd/hme-login [-proxy ...] [-host icloud.com.cn]")
		fmt.Println("如需 2FA,程序会提示输入验证码")
		os.Exit(1)
	}
	email = strings.TrimSpace(email)

	fmt.Printf("[*] 账号: %s (长度 %d)  密码长度: %d  host: %s  proxy: %s\n",
		email, len(email), len(password), *host, emptyDash(*proxy))
	fmt.Println("[*] 提示: 如果密码长度与你预期不符,说明密码被 shell 转义了,请用单引号: HME_PASSWORD='你的密码'")

	client, err := hme.NewClient(nil, *host, *proxy, true)
	if err != nil {
		fmt.Printf("[!] 创建客户端失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[*] 开始 SRP 登录 (authStart → federate → init → complete) ...")
	err = client.BeginLogin(email, password)
	switch {
	case err == nil:
		fmt.Println("[+] 无需 2FA,登录直接完成")
	case errors.Is(err, hme.ErrOTPRequired):
		fmt.Println("[*] 账号需要双重认证 (2FA)")
		fmt.Println("[?] 选择验证方式: 1) 受信任设备推送  2) 手机短信")
		fmt.Print("    输入 1 或 2 [默认 1]: ")
		choice, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		choice = strings.TrimSpace(choice)

		if choice == "2" {
			// 手机短信通道
			phones, err := client.TrustedPhones()
			if err != nil {
				fmt.Printf("[!] 获取受信任手机号失败: %v\n", err)
				os.Exit(1)
			}
			if len(phones) == 0 {
				fmt.Println("[!] 账号没有受信任手机号")
				os.Exit(1)
			}
			phone := phones[0]
			if len(phones) > 1 {
				for i, p := range phones {
					fmt.Printf("    %d) %s\n", i+1, p.NumberWithDialCode)
				}
				fmt.Print("    选择手机号 [默认 1]: ")
				sel, _ := bufio.NewReader(os.Stdin).ReadString('\n')
				sel = strings.TrimSpace(sel)
				if n, err := strconv.Atoi(sel); err == nil && n >= 1 && n <= len(phones) {
					phone = phones[n-1]
				}
			}
			fmt.Printf("[*] 向 %s 发送短信验证码...\n", phone.NumberWithDialCode)
			if err := client.SendSMS(phone.ID); err != nil {
				fmt.Printf("[!] 发送失败: %v\n", err)
				os.Exit(1)
			}
			fmt.Print("[?] 请输入收到的 6 位短信验证码: ")
			code, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if err := client.CompleteSMS(phone.ID, strings.TrimSpace(code)); err != nil {
				fmt.Printf("[!] 短信验证失败: %v\n", err)
				os.Exit(1)
			}
		} else {
			// 受信任设备推送通道
			if err := client.ResendOTP(); err != nil {
				fmt.Printf("[!] 请求推送验证码失败 (%v),如已收到验证码可直接输入\n", err)
			} else {
				fmt.Println("[*] 已请求向受信任设备推送验证码,请查收 (iPhone/Mac 弹窗)")
			}
			fmt.Print("[?] 请输入 6 位验证码: ")
			code, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			code = strings.TrimSpace(code)
			if err := client.CompleteOTP(code); err != nil {
				fmt.Printf("[!] 2FA 阶段失败: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Println("[+] 2FA 验证通过,登录完成")
	default:
		fmt.Printf("[!] 登录失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[+] 成功获取 %d 个 Cookie\n", len(client.Cookies))
	for name := range client.Cookies {
		fmt.Printf("    - %s\n", name)
	}

	fmt.Println("[*] 校验会话 (validate) ...")
	if ok, err := client.Validate(); err != nil {
		fmt.Printf("[!] 会话校验失败: %v\n", err)
		os.Exit(1)
	} else if !ok {
		fmt.Println("[!] 会话无效")
		os.Exit(1)
	}
	fmt.Println("[+] 会话有效,授权流程全部正常")

	if info := client.AccountInfo(); info != nil {
		fmt.Printf("[+] 账号信息: appleId=%s primaryEmail=%s fullName=%s\n",
			info.AppleID, info.PrimaryEmail, info.FullName)
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
