import { useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { api } from './api/client'
import type { UIIdentity } from './api/client'
import { AuthContext } from './auth-context'
import LoginPage from './pages/Login'
import AccountsPage from './pages/Accounts'
import DisabledAccountsPage from './pages/DisabledAccounts'
import AccountDetailPage from './pages/AccountDetail'
import SharePage from './pages/SharePage'
import UsersPage from './pages/Users'
import ProfilePage from './pages/Profile'

export default function App() {
  const [loading, setLoading] = useState(true)
  const [authed, setAuthed] = useState(false)
  const [identity, setIdentity] = useState<UIIdentity | null>(null)

  useEffect(() => {
    api.uiStatus().then((status) => { setAuthed(status.authenticated); setIdentity(status.user || null) }).catch(() => { setAuthed(false); setIdentity(null) }).finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="app-loading"><div className="loading-mark">IM</div><span>正在连接本地服务</span></div>
  const logout = () => { setAuthed(false); setIdentity(null) }

  return (
    <AuthContext.Provider value={identity}><BrowserRouter>
      <AuthRedirect onExpire={logout} />
      <Routes>
        <Route path="/login" element={<LoginPage onSuccess={(user) => { setAuthed(true); setIdentity(user) }} />} />
        <Route path="/share/:token" element={<SharePage />} />
        <Route path="/" element={authed ? <AccountsPage onLogout={logout} /> : <Navigate to="/login" replace />} />
        <Route path="/disabled" element={authed ? <DisabledAccountsPage onLogout={logout} /> : <Navigate to="/login" replace />} />
        <Route path="/accounts/:id" element={authed ? <AccountDetailPage onLogout={logout} /> : <Navigate to="/login" replace />} />
        <Route path="/users" element={authed && identity?.role === 'superadmin' ? <UsersPage onLogout={logout} /> : <Navigate to="/" replace />} />
        <Route path="/profile" element={authed && identity?.role === 'user' ? <ProfilePage onLogout={logout} /> : <Navigate to="/" replace />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter></AuthContext.Provider>
  )
}

function AuthRedirect({ onExpire }: { onExpire: () => void }) {
  const navigate = useNavigate()
  useEffect(() => {
    const handler = () => { onExpire(); navigate('/login') }
    window.addEventListener('hme:unauthorized', handler)
    return () => window.removeEventListener('hme:unauthorized', handler)
  }, [navigate, onExpire])
  return null
}
