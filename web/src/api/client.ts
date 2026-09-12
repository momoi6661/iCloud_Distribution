// Thin client for the existing JSON envelope: { success, data, message }.

export interface ApiResp<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message)
    this.name = 'ApiError'
  }
}

async function request<T>(method: string, url: string, body?: unknown): Promise<T> {
  const response = await fetch(url, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })

  const json = (await response.json().catch(() => ({}))) as ApiResp<T>
  if (!response.ok || !json.success) {
    const reason = (json.data as { reason?: string } | undefined)?.reason
    if (response.status === 401 && reason === 'ui_auth_expired') {
      window.dispatchEvent(new Event('hme:unauthorized'))
    }
    throw new ApiError(response.status, json.message || `Request failed: HTTP ${response.status}`)
  }
  return json.data as T
}

export interface Account {
  id: string
  name: string
  real_email: string
  icloud_email: string
  host: string
  proxy?: string
  status: string // active / pending / error / disabled
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

export interface BatchAccountResult {
  requested?: number
  changed?: number
  already_disabled?: number
  restored?: number
  deleted?: number
  not_found?: number
  failed?: number
  results?: { id: string; success: boolean; error?: string }[]
}

export interface UIStatus {
  auth_required: boolean
  initialized: boolean
  token_mode: boolean
  authenticated: boolean
}

export interface ShareLink {
  token: string
  account_id: string
  alias: string
  label?: string
  created_at: string
}

export interface OrganizerGroup {
  id: string
  name: string
  created_at: string
}

export interface AliasMetadata {
  alias_id: string
  email: string
  group_id: string
  note: string
  updated_at: string
}

export interface OrganizerData {
  groups: OrganizerGroup[]
  metadata: Record<string, AliasMetadata>
}

export const api = {
  uiStatus: () => request<UIStatus>('GET', '/api/ui/status'),
  uiLogin: (req: { username?: string; password?: string; token?: string }) => request('POST', '/api/ui/login', req),
  uiSetup: (username: string, password: string) => request('POST', '/api/ui/setup', { username, password }),
  uiLogout: () => request('POST', '/api/ui/logout'),

  listAccounts: () => request<Account[]>('GET', '/api/accounts'),
  addAccount: (req: { name: string; email?: string; cookies?: string; host?: string; proxy?: string }) => request<Account>('POST', '/api/accounts', req),
  removeAccount: (id: string) => request<{ id: string }>('DELETE', `/api/accounts/${id}`),

  // Assumed contract for the new operations surface. Isolated here for the backend rollout.
  listDisabledAccounts: () => request<Account[]>('GET', '/api/accounts/disabled'),
  batchDisableAccounts: (ids: string[]) => request<BatchAccountResult>('POST', '/api/accounts/batch/deactivate', { ids }),
  batchDeleteAccounts: (ids: string[]) => request<BatchAccountResult>('POST', '/api/accounts/batch/delete', { ids }),
  restoreAccount: (id: string) => request<{ id: string; status: string }>('POST', `/api/accounts/${id}/restore`),

  setAppPassword: (id: string, icloudEmail: string, appPassword: string) => request('POST', `/api/accounts/${id}/password`, { icloud_email: icloudEmail, app_password: appPassword }),
  updateCookies: (id: string, cookies: Record<string, string>) => request('PUT', `/api/accounts/${id}/cookies`, { cookies }),

  loginStart: (id: string, password: string) => request<LoginStartResult>('POST', `/api/accounts/${id}/login/start`, { password }),
  loginOTP: (id: string, sessionId: string, code: string, method: 'device' | 'sms' = 'device', phoneId?: number) => request<LoginStartResult>('POST', `/api/accounts/${id}/login/otp`, { session_id: sessionId, code, method, phone_id: phoneId }),
  loginPhones: (id: string, sessionId: string) => request<{ phones: { id: number; numberWithDialCode: string }[] }>('GET', `/api/accounts/${id}/login/phones?session_id=${sessionId}`),
  loginSMS: (id: string, sessionId: string, phoneId: number) => request('POST', `/api/accounts/${id}/login/sms`, { session_id: sessionId, phone_id: phoneId }),
  loginResend: (id: string, sessionId: string) => request('POST', `/api/accounts/${id}/login/resend`, { session_id: sessionId }),

  listAliases: (accountId: string) => request<{ count: number; aliases: Alias[] }>('GET', `/api/aliases?account_id=${encodeURIComponent(accountId)}`),
  getOrganizer: (accountId: string) => request<OrganizerData>('GET', `/api/accounts/${accountId}/organizer`),
  createOrganizerGroup: (accountId: string, name: string) => request<OrganizerGroup>('POST', `/api/accounts/${accountId}/groups`, { name }),
  renameOrganizerGroup: (accountId: string, groupId: string, name: string) => request<OrganizerGroup>('PUT', `/api/accounts/${accountId}/groups/${groupId}`, { name }),
  deleteOrganizerGroup: (accountId: string, groupId: string) => request<{ id: string }>('DELETE', `/api/accounts/${accountId}/groups/${groupId}`),
  updateAliasMeta: (accountId: string, meta: { alias_id: string; email: string; group_id: string; note: string }) => request<AliasMetadata>('PUT', `/api/accounts/${accountId}/alias-meta`, meta),
  createAlias: (accountId: string, label: string) => request<{ email: string; label: string }>('POST', '/api/create', { account_id: accountId, label }),
  batchCreate: (accountId: string, count: number, labelPrefix: string) => request<BatchCreateResult>('POST', `/api/accounts/${accountId}/aliases/batch`, { count, label_prefix: labelPrefix }),
  deactivateAlias: (accountId: string, anonymousId: string) => request('POST', `/api/aliases/${anonymousId}/deactivate`, { account_id: accountId }),
  reactivateAlias: (accountId: string, anonymousId: string) => request('POST', `/api/aliases/${anonymousId}/reactivate`, { account_id: accountId }),
  deleteAlias: (accountId: string, anonymousId: string) => request('DELETE', `/api/aliases/${anonymousId}`, { account_id: accountId }),
  setForwardTo: (accountId: string, email: string) => request('POST', `/api/accounts/${accountId}/forward-to`, { email }),

  createShare: (accountId: string, alias: string, label: string) => request<{ token: string; url: string; alias: string; created_at: string }>('POST', '/api/aliases/share', { account_id: accountId, alias, label }),
  listShares: (accountId: string) => request<ShareLink[]>('GET', `/api/shares?account_id=${encodeURIComponent(accountId)}`),
  deleteShare: (token: string) => request('DELETE', `/api/shares/${token}`),

  publicShareInfo: (token: string) => request<{ alias: string; label?: string; created_at: string }>('GET', `/api/public/share/${token}`),
  publicShareInbox: (token: string, limit = 30, days = 7) => request<{ alias: string; count: number; method: 'imap' | 'web_api'; messages: MailMessage[] }>('GET', `/api/public/share/${token}/inbox?limit=${limit}&days=${days}`),
  publicShareMessage: (token: string, uid: string, folder?: string) => request<FullMailMessage>('GET', `/api/public/share/${token}/message?uid=${uid}${folder ? `&folder=${folder}` : ''}`),

  inbox: (accountId: string, alias: string, limit = 20, days = 7) => request<InboxData>('GET', `/api/inbox?account_id=${encodeURIComponent(accountId)}&alias=${encodeURIComponent(alias)}&limit=${limit}&days=${days}`),
  getMessage: (accountId: string, uid: string, folder?: string) => request<FullMailMessage>('GET', `/api/inbox/message?account_id=${encodeURIComponent(accountId)}&uid=${uid}${folder ? `&folder=${folder}` : ''}`),
}
