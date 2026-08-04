import { ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Layout, Space, Typography } from 'antd'
import { HomeOutlined, LogoutOutlined, MailOutlined } from '@ant-design/icons'
import { api } from '../api/client'

// PageLayout 页面骨架: 渐变顶栏 + 内容区。
export default function PageLayout({ title, children, onLogout }: { title: string; children: ReactNode; onLogout: () => void }) {
  const navigate = useNavigate()

  const logout = async () => {
    await api.uiLogout().catch(() => undefined)
    onLogout()
    navigate('/login')
  }

  return (
    <Layout style={{ minHeight: '100vh', background: '#f0f2f7' }}>
      <Layout.Header
        className="hme-header"
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          paddingInline: 28,
        }}
      >
        <Space size={12}>
          <div
            style={{
              width: 34,
              height: 34,
              borderRadius: 10,
              background: 'rgba(255,255,255,0.2)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <MailOutlined style={{ fontSize: 18, color: '#fff' }} />
          </div>
          <Typography.Title level={4} style={{ color: '#fff', margin: 0, letterSpacing: 0.5 }}>
            {title}
          </Typography.Title>
        </Space>
        <Space>
          <Button ghost icon={<HomeOutlined />} onClick={() => navigate('/')}>
            账号列表
          </Button>
          <Button ghost icon={<LogoutOutlined />} onClick={logout}>
            注销
          </Button>
        </Space>
      </Layout.Header>
      <Layout.Content style={{ padding: '24px 28px', maxWidth: 1200, width: '100%', margin: '0 auto' }}>
        {children}
      </Layout.Content>
    </Layout>
  )
}
