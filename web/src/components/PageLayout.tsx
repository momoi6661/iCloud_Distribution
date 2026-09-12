import { useState, type ReactNode } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { api } from '../api/client'
import Icon from './Icon'
import { getInitialTheme, persistTheme, type ThemeMode } from '../theme'

export default function PageLayout({ title, eyebrow = '操作台', children, onLogout, showHeading = true }: { title: string; eyebrow?: string; children: ReactNode; onLogout: () => void; showHeading?: boolean }) {
  const navigate = useNavigate()
  const location = useLocation()
  const isDetail = location.pathname.startsWith('/accounts/')
  const active = isDetail ? 'accounts' : location.pathname === '/disabled' ? 'disabled' : 'accounts'
  const [theme, setTheme] = useState<ThemeMode>(() => getInitialTheme())
  const toggleTheme = () => { const next = theme === 'dark' ? 'light' : 'dark'; setTheme(next); persistTheme(next) }

  const go = (path: string) => {
    document.body.classList.remove('nav-open')
    navigate(path)
  }

  const logout = async () => {
    await api.uiLogout().catch(() => undefined)
    document.body.classList.remove('nav-open')
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
          <button className={`nav-item ${active === 'accounts' ? 'active' : ''}`} onClick={() => go('/')}><Icon name="grid" /><span>活跃账号</span><span className="nav-key">1</span></button>
          <button className={`nav-item ${active === 'disabled' ? 'active' : ''}`} onClick={() => go('/disabled')}><Icon name="archive" /><span>禁用账号</span><span className="nav-key">2</span></button>
        </nav>
        <div className="sidebar-bottom">
          <button className="nav-item logout-item" onClick={logout}><Icon name="logout" /><span>退出登录</span></button>
        </div>
      </aside>
      <main className="main-shell">
        <header className="topbar">
          <button className="mobile-menu" type="button" aria-label="打开导航" onClick={() => document.body.classList.toggle('nav-open')}><Icon name="menu" /></button>
          <div className="breadcrumb"><span>工作区</span><span className="breadcrumb-separator">/</span><strong>{title}</strong></div>
          <div className="topbar-actions"><button className="button secondary theme-trigger" type="button" aria-label={`切换到${theme === 'light' ? '深色' : '浅色'}`} onClick={toggleTheme}><Icon name="settings" size={16} /><span>切换到{theme === 'light' ? '深色' : '浅色'}</span></button></div>
        </header>
        <div className="page-content">{showHeading && <div className="page-heading"><div><span className="eyebrow">{eyebrow}</span><h1>{title}</h1></div><time dateTime={new Date().toISOString()}>刚刚更新</time></div>}{children}</div>
      </main>
    </div>
  )
}
