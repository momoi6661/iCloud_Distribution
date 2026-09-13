import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, type FullMailMessage, type MailMessage, type MailReadMethod } from '../api/client'
import Icon from '../components/Icon'

const dateText = (date?: string) => date ? new Date(date).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }) : '—'

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
    try { await navigator.clipboard.writeText(code) } catch { return }
  }

  if (pageError) return <main className="public-share"><div className="share-error"><div className="form-icon"><Icon name="alert" /></div><span className="eyebrow">共享收件箱</span><h1>链接不可用</h1><p>{pageError}</p><button className="button secondary" onClick={load}>重新加载</button></div></main>

  return <main className="public-share">
    <header className="public-header"><span className="auth-brand">iCloud 邮箱共享</span><span className="status status-ready">只读访问</span></header>
    <section className="public-card panel">
      {selected ? <>
        <div className="public-card-header public-message-header">
          <div><button className="back-link public-message-back" onClick={closeMessage}><Icon name="back" size={16} />邮件列表</button><span className="eyebrow">邮件正文</span><h1>{selected.subject || '（无主题）'}</h1><p className="public-identity">{selected.from} · {dateText(selected.date)}</p></div>
          {selected.code && <button className="button secondary small" onClick={() => void copyCode(selected.code || '')}><Icon name="copy" size={15} />复制验证码 {selected.code}</button>}
        </div>
        <div className="public-message-view" aria-live="polite">
          {messageLoading ? <div className="inbox-reader-state" role="status"><span className="message-loading-bar" /><strong>正在读取正文</strong><small>邮件列表已经保留，返回后可继续查看。</small></div> : messageError ? <div className="inbox-reader-state error-state" role="alert"><strong>正文读取失败</strong><small>{messageError}</small><button className="button secondary" onClick={() => openMessage(selected)}>重新读取</button></div> : <div className="public-message-body">{method === 'web_api' ? selected.preview || '这封邮件没有可显示的摘要。' : selected.body || '这封邮件没有可显示的正文。'}</div>}
        </div>
      </> : <>
        <div className="public-card-header"><div><span className="eyebrow">共享收件箱</span><h1>{alias || '共享邮箱'}</h1><p className="public-identity">此地址仅用于接收邮件，内容不会被修改或转发。</p></div><button className="button secondary small" onClick={load} disabled={loading}><Icon name="grid" size={15} />{loading ? '读取中…' : '刷新'}</button></div>
        {method === 'web_api' && <div className="inline-banner">当前链接通过 Web API 提供邮件摘要，完整正文需要 IMAP。</div>}
        {loading && messages.length === 0 ? <div className="public-mail-loading" role="status"><strong>正在读取邮件列表</strong><span>正在获取标题、发件人和简短正文摘要。</span></div> : messages.length === 0 ? <div className="empty-state small-empty"><h3>还没有邮件。</h3><p>刷新收件箱后，新邮件会显示在这里。</p></div> : <div className="mail-list public-mail-list">{messages.map((item) => <button className="mail-row mail-row-button public-mail-row" key={`${item.folder}-${item.id}`} onClick={() => openMessage(item)}><div className="mail-avatar">{(item.from || '?').slice(0, 1).toUpperCase()}</div><div><strong>{item.subject || '（无主题）'}</strong><span>{item.from}</span>{item.preview && <small>{item.preview}</small>}{item.code && <small className="mail-code-hint">验证码：{item.code}</small>}</div><time>{dateText(item.date)}</time></button>)}</div>}
      </>}
      <p className="public-footnote">此链接仅限只读访问 · {expiresAt ? `有效至 ${dateText(expiresAt)}` : '永久有效'}</p>
    </section>
  </main>
}
