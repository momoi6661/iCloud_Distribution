// Thin client for the existing JSON envelope: { success, data, message }.

export interface ApiResp<T = unknown> {
  success: boolean
  message?: string
  data?: T
}
export interface UIIdentity { id: string; username: string; role: 'superadmin' | 'user'; must_change_password?: boolean }
export interface AppUser extends UIIdentity { status: 'active' | 'disabled'; must_change_password: boolean; created_at: string; last_login_at?: string }

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
  has_cookies?: boolean
  has_app_password?: boolean
	forward_imap?: { host: string; port: number; email: string; mailboxes?: string[] }
	has_forward_imap?: boolean
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
  code?: string
  folder?: string
  alias?: string
}

export type MailReadMethod = 'imap' | 'web_api' | 'forward_imap'
export type MailReadPreference = 'auto' | MailReadMethod

export interface InboxData {
  account_id: string
  alias: string
  count: number
  method: MailReadMethod
  messages: MailMessage[]
  page?: number
  page_size?: number
  has_more?: boolean
}

export interface FullMailMessage extends MailMessage {
  body: string
  content_type: string
}

export interface BatchMessageDeleteResult {
	requested: number
	deleted: number
}

export interface LoginStartResult {
  status: 'done' | 'otp_required'
  session_id?: string
  cookies_count?: number
  method?: 'device' | 'sms'
  sms_sent?: boolean
  phones?: { id: number; numberWithDialCode: string }[]
  warning?: string
}

export interface BatchResult {
  index: number
  success: boolean
  email?: string
  label: string
  error?: string
  anonymous_id?: string
  metadata_error?: string
}

export interface BatchCreateResult {
  requested: number
  succeeded: number
  failed: number
  interrupted: boolean
  metadata_failed?: number
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
  authenticated: boolean
  user?: UIIdentity
}

export interface ShareLink {
  token: string
  account_id: string
  alias: string
  label?: string
  created_at: string
	expires_at?: string
}

export interface BatchShareResult {
  requested: number
  deleted: number
  not_found: number
}

export interface BatchAliasResult {
  requested: number
  deleted: number
  failed: number
  results: { id: string; success: boolean; error?: string }[]
}

export interface OrganizerGroup {
  id: string
  name: string
  created_at: string
}

export interface AliasMetadata {
  alias_id: string
  email: string
  label: string
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
  uiLogin: (username: string, password: string) => request<{ auth_required: boolean; user: UIIdentity }>('POST', '/api/ui/login', { username, password }),
  listUsers: () => request<{ users: AppUser[] }>('GET', '/api/admin/users'),
  createUser: (username: string, password: string, mustChange = true) => request<AppUser>('POST', '/api/admin/users', { username, password, must_change_password: mustChange }),
  resetUserPassword: (id: string, password: string, mustChange = true) => request('PUT', `/api/admin/users/${id}/password`, { password, must_change_password: mustChange }),
  setUserStatus: (id: string, status: 'active' | 'disabled') => request('PUT', `/api/admin/users/${id}/status`, { status }),
  deleteUser: (id: string) => request('DELETE', `/api/admin/users/${id}`),
  changeOwnPassword: (currentPassword: string, newPassword: string) => request('POST', '/api/ui/password', { current_password: currentPassword, new_password: newPassword }),
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
  updateAliasMeta: (accountId: string, meta: { alias_id: string; email: string; label: string; group_id: string; note: string }) => request<AliasMetadata>('PUT', `/api/accounts/${accountId}/alias-meta`, meta),
  createAlias: (accountId: string, label: string, groupId = '') => request<{ email: string; label: string; anonymous_id?: string }>('POST', '/api/create', { account_id: accountId, label, group_id: groupId }),
  batchCreate: (accountId: string, options: { count: number; label_prefix: string; naming_rule: 'sequence' | 'same'; separator: string; start_number: number; padding: number; group_id: string; note: string }) => request<BatchCreateResult>('POST', `/api/accounts/${accountId}/aliases/batch`, options),
  deactivateAlias: (accountId: string, anonymousId: string) => request('POST', `/api/aliases/${anonymousId}/deactivate`, { account_id: accountId }),
  reactivateAlias: (accountId: string, anonymousId: string) => request('POST', `/api/aliases/${anonymousId}/reactivate`, { account_id: accountId }),
  deleteAlias: (accountId: string, anonymousId: string) => request('DELETE', `/api/aliases/${anonymousId}`, { account_id: accountId }),
  batchDeleteAliases: (accountId: string, ids: string[]) => request<BatchAliasResult>('POST', '/api/aliases/batch/delete', { account_id: accountId, ids }),
  setForwardTo: (accountId: string, email: string) => request('POST', `/api/accounts/${accountId}/forward-to`, { email }),

  createShare: (accountId: string, alias: string, label: string, expiresMinutes: number) => request<{ token: string; url: string; alias: string; created_at: string; expires_at?: string }>('POST', '/api/aliases/share', { account_id: accountId, alias, label, expires_minutes: expiresMinutes }),
  listShares: (accountId: string) => request<ShareLink[]>('GET', `/api/shares?account_id=${encodeURIComponent(accountId)}`),
  updateShare: (token: string, label: string) => request<ShareLink>('PUT', `/api/shares/${token}`, { label }),
  deleteShare: (token: string) => request('DELETE', `/api/shares/${token}`),
  batchDeleteShares: (tokens: string[]) => request<BatchShareResult>('POST', '/api/shares/batch/delete', { tokens }),

  publicShareInfo: (token: string) => request<{ alias: string; label?: string; created_at: string; expires_at?: string }>('GET', `/api/public/share/${token}`),
  publicShareInbox: (token: string, limit = 30, days = 7, page = 1) => request<{ alias: string; count: number; method: MailReadMethod; messages: MailMessage[]; page?: number; page_size?: number; has_more?: boolean }>('GET', `/api/public/share/${token}/inbox?limit=${limit}&days=${days}&page=${page}&refresh=${Date.now()}`),
  publicShareMessage: (token: string, uid: string, folder: string | undefined, source: InboxData['method']) => request<FullMailMessage>('GET', `/api/public/share/${token}/message?uid=${uid}${folder ? `&folder=${encodeURIComponent(folder)}` : ''}&source=${source}`),

  inbox: (accountId: string, alias: string, limit = 20, days = 7, method: MailReadPreference = 'auto', page = 1) => request<InboxData>('GET', `/api/inbox?account_id=${encodeURIComponent(accountId)}&alias=${encodeURIComponent(alias)}&limit=${limit}&days=${days}&page=${page}&method=${encodeURIComponent(method)}`),
  inboxCount: (accountId: string, alias: string, days = 7) => request<{ account_id: string; alias: string; count: number }>('GET', `/api/inbox/count?account_id=${encodeURIComponent(accountId)}&alias=${encodeURIComponent(alias)}&days=${days}`),
  getMessage: (accountId: string, uid: string, folder: string | undefined, source: InboxData['method'], alias = '') => request<FullMailMessage>('GET', `/api/inbox/message?account_id=${encodeURIComponent(accountId)}&uid=${uid}${folder ? `&folder=${encodeURIComponent(folder)}` : ''}&source=${source}&alias=${encodeURIComponent(alias)}`),
  deleteMessage: (accountId: string, uid: string, folder: string | undefined, source: InboxData['method'], alias = '') => request('DELETE', `/api/inbox/message?account_id=${encodeURIComponent(accountId)}&uid=${uid}${folder ? `&folder=${encodeURIComponent(folder)}` : ''}&source=${source}&alias=${encodeURIComponent(alias)}`),
	deleteMessages: (accountId: string, source: InboxData['method'], messages: Array<{ uid: string; folder?: string; alias?: string }>) => request<BatchMessageDeleteResult>('POST', '/api/inbox/messages/delete', { account_id: accountId, source, messages }),
	setForwardIMAP: (accountId: string, value: { host: string; port: number; email: string; password: string; mailboxes: string[] }) => request('POST', `/api/accounts/${accountId}/forward-imap`, value),
}
