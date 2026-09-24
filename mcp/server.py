"""Project-level FastMCP control server for iCloud Distribution.

The MCP server is a thin authenticated client of the existing application API.
It does not access accounts.json, IMAP credentials, or cookies directly.
"""

from __future__ import annotations

import os
from typing import Any
from urllib.parse import quote

import httpx
from fastmcp import FastMCP


BASE_URL = os.getenv("ICLOUD_DISTRIBUTION_URL", "http://127.0.0.1:6981").rstrip("/")
USERNAME = os.getenv("HME_MCP_USERNAME") or os.getenv("HME_SUPERADMIN_USERNAME", "")
PASSWORD = os.getenv("HME_MCP_PASSWORD") or os.getenv("HME_SUPERADMIN_PASSWORD", "")


class ProjectAPI:
    def __init__(self) -> None:
        self.client = httpx.Client(base_url=BASE_URL, timeout=30.0)
        self.authenticated = False

    def _login(self) -> None:
        if self.authenticated:
            return
        if not USERNAME or not PASSWORD:
            raise RuntimeError(
                "缺少 HME_MCP_USERNAME/HME_MCP_PASSWORD，或 HME_SUPERADMIN_USERNAME/HME_SUPERADMIN_PASSWORD"
            )
        response = self.client.post(
            "/api/ui/login", json={"username": USERNAME, "password": PASSWORD}
        )
        self._unwrap(response)
        self.authenticated = True

    def _request(self, method: str, path: str, **kwargs: Any) -> Any:
        self._login()
        response = self.client.request(method, path, **kwargs)
        try:
            reason = response.json().get("data", {}).get("reason")
        except (ValueError, AttributeError):
            reason = None
        if response.status_code == 401 and reason == "ui_auth_expired":
            self.authenticated = False
            self._login()
            response = self.client.request(method, path, **kwargs)
        return self._unwrap(response)

    @staticmethod
    def _unwrap(response: httpx.Response) -> Any:
        try:
            payload = response.json()
        except ValueError as exc:
            raise RuntimeError(f"项目 API 返回了非 JSON 响应（HTTP {response.status_code}）") from exc
        if response.status_code >= 400 or not payload.get("success", False):
            raise RuntimeError(payload.get("message") or f"项目 API 请求失败（HTTP {response.status_code}）")
        return payload.get("data")


api = ProjectAPI()
mcp = FastMCP("iCloud Distribution")


def _account_path(account_id: str) -> str:
    if not account_id.strip():
        raise ValueError("account_id 不能为空")
    return f"/api/accounts/{quote(account_id.strip(), safe='')}"


@mcp.tool
def project_status() -> dict[str, Any]:
    """检查项目 API 和当前 MCP 登录状态。"""
    return api._request("GET", "/api/ui/status")


@mcp.tool
def list_accounts(include_disabled: bool = False) -> Any:
    """列出当前用户可访问的 iCloud 账号。"""
    path = "/api/accounts/disabled" if include_disabled else "/api/accounts"
    return api._request("GET", path)


@mcp.tool
def list_aliases(account_id: str) -> Any:
    """列出指定账号的隐藏邮箱别名和状态。"""
    return api._request("GET", "/api/aliases", params={"account_id": account_id})


@mcp.tool
def get_organizer(account_id: str) -> Any:
    """查看账号的项目内分组和别名备注。"""
    return api._request("GET", _account_path(account_id) + "/organizer")


@mcp.tool
def inbox_count(account_id: str, alias: str = "", days: int = 7) -> Any:
    """只查询邮件数量；不加载邮件正文。"""
    return api._request(
        "GET",
        "/api/inbox/count",
        params={"account_id": account_id, "alias": alias, "days": days},
    )


@mcp.tool
def list_inbox(
    account_id: str,
    alias: str = "",
    days: int = 7,
    page: int = 1,
    method: str = "auto",
) -> Any:
    """读取收件箱列表和摘要。默认只查近 7 天；alias 为空表示全部邮件。"""
    return api._request(
        "GET",
        "/api/inbox",
        params={
            "account_id": account_id,
            "alias": alias,
            "limit": 20,
            "days": days,
            "page": page,
            "method": method,
        },
    )


@mcp.tool
def get_message(
    account_id: str,
    uid: str,
    folder: str = "",
    source: str = "auto",
    alias: str = "",
) -> Any:
    """读取一封邮件正文。uid、folder、source 使用 list_inbox 返回值。"""
    return api._request(
        "GET",
        "/api/inbox/message",
        params={
            "account_id": account_id,
            "uid": uid,
            "folder": folder,
            "source": source,
            "alias": alias,
        },
    )


@mcp.tool
def list_shares(account_id: str) -> Any:
    """列出指定账号的分享链接及有效期状态。"""
    return api._request("GET", "/api/shares", params={"account_id": account_id})


@mcp.tool
def create_share(
    account_id: str,
    alias: str,
    label: str = "",
    expires_minutes: int = 0,
) -> Any:
    """为别名创建只读分享链接。expires_minutes=0 表示永久。"""
    if expires_minutes < 0:
        raise ValueError("expires_minutes 不能为负数")
    return api._request(
        "POST",
        "/api/aliases/share",
        json={
            "account_id": account_id,
            "alias": alias,
            "label": label,
            "expires_minutes": expires_minutes,
        },
    )


@mcp.tool
def create_alias(
    account_id: str,
    label: str = "",
    group_id: str = "",
    note: str = "",
) -> Any:
    """在指定 iCloud 账号创建一个隐藏邮箱，并可保存本地分组和备注。"""
    return api._request(
        "POST",
        "/api/create",
        json={
            "account_id": account_id,
            "label": label,
            "group_id": group_id,
            "note": note,
        },
    )


@mcp.tool
def set_mail_read_method(account_id: str, method: str) -> Any:
    """设置账号默认邮件读取方式：web_api、imap 或 forward_imap。"""
    if method not in {"web_api", "imap", "forward_imap"}:
        raise ValueError("method 必须是 web_api、imap 或 forward_imap")
    return api._request("PUT", _account_path(account_id) + "/mail-read-method", json={"method": method})


if __name__ == "__main__":
    # STDIO is the project default. For a remote deployment, use:
    # fastmcp run mcp/server.py --transport http --host 127.0.0.1 --port 8787
    mcp.run()
