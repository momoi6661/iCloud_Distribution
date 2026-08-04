import { useEffect, useState } from 'react'
import { BrowserRouter, Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { Spin } from 'antd'
import { api } from './api/client'
import LoginPage from './pages/Login'
import AccountsPage from './pages/Accounts'
import AccountDetailPage from './pages/AccountDetail'
import SharePage from './pages/SharePage'

// App 根组件: 启动时查询鉴权状态,决定渲染登录页还是主界面。
export default function App() {
  const [loading, setLoading] = useState(true)
  const [authed, setAuthed] = useState(false)

  useEffect(() => {
    api
      .uiStatus()
      .then((s) => setAuthed(s.authenticated))
      .catch(() => setAuthed(false))
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', marginTop: 200 }}>
        <Spin size="large" tip="加载中..." />
      </div>
    )
  }

  return (
    <BrowserRouter>
      <AuthRedirect onExpire={() => setAuthed(false)} />
      <Routes>
        <Route path="/login" element={<LoginPage onSuccess={() => setAuthed(true)} />} />
        {/* 公开分享页: 免登录 */}
        <Route path="/share/:token" element={<SharePage />} />
        <Route
          path="/"
          element={authed ? <AccountsPage onLogout={() => setAuthed(false)} /> : <Navigate to="/login" replace />}
        />
        <Route
          path="/accounts/:id"
          element={authed ? <AccountDetailPage onLogout={() => setAuthed(false)} /> : <Navigate to="/login" replace />}
        />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  )
}

// AuthRedirect 监听 API 层发出的 401 事件,跳转登录页。
function AuthRedirect({ onExpire }: { onExpire: () => void }) {
  const navigate = useNavigate()
  useEffect(() => {
    const handler = () => {
      onExpire()
      navigate('/login')
    }
    window.addEventListener('hme:unauthorized', handler)
    return () => window.removeEventListener('hme:unauthorized', handler)
  }, [navigate, onExpire])
  return null
}
