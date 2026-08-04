// API 客户端封装: 统一 {success, data, message} 响应格式与 401 处理。

export interface ApiResp<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const resp = await fetch(url, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })
  const json = (await resp.json()) as ApiResp<T>
  if (!resp.ok || !json.success) {
    // 只有 UI 鉴权中间件返回的 401 (带标记) 才是"UI 会话过期" → 跳转登录页。
    // 业务接口的 401 (如 iCloud 登录失败、Cookie 失效) 只是普通错误,弹错误信息即可。
    const reason = (json.data as { reason?: string } | undefined)?.reason
    if (resp.status === 401 && reason === 'ui_auth_expired') {
      window.dispatchEvent(new Event('hme:unauthorized'))
    }
    throw new ApiError(resp.status, json.message || `请求失败: HTTP ${resp.status}`)
  }
  return json.data as T
}

// ---- 类型定义 ----

export interface Account {
  id: string
  name: string
  real_email: string
  icloud_email: string
  host: string
  proxy?: string
  status: string // active / pending / error
  alias_total: number
  alias_active: number
  last_validated: string
  last_error?: string
  created_at: string
}

export interface Alias {
  email: string
  anonymousId: string
  label: string
  active: boolean
  createdAt?: string
  forwardTo?: string
}

export interface MailMessage {
  id: string
  from: string
  to: string
  subject: string
  date: string
  preview: string
  folder?: string
}

export interface InboxData {
  account_id: string
  alias: string
  count: number
  method: 'imap' | 'web_api'
  messages: MailMessage[]
}

export interface FullMailMessage extends MailMessage {
  body: string
  content_type: string
}

export interface LoginStartResult {
  status: 'done' | 'otp_required'
  session_id?: string
  cookies_count?: number
}

export interface BatchResult {
  index: number
  success: boolean
  email?: string
  label: string
  error?: string
}

export interface BatchCreateResult {
  requested: number
  succeeded: number
  failed: number
  interrupted: boolean
  results: BatchResult[]
}

export interface UIStatus {
  auth_required: boolean
  initialized: boolean
  token_mode: boolean
  authenticated: boolean
}

// ---- API 方法 ----

export const api = {
  // UI 鉴权
  uiStatus: () => request<UIStatus>('GET', '/api/ui/status'),
  uiLogin: (req: { username?: string; password?: string; token?: string }) =>
    request('POST', '/api/ui/login', req),
  uiSetup: (username: string, password: string) =>
    request('POST', '/api/ui/setup', { username, password }),
  uiLogout: () => request('POST', '/api/ui/logout'),

  // 账号
  listAccounts: () => request<Account[]>('GET', '/api/accounts'),
  addAccount: (req: { name: string; email?: string; cookies?: string; host?: string; proxy?: string }) =>
    request<Account>('POST', '/api/accounts', req),
  removeAccount: (id: string) => request('DELETE', `/api/accounts/${id}`),
  setAppPassword: (id: string, icloudEmail: string, appPassword: string) =>
    request('POST', `/api/accounts/${id}/password`, { icloud_email: icloudEmail, app_password: appPassword }),
  updateCookies: (id: string, cookies: Record<string, string>) =>
    request('PUT', `/api/accounts/${id}/cookies`, { cookies }),

  // 两段式自动授权
  loginStart: (id: string, password: string) =>
    request<LoginStartResult>('POST', `/api/accounts/${id}/login/start`, { password }),
  loginOTP: (id: string, sessionId: string, code: string, method: 'device' | 'sms' = 'device', phoneId?: number) =>
    request<LoginStartResult>('POST', `/api/accounts/${id}/login/otp`, {
      session_id: sessionId,
      code,
      method,
      phone_id: phoneId,
    }),
  loginPhones: (id: string, sessionId: string) =>
    request<{ phones: { id: number; numberWithDialCode: string }[] }>(
      'GET',
      `/api/accounts/${id}/login/phones?session_id=${sessionId}`,
    ),
  loginSMS: (id: string, sessionId: string, phoneId: number) =>
    request('POST', `/api/accounts/${id}/login/sms`, { session_id: sessionId, phone_id: phoneId }),
  loginResend: (id: string, sessionId: string) =>
    request('POST', `/api/accounts/${id}/login/resend`, { session_id: sessionId }),

  // 别名
  listAliases: (accountId: string) =>
    request<{ count: number; aliases: Alias[] }>('GET', `/api/aliases?account_id=${accountId}`),
  createAlias: (accountId: string, label: string) =>
    request<{ email: string; label: string }>('POST', '/api/create', { account_id: accountId, label }),
  batchCreate: (accountId: string, count: number, labelPrefix: string) =>
    request<BatchCreateResult>('POST', `/api/accounts/${accountId}/aliases/batch`, {
      count,
      label_prefix: labelPrefix,
    }),
  deactivateAlias: (accountId: string, anonymousId: string) =>
    request('POST', `/api/aliases/${anonymousId}/deactivate`, { account_id: accountId }),
  reactivateAlias: (accountId: string, anonymousId: string) =>
    request('POST', `/api/aliases/${anonymousId}/reactivate`, { account_id: accountId }),
  deleteAlias: (accountId: string, anonymousId: string) =>
    request('DELETE', `/api/aliases/${anonymousId}`, { account_id: accountId }),
  setForwardTo: (accountId: string, email: string) =>
    request('POST', `/api/accounts/${accountId}/forward-to`, { email }),

  // 分享链接 (管理端)
  createShare: (accountId: string, alias: string, label: string) =>
    request<{ token: string; url: string; alias: string; created_at: string }>(
      'POST',
      '/api/aliases/share',
      { account_id: accountId, alias, label },
    ),
  listShares: (accountId: string) =>
    request<
      { token: string; account_id: string; alias: string; label?: string; created_at: string }[]
    >('GET', `/api/shares?account_id=${accountId}`),
  deleteShare: (token: string) => request('DELETE', `/api/shares/${token}`),

  // 分享链接 (公开端,免登录)
  publicShareInfo: (token: string) =>
    request<{ alias: string; label?: string; created_at: string }>('GET', `/api/public/share/${token}`),
  publicShareInbox: (token: string, limit = 30, days = 7) =>
    request<{ alias: string; count: number; method: 'imap' | 'web_api'; messages: MailMessage[] }>(
      'GET',
      `/api/public/share/${token}/inbox?limit=${limit}&days=${days}`,
    ),
  publicShareMessage: (token: string, uid: string, folder?: string) =>
    request<FullMailMessage>(
      'GET',
      `/api/public/share/${token}/message?uid=${uid}${folder ? `&folder=${folder}` : ''}`,
    ),

  // 邮件
  inbox: (accountId: string, alias: string, limit = 20, days = 7) =>
    request<InboxData>(
      'GET',
      `/api/inbox?account_id=${accountId}&alias=${encodeURIComponent(alias)}&limit=${limit}&days=${days}`,
    ),
  getMessage: (accountId: string, uid: string, folder?: string) =>
    request<FullMailMessage>(
      'GET',
      `/api/inbox/message?account_id=${accountId}&uid=${uid}${folder ? `&folder=${folder}` : ''}`,
    ),
}
