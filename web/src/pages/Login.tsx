import { useEffect, useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type UIStatus } from '../api/client'
import Icon from '../components/Icon'

export default function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const [status, setStatus] = useState<UIStatus | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const navigate = useNavigate()

  useEffect(() => {
    api.uiStatus().then(setStatus).catch((e) => setError((e as Error).message))
  }, [])

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    const values = Object.fromEntries(new FormData(event.currentTarget).entries()) as Record<string, string>
    const setupMode = status !== null && !status.initialized
    if (setupMode && values.password !== values.confirm) { setError('两次输入的密码不一致。'); return }
    setBusy(true)
    try {
      if (setupMode) await api.uiSetup(values.username, values.password)
      else await api.uiLogin(status?.token_mode ? { token: values.token } : { username: values.username, password: values.password })
      onSuccess(); navigate('/')
    } catch (e) { setError((e as Error).message) } finally { setBusy(false) }
  }

  const isToken = status?.token_mode
  const setupMode = status !== null && !status.initialized
  return (
    <main className="auth-page">
      <section className="auth-panel" aria-labelledby="auth-title">
        <div className="auth-brand"><span>iCloud 邮箱管理</span><span className="auth-version">本地控制台</span></div>
        <div className="auth-copy"><span className="eyebrow">私密邮箱运维</span><h1 id="auth-title">让每条转发<br /><em>保持在线。</em></h1><p>在一个清晰的操作台中管理 iCloud 隐藏邮箱、别名和收件箱。</p></div>
        <div className="auth-telemetry"><span>本地服务</span><strong>就绪</strong></div>
      </section>
      <section className="auth-form-wrap">
        <div className="auth-form-card">
          <div className="auth-form-heading"><div className="form-icon"><Icon name={setupMode ? 'plus' : 'settings'} /></div><div><span className="eyebrow">{setupMode ? '首次部署' : '操作员登录'}</span><h2>{setupMode ? '设置本地服务' : '欢迎回来'}</h2></div></div>
          <p className="form-intro">{setupMode ? '创建本地操作员账号。凭据只保存在当前主机。' : isToken ? '输入此服务配置的访问口令。' : '登录后继续管理你的账号组。'}</p>
          <form onSubmit={submit}>
            {!setupMode && isToken ? <label className="field"><span>访问口令</span><input name="token" type="password" autoComplete="current-password" placeholder="粘贴访问口令" required autoFocus /></label> : <>
              <label className="field"><span>用户名</span><input name="username" autoComplete="username" placeholder="operator" required autoFocus={setupMode} minLength={2} /></label>
              <label className="field"><span>密码</span><input name="password" type="password" autoComplete={setupMode ? 'new-password' : 'current-password'} placeholder="至少 6 个字符" required minLength={6} /></label>
              {setupMode && <label className="field"><span>确认密码</span><input name="confirm" type="password" autoComplete="new-password" placeholder="再次输入密码" required minLength={6} /> </label>}
            </>}
            {error && <div className="form-error" role="alert"><Icon name="alert" size={16} />{error}</div>}
            <button className="button primary full-width" type="submit" disabled={busy}>{busy ? '连接中…' : setupMode ? '创建操作员账号' : '登录'}<Icon name="arrow" size={17} /></button>
          </form>
          <p className="auth-footnote">凭据由本地服务处理，不会发送给第三方。</p>
        </div>
      </section>
    </main>
  )
}
