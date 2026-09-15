import { useEffect, useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import { useIdentity } from '../auth-context'
import Icon from './Icon'
import { getInitialTheme, persistTheme, type ThemeMode } from '../theme'

export default function PageLayout({ title, eyebrow = '操作台', children, onLogout, showHeading = true }: { title: string; eyebrow?: string; children: ReactNode; onLogout: () => void; showHeading?: boolean }) {
  const navigate = useNavigate()
  const location = useLocation()
  const isDetail = location.pathname.startsWith('/accounts/')
  const active = isDetail ? 'accounts' : location.pathname === '/disabled' ? 'disabled' : 'accounts'
  const [theme, setTheme] = useState<ThemeMode>(() => getInitialTheme())
  const [mobileNavOpen, setMobileNavOpen] = useState(false)
  const role = useIdentity()?.role || ''
  const toggleTheme = () => { const next = theme === 'dark' ? 'light' : 'dark'; setTheme(next); persistTheme(next) }

  const closeMobileNav = () => setMobileNavOpen(false)

  useEffect(() => {
    document.body.classList.toggle('nav-open', mobileNavOpen)
    return () => document.body.classList.remove('nav-open')
  }, [mobileNavOpen])

  useEffect(() => {
    if (!mobileNavOpen) return undefined
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') closeMobileNav()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  }, [mobileNavOpen])

  const go = (path: string) => {
    closeMobileNav()
    navigate(path)
  }

  const logout = async () => {
    await api.uiLogout().catch(() => undefined)
    closeMobileNav()
    onLogout()
    navigate('/login')
  }

  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="主导航">
        <div className="sidebar-top">
          <button className="wordmark" onClick={() => go('/')} aria-label="返回活跃账号">
            <span>iCloud 邮箱管理</span>
          </button>
        </div>
        <nav className="sidebar-nav">
          <p className="nav-label">工作区</p>
          <button className={`nav-item ${active === 'accounts' ? 'active' : ''}`} onClick={() => go('/')}><Icon name="grid" /><span>活跃账号</span></button>
          <button className={`nav-item ${active === 'disabled' ? 'active' : ''}`} onClick={() => go('/disabled')}><Icon name="archive" /><span>禁用账号</span></button>
          {role==='superadmin'&&<button className={`nav-item ${location.pathname==='/users'?'active':''}`} onClick={()=>go('/users')}><Icon name="settings" /><span>用户管理</span></button>}
          {role==='user'&&<button className={`nav-item ${location.pathname==='/profile'?'active':''}`} onClick={()=>go('/profile')}><Icon name="settings" /><span>账号安全</span></button>}
        </nav>
        <div className="sidebar-bottom">
          <button className="nav-item logout-item" onClick={logout}><Icon name="logout" /><span>退出登录</span></button>
        </div>
      </aside>
      {mobileNavOpen && (
        <button
          className="mobile-nav-dismiss"
          type="button"
          aria-label="关闭导航"
          onClick={closeMobileNav}
        />
      )}
      <main className="main-shell">
        <header className="topbar">
          <button className="mobile-menu" type="button" aria-label={mobileNavOpen ? '关闭导航' : '打开导航'} aria-expanded={mobileNavOpen} onClick={() => setMobileNavOpen((open) => !open)}><Icon name="menu" /></button>
          <div className="breadcrumb"><span>工作区</span><span className="breadcrumb-separator">/</span><strong>{title}</strong></div>
          <div className="topbar-actions"><button className="button secondary theme-trigger" type="button" aria-label={`切换到${theme === 'light' ? '深色' : '浅色'}`} onClick={toggleTheme}><Icon name="settings" size={16} /><span>切换到{theme === 'light' ? '深色' : '浅色'}</span></button></div>
        </header>
        <div className="page-content">{showHeading && <div className="page-heading"><div><span className="eyebrow">{eyebrow}</span><h1>{title}</h1></div><time dateTime={new Date().toISOString()}>刚刚更新</time></div>}{children}</div>
      </main>
    </div>
  )
}
