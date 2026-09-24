# 项目级 FastMCP

这个 MCP 服务是 iCloud Distribution 的远程 Agent 控制入口，复用项目已有的 HTTP API。
线上通过 `https://icloud.riria.org/mcp` 提供 Streamable HTTP；每个请求使用用户自己的账号级 Bearer Token，不会读取或返回 IMAP 密码、App 密码或 Cookie。

## 安装

```powershell
python -m pip install -r mcp/requirements.txt
```

## 环境变量

本地项目 API 默认地址是 `http://127.0.0.1:6981`。需要设置：

```powershell
$env:HME_MCP_USERNAME = "liuyuquan"
$env:HME_MCP_PASSWORD = "你的项目登录密码"
```

如果要让 Agent 只访问一个账号，使用该账号生成的 Token：

```powershell
$env:HME_MCP_TOKEN = "mcp_..."
```

设置 `HME_MCP_TOKEN` 后，MCP 请求使用账号级 Bearer Token，不再使用项目登录账号；服务端会把所有请求限制到 Token 对应的账号。

也可以使用 `HME_SUPERADMIN_USERNAME` 和 `HME_SUPERADMIN_PASSWORD`，但建议给 MCP 单独使用一组环境变量。

## 运行

```powershell
python mcp/server.py
```

本地开发仍可使用项目根目录的 `.codex/config.toml` 和 STDIO。线上用户直接复制账号面板生成的 `.mcp.json`，其中包含远程 MCP 地址和账号 Token。

远程服务由 Docker Compose 启动，不需要用户安装 Python 或运行本地脚本。

```powershell
MCP_TRANSPORT=streamable-http python mcp/server.py
```

不要直接把 HTTP MCP 端口暴露到公网；远程访问应放在已有认证和反向代理之后。

## 当前工具

- `project_status`
- `list_accounts`
- `list_aliases`
- `get_organizer`
- `inbox_count`
- `list_inbox`
- `get_message`
- `list_shares`
- `create_share`
- `create_alias`
- `set_mail_read_method`
