import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import Icon from '../components/Icon'

export default function LoginPage({ onSuccess }: { onSuccess: (mustChange?: boolean) => void }) {
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
      const result = await api.uiLogin(values.username, values.password)
      onSuccess(result.user?.must_change_password)
      navigate(result.user?.must_change_password ? '/profile' : '/')
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
          <p className="form-intro">使用系统账号登录。普通账号由超级管理员创建。</p>
          <form onSubmit={submit}>
            <label className="field"><span>用户名</span><input name="username" autoComplete="username" placeholder="输入用户名" required autoFocus /></label>
            <label className="field"><span>密码</span><input name="password" type="password" autoComplete="current-password" placeholder="输入密码" required /></label>
            {error && <div className="form-error" role="alert"><Icon name="alert" size={16} />{error}</div>}
            <button className="button primary full-width" type="submit" disabled={busy}>{busy ? '连接中…' : '登录'}<Icon name="arrow" size={17} /></button>
          </form>
          <p className="auth-footnote">系统不开放注册。如需账号，请联系超级管理员。</p>
        </div>
      </section>
    </main>
  )
}
