import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, type FullMailMessage, type MailMessage, type MailReadMethod } from '../api/client'
import Icon from '../components/Icon'
import { getInitialTheme, persistTheme, type ThemeMode } from '../theme'

const dateText = (date?: string) => date ? new Date(date).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }) : '—'
const urlPattern = /(https?:\/\/[^\s<]+)/g
function MailBody({ text, html = false }: { text: string; html?: boolean }) {
  if (html && text.trim()) return <div className="mail-body-html" dangerouslySetInnerHTML={{ __html: text }} />
  return <>{text.split(urlPattern).map((part, index) => {
    if (!/^https?:\/\//i.test(part)) return <span key={index}>{part}</span>
    const match = part.match(/^(.*?)([),.;!?，。；！）]*)$/)
    const url = match?.[1] || part
    return <span key={index}><a href={url} target="_blank" rel="noreferrer">{url}</a>{match?.[2] || ''}</span>
  })}</>
}

function InlinePublicMailRow({ item, selected, loading, error, method, onOpen, onClose, onRetry, onCopyCode }: { item: MailMessage; selected: FullMailMessage | null; loading: boolean; error: string; method: MailReadMethod; onOpen: () => void; onClose: () => void; onRetry: () => void; onCopyCode: (code: string) => void }) {
  const expanded = selected?.id === item.id && (!selected.folder || selected.folder === item.folder)
  return <div className={`mail-item ${expanded ? 'expanded' : ''}`}><button className={`mail-row mail-row-button public-mail-row ${expanded ? 'selected' : ''}`} onClick={onOpen} aria-expanded={expanded}><div className="mail-avatar">{(item.from || '?').slice(0, 1).toUpperCase()}</div><div><strong>{item.subject || '（无主题）'}</strong><span>{item.from}</span>{item.preview && <small>{item.preview}</small>}{item.code && <span role="button" tabIndex={0} className="mail-code-button" onClick={(event) => { event.stopPropagation(); onCopyCode(item.code || '') }} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); event.stopPropagation(); onCopyCode(item.code || '') } }} aria-label={`复制验证码 ${item.code}`}><span>验证码：{item.code}</span><Icon name="copy" size={16} /></span>}</div><time>{dateText(item.date)}</time></button>{expanded && <div className="inline-mail-detail public-inline-mail-detail" aria-live="polite"><div className="inline-mail-detail-head"><div><span className="eyebrow">邮件正文</span><h2>{selected.subject || '（无主题）'}</h2><p>发件人：{selected.from} · 收件人：{selected.to || '未知'} · {dateText(selected.date)}</p></div><div className="inline-actions"><button className="button small secondary" onClick={onClose}>收起</button>{selected.code && <button className="button small secondary" onClick={() => onCopyCode(selected.code || '')}><Icon name="copy" size={15} />复制验证码</button>}</div></div>{loading ? <div className="inbox-reader-state" role="status"><span className="message-loading-bar" /><strong>正在读取正文</strong></div> : error ? <div className="inbox-reader-state error-state" role="alert"><strong>正文读取失败</strong><small>{error}</small><button className="button secondary" onClick={onRetry}>重新读取</button></div> : method === 'web_api' ? <div className="inline-mail-body"><MailBody text={selected.preview || '这封邮件没有可显示的摘要。'} /></div> : <div className="inline-mail-body"><MailBody text={selected.body || '这封邮件没有可显示的正文。'} html={selected.content_type === 'text/html'} /></div>}</div>}</div>
}

export default function SharePage() {
  const { token = '' } = useParams()
  const [alias, setAlias] = useState('')
  const [messages, setMessages] = useState<MailMessage[]>([])
  const [method, setMethod] = useState<MailReadMethod>('web_api')
  const [expiresAt, setExpiresAt] = useState('')
  const [selected, setSelected] = useState<FullMailMessage | null>(null)
  const [pageError, setPageError] = useState('')
  const [messageError, setMessageError] = useState('')
  const [loading, setLoading] = useState(true)
  const [messageLoading, setMessageLoading] = useState(false)
  const [copyNotice, setCopyNotice] = useState('')
  const [theme, setTheme] = useState<ThemeMode>(() => getInitialTheme())
  const messageRequest = useRef(0)

  const load = async () => {
    messageRequest.current += 1
    setLoading(true)
    setPageError('')
    setMessageError('')
    setSelected(null)
    void api.publicShareInfo(token).then((info) => { setAlias(info.alias); setExpiresAt(info.expires_at || '') }).catch(() => {})
    try {
      const inbox = await api.publicShareInbox(token)
      setAlias(inbox.alias)
      setMessages(inbox.messages || [])
      setMethod(inbox.method)
    } catch (error) {
      setPageError((error as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [token])

  const openMessage = async (item: MailMessage) => {
    const request = ++messageRequest.current
    setSelected({ ...item, body: '', content_type: '' })
    setMessageError('')
    if (method === 'web_api') return
    setMessageLoading(true)
    try {
      const full = await api.publicShareMessage(token, item.id, item.folder, method)
      if (request === messageRequest.current) setSelected(full)
    } catch (error) {
      if (request === messageRequest.current) setMessageError((error as Error).message)
    } finally {
      if (request === messageRequest.current) setMessageLoading(false)
    }
  }

  const closeMessage = () => {
    messageRequest.current += 1
    setSelected(null)
    setMessageError('')
    setMessageLoading(false)
  }

  const copyCode = async (code: string) => {
    try { await navigator.clipboard.writeText(code); setCopyNotice('验证码已复制') } catch { setCopyNotice('复制失败，请手动复制') }
  }

  const copyAlias = async () => {
    try { await navigator.clipboard.writeText(alias); setCopyNotice('邮箱地址已复制') } catch { setCopyNotice('复制失败，请手动复制') }
  }

  if (pageError) return <main className="public-share"><div className="share-error"><div className="form-icon"><Icon name="alert" /></div><span className="eyebrow">共享收件箱</span><h1>链接不可用</h1><p>{pageError}</p><button className="button secondary" onClick={load}>重新加载</button></div></main>

  return <main className="public-share">
    {copyNotice && <div className="notice toast" role="status" aria-live="polite"><span>{copyNotice}</span><button className="text-button" onClick={() => setCopyNotice('')}>关闭</button></div>}
    <header className="public-header"><span className="auth-brand">iCloud 邮箱共享</span><div className="public-header-actions"><span className="status status-ready">只读访问</span><button className="button secondary public-theme-toggle" type="button" onClick={() => { const next = theme === 'dark' ? 'light' : 'dark'; setTheme(next); persistTheme(next) }} aria-label={`切换到${theme === 'dark' ? '浅色' : '深色'}`}>切换到{theme === 'dark' ? '浅色' : '深色'}</button></div></header>
    <section className="public-card panel">
      <>
        <div className="public-card-header"><div className="public-share-heading"><span className="eyebrow">共享收件箱</span><div className="public-alias-line"><h1>{alias || '共享邮箱'}</h1>{alias && <button className="icon-button copy-alias-button" aria-label="复制邮箱地址" title="复制邮箱地址" onClick={() => void copyAlias()}><Icon name="copy" size={17} /></button>}</div><p className="public-identity">此地址仅用于接收邮件，内容不会被修改或转发。</p></div><button className="icon-button share-refresh-button" aria-label={loading ? '正在刷新邮件' : '刷新邮件列表'} title={loading ? '正在刷新邮件' : '刷新邮件列表'} onClick={() => void load()} disabled={loading}><Icon name="refresh" size={18} /></button></div>
        {method === 'web_api' && <div className="inline-banner">当前链接通过 Web API 提供邮件摘要，完整正文需要 IMAP。</div>}
        {loading && messages.length === 0 ? <div className="public-mail-loading" role="status"><strong>正在读取邮件列表</strong><span>正在获取标题、发件人和简短正文摘要。</span></div> : messages.length === 0 ? <div className="empty-state small-empty"><h3>还没有邮件。</h3><p>刷新收件箱后，新邮件会显示在这里。</p></div> : <div className="mail-list public-mail-list">{messages.map((item) => <InlinePublicMailRow item={item} selected={selected} loading={messageLoading} error={messageError} method={method} onOpen={() => void openMessage(item)} onClose={closeMessage} onRetry={() => void openMessage(item)} onCopyCode={(code) => void copyCode(code)} key={`${item.folder}-${item.id}`} />)}</div>}
      </>
      <p className="public-footnote">此链接仅限只读访问 · {expiresAt ? `有效至 ${dateText(expiresAt)}` : '永久有效'}</p>
    </section>
  </main>
}
