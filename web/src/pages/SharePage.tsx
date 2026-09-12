import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, type FullMailMessage, type MailMessage } from '../api/client'
import Icon from '../components/Icon'

const dateText = (date?: string) => date ? new Date(date).toLocaleString('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }) : '—'

export default function SharePage() {
  const { token = '' } = useParams()
  const [alias, setAlias] = useState('')
  const [messages, setMessages] = useState<MailMessage[]>([])
  const [method, setMethod] = useState<'imap' | 'web_api'>('web_api')
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
    void api.publicShareInfo(token).then((info) => setAlias(info.alias)).catch(() => {})
    try {
      const inbox = await api.publicShareInbox(token)
      setAlias(inbox.alias)
      setMessages(inbox.messages || [])
      setMethod(inbox.method)
    } catch (e) {
      setPageError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { void load() }, [token])

  const openMessage = async (item: MailMessage) => {
    const request = ++messageRequest.current
    setSelected({ ...item, body: '', content_type: '' })
    setMessageError('')
    if (method !== 'imap') return
    setMessageLoading(true)
    try {
      const full = await api.publicShareMessage(token, item.id, item.folder)
      if (request === messageRequest.current) setSelected(full)
    } catch (e) {
      if (request === messageRequest.current) setMessageError((e as Error).message)
    } finally {
      if (request === messageRequest.current) setMessageLoading(false)
    }
  }

  if (pageError) return <main className="public-share"><div className="share-error"><div className="form-icon"><Icon name="alert" /></div><span className="eyebrow">共享收件箱</span><h1>链接不可用</h1><p>{pageError}</p><button className="button secondary" onClick={load}>重新加载</button></div></main>

  return <main className="public-share">
    <header className="public-header"><span className="auth-brand">iCloud 邮箱共享</span><span className="status status-ready">只读访问</span></header>
    <section className="public-card panel">
      <div className="public-card-header"><div><span className="eyebrow">共享收件箱</span><h1>{alias || '共享邮箱'}</h1><p className="public-identity">此地址仅用于接收邮件，内容不会被修改或转发。</p></div><button className="button secondary small" onClick={load} disabled={loading}><Icon name="grid" size={15} />{loading ? '读取中…' : '刷新'}</button></div>
      {method !== 'imap' && <div className="inline-banner">当前链接只提供邮件摘要。此邮箱暂不支持读取完整正文。</div>}
      {loading && messages.length === 0 ? <div className="public-mail-loading" role="status"><strong>正在读取邮件</strong><span>页面已打开，邮件会直接显示在这里。</span></div> : messages.length === 0 ? <div className="empty-state small-empty"><h3>还没有邮件。</h3><p>刷新收件箱后，新邮件会显示在这里。</p></div> : <div className="inbox-split public-inbox-split"><div className="mail-list inbox-mail-list">{messages.map((item) => <button className={`mail-row mail-row-button public-mail-row ${selected?.id === item.id && selected?.folder === item.folder ? 'selected' : ''}`} key={`${item.folder}-${item.id}`} onClick={() => openMessage(item)} aria-pressed={selected?.id === item.id && selected?.folder === item.folder}><div className="mail-avatar">{(item.from || '?').slice(0, 1).toUpperCase()}</div><div><strong>{item.subject || '（无主题）'}</strong><span>{item.from}</span><small>{item.preview || '点击查看完整内容'}</small></div><time>{dateText(item.date)}</time></button>)}</div><aside className="inbox-reader" aria-live="polite">{!selected ? <div className="inbox-reader-empty"><Icon name="mail" size={22} /><strong>选择一封邮件</strong><span>点击后才会读取正文，并直接显示在这里。</span></div> : <><header className="inbox-reader-header"><span className="eyebrow">邮件正文</span><h3>{selected.subject || '（无主题）'}</h3><p>{selected.from}<br />{dateText(selected.date)}</p></header>{messageLoading ? <div className="inbox-reader-state" role="status"><span className="message-loading-bar" /><strong>正在读取正文</strong><small>可以直接选择其他邮件。</small></div> : messageError ? <div className="inbox-reader-state error-state" role="alert"><strong>正文读取失败</strong><small>{messageError}</small><button className="button secondary" onClick={() => openMessage(selected)}>重新读取</button></div> : method === 'web_api' ? <div className="inbox-reader-body">{selected.preview || '这封邮件没有可显示的摘要。'}</div> : <div className="inbox-reader-body">{selected.body || '这封邮件没有可显示的正文。'}</div>}</>}</aside></div>}
      <p className="public-footnote">此链接仅限只读访问</p>
    </section>
  </main>
}
