import { useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { api } from './api/client'
import LoginPage from './pages/Login'
import AccountsPage from './pages/Accounts'
import DisabledAccountsPage from './pages/DisabledAccounts'
import AccountDetailPage from './pages/AccountDetail'
import SharePage from './pages/SharePage'

export default function App() {
  const [loading, setLoading] = useState(true)
  const [authed, setAuthed] = useState(false)

  useEffect(() => {
    api.uiStatus().then((status) => setAuthed(!status.auth_required || status.authenticated)).catch(() => setAuthed(false)).finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="app-loading"><div className="loading-mark">IM</div><span>正在连接本地服务</span></div>

  return (
    <BrowserRouter>
      <AuthRedirect onExpire={() => setAuthed(false)} />
      <Routes>
        <Route path="/login" element={<LoginPage onSuccess={() => setAuthed(true)} />} />
        <Route path="/share/:token" element={<SharePage />} />
        <Route path="/" element={authed ? <AccountsPage onLogout={() => setAuthed(false)} /> : <Navigate to="/login" replace />} />
        <Route path="/disabled" element={authed ? <DisabledAccountsPage onLogout={() => setAuthed(false)} /> : <Navigate to="/login" replace />} />
        <Route path="/accounts/:id" element={authed ? <AccountDetailPage onLogout={() => setAuthed(false)} /> : <Navigate to="/login" replace />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
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
