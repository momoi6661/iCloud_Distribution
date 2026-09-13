import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import Icon from '../components/Icon'

export default function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    api.uiStatus().catch((e) => setError((e as Error).message))
  }, [])

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    const values = Object.fromEntries(new FormData(event.currentTarget).entries()) as Record<string, string>
    setBusy(true)
    try {
      await api.uiLogin(values.password)
      onSuccess(); navigate('/')
    } catch (e) { setError((e as Error).message) } finally { setBusy(false) }
  }

  return (
    <main className="auth-page">
      <section className="auth-panel" aria-labelledby="auth-title">
        <div className="auth-brand"><span>iCloud 邮箱管理</span><span className="auth-version">本地控制台</span></div>
        <div style={{ flex: 1, display: 'flex', alignItems: 'center' }}>
          <div className="auth-copy"><span className="eyebrow">私密邮箱运维</span><h1 id="auth-title">让每条转发<br /><em>保持在线。</em></h1><p>在一个清晰的操作台中管理 iCloud 隐藏邮箱、别名和收件箱。</p></div>
        </div>
      </section>
      <section className="auth-form-wrap">
        <div className="auth-form-card">
          <div className="auth-form-heading"><div className="form-icon"><Icon name="settings" /></div><div><span className="eyebrow">控制台登录</span><h2>欢迎回来</h2></div></div>
          <p className="form-intro">输入服务器环境变量中配置的访问密码。</p>
          <form onSubmit={submit}>
            <label className="field"><span>访问密码</span><input name="password" type="password" autoComplete="current-password" placeholder="输入访问密码" required autoFocus /></label>
            {error && <div className="form-error" role="alert"><Icon name="alert" size={16} />{error}</div>}
            <button className="button primary full-width" type="submit" disabled={busy}>{busy ? '连接中…' : '登录'}<Icon name="arrow" size={17} /></button>
          </form>
          <p className="auth-footnote">密码只用于当前服务验证，不会写入浏览器或项目数据。</p>
        </div>
      </section>
    </main>
  )
}
