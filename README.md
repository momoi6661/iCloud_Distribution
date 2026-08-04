# iCloud Distribution

iCloud Hide My Email 多账号管理平台 —— 带 Web UI、交互式自动授权和分享链接的临时邮箱系统。

通过逆向 iCloud Web 接口与 IMAP 协议，实现苹果「隐藏我的邮箱」(HME) 别名的自动化管理：登录一个 iCloud 账号，即可批量生成临时邮箱、读取每个邮箱的邮件，并通过带 token 的公开链接把某个邮箱分享给任何人（类似 cloudflare-temp-mail）。

## 功能特性

- ⚡ **交互式自动授权** —— 输入 Apple ID 密码即可，SRP 协议全流程自动完成；支持双重认证（受信任设备推送 / 手机短信双通道），无需手动抓 Cookie
- 📮 **别名管理** —— 创建 / 批量创建（单次 ≤50）/ 停用 / 激活 / 删除 / 搜索过滤
- 📨 **邮件读取** —— IMAP（App 专用密码，优先）+ Web API（Cookie，回退）双路径；同时扫描收件箱与垃圾邮件文件夹；IMAP 连接池复用
- 🔗 **分享链接** —— 为任意别名生成公开 token 链接，持链接者免登录只读查看该邮箱邮件，自动 30 秒轮询，可随时吊销
- 👥 **多账号** —— 国区 (icloud.com.cn) / 国际区，支持 HTTP/SOCKS5 代理
- 🔐 **UI 访问鉴权** —— 首跑创建管理员账号 (bcrypt)，HMAC 签名会话 Cookie；改密码旧会话全失效
- 🎨 **现代 Web UI** —— React + Ant Design，明暗层次分明的邮件阅读体验
- 🐳 **Docker 一键部署** —— 多阶段构建，单容器运行

## 快速开始

### 方式一：Docker（推荐）

```bash
git clone https://github.com/paipaiio/iCloud_Distribution.git
cd iCloud_Distribution
docker compose up -d --build
```

打开 http://localhost:8081 —— **首次访问会引导你创建管理员用户名和密码**（bcrypt 存储在 `data/admin.json`），之后用它登录。数据持久化在 `./data/`。

> 自动化场景也可以用静态口令跳过初始化：`HME_UI_TOKEN=xxx docker compose up -d --build`

### 方式二：本地构建

前置要求：Go 1.26+、Node.js 18+

```bash
make build                    # 前端构建 + Go 单二进制（内嵌前端）
./icloud_distribution         # 首跑创建管理员账号
# 或静态口令: ./icloud_distribution -token xxx
```

### 开发模式

```bash
make dev                      # 一条命令：后端 :8081 + Vite 热更新 :5173
```

## 使用流程

```
1. 添加账号（填 Apple ID，选对区域）
2. 点「自动授权」→ 输入 iCloud 密码
   → 双重认证：选「设备推送」或「手机短信」→ 输入验证码
   → Cookie 自动保存（密码不落盘）
3.（推荐）设置 App 专用密码 → IMAP 读邮件更快更全
4. 建议把转发地址切到 iCloud 邮箱（账号页一键切换）
   别名邮件才会进 iCloud 收件箱，面板才能读到
5. 创建别名 → 点「邮件」查看收件箱 → 点「分享」生成公开链接
```

> **注意**：HME 转发地址是账号级设置。若别名原来转发到外部邮箱（如 Outlook），
> 别名邮件不会进入 iCloud 邮箱，面板自然读不到 —— 先切换转发地址。

## API 概览

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/ui/login` | UI 口令登录 |
| POST | `/api/accounts/:id/login/start` | 自动授权阶段一（密码） |
| POST | `/api/accounts/:id/login/otp` | 自动授权阶段二（2FA，支持 device/sms） |
| GET | `/api/accounts/:id/login/phones` | 受信任手机号列表 |
| POST | `/api/accounts/:id/login/sms` | 发送短信验证码 |
| POST | `/api/accounts/:id/login/resend` | 重发设备推送 |
| GET/POST/DELETE | `/api/accounts` | 账号管理 |
| POST | `/api/create` | 创建别名 |
| POST | `/api/accounts/:id/aliases/batch` | 批量创建别名 |
| GET | `/api/aliases` | 别名列表（含转发地址） |
| POST | `/api/accounts/:id/forward-to` | 修改转发地址（账号级） |
| GET | `/api/inbox` | 读取邮件（IMAP 优先，Web API 回退） |
| GET | `/api/inbox/message` | 读取邮件正文 |
| POST | `/api/aliases/share` | 创建分享链接 |
| GET/DELETE | `/api/shares` | 分享管理 |
| GET | `/api/public/share/:token/*` | 公开分享端点（免登录） |

统一响应格式：`{success: bool, data: any, message: string}`

## 技术栈

- **后端**：Go 1.26 + Gin + tls-client（TLS 指纹模拟）+ go-imap
- **前端**：React 18 + Vite + TypeScript + Ant Design 5
- **认证**：SRP (Secure Remote Password) + HSA2 双重认证，基于对 idmsa.apple.com 协议的逆向

### 项目结构

```
├── main.go                  # 入口
├── internal/
│   ├── srp/                 # SRP 协议 (与 @foxt/js-srp GSA 模式逐字节一致,有对比测试)
│   ├── hme/                 # HME 客户端 + 两段式 SRP 登录 (BeginLogin/CompleteOTP/CompleteSMS)
│   ├── mail/                # IMAP (连接池, INBOX+Junk) + Web API 客户端
│   ├── account/             # 多账号管理 + IMAP 连接池 + 网关缓存
│   ├── auth/                # 登录会话存储 + UI 鉴权
│   ├── share/               # 分享链接存储
│   └── server/              # Gin 路由 + 内嵌前端
├── web/                     # React + Vite 前端
├── cmd/hme-login/           # SRP 登录调试工具
├── cmd/hme-mail/            # 邮件读取调试工具
└── Dockerfile               # 三阶段构建: 前端 → 后端 → alpine 运行时
```

## 调试工具

```bash
# 分步调试 SRP 登录 (打印每一步状态,支持 2FA 两种通道)
HME_EMAIL=you@example.com HME_PASSWORD=xxx go run ./cmd/hme-login -host icloud.com.cn

# 调试邮件读取 (Web API / IMAP / 别名过滤 / 正文)
go run ./cmd/hme-mail -account acc_xxx -imap -alias x@icloud.com
go run ./cmd/hme-mail -account acc_xxx -imap -uid 3 -folder Junk
```

## 安全说明

- iCloud 密码仅存在于内存中的登录会话（TTL 5 分钟），**不落盘**
- `data/accounts.json`、`data/shares.json`、`data/admin.json` 权限 0600，含敏感凭证，请妥善保护
- 管理员密码以 bcrypt 哈希存储；会话签名密钥派生自密码哈希，改密码后旧会话全部失效
- 分享链接任何人持有即可读邮件（设计如此），只发给信任的人；可随时吊销

## 测试

```bash
go vet ./...
go test -race ./...
```

## 许可证

MIT License
