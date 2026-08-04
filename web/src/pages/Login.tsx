import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Form, Input, Spin, Typography, message } from 'antd'
import { LockOutlined, MailOutlined, UserOutlined } from '@ant-design/icons'
import { api, UIStatus } from '../api/client'

// LoginPage 登录/初始化页:
//   - 首次部署 (未初始化): 创建管理员账号
//   - 管理员账号模式: 用户名 + 密码登录
//   - 令牌模式 (-token 启动): 单口令登录
export default function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const [status, setStatus] = useState<UIStatus | null>(null)
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    api.uiStatus().then(setStatus).catch(() => setStatus(null))
  }, [])

  const submitSetup = async (values: { username: string; password: string; confirm: string }) => {
    if (values.password !== values.confirm) {
      message.error('两次输入的密码不一致')
      return
    }
    setLoading(true)
    try {
      await api.uiSetup(values.username, values.password)
      message.success(`管理员 ${values.username} 创建成功`)
      onSuccess()
      navigate('/')
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const submitLogin = async (values: { username?: string; password?: string; token?: string }) => {
    setLoading(true)
    try {
      await api.uiLogin(values)
      onSuccess()
      navigate('/')
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const card = (children: React.ReactNode) => (
    <div
      style={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        minHeight: '100vh',
        background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
      }}
    >
      <Card
        className="login-card"
        style={{ width: 400, borderRadius: 18, boxShadow: '0 24px 64px rgba(0,0,0,0.35)', border: 'none' }}
        styles={{ body: { padding: '44px 38px' } }}
      >
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <div
            style={{
              width: 64,
              height: 64,
              margin: '0 auto 16px',
              borderRadius: 16,
              background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <MailOutlined style={{ fontSize: 30, color: '#fff' }} />
          </div>
          <Typography.Title level={3} style={{ margin: 0 }}>
            iCloud Distribution
          </Typography.Title>
          <Typography.Text type="secondary">隐藏邮箱别名管理平台</Typography.Text>
        </div>
        {children}
      </Card>
    </div>
  )

  if (!status) {
    return card(
      <div style={{ textAlign: 'center' }}>
        <Spin />
      </div>,
    )
  }

  const primaryBtnStyle = {
    height: 44,
    borderRadius: 8,
    background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
    border: 'none',
  } as const

  // 首次部署: 创建管理员账号
  if (!status.initialized) {
    return card(
      <>
        <Typography.Paragraph type="secondary" style={{ textAlign: 'center' }}>
          首次使用,请创建管理员账号
        </Typography.Paragraph>
        <Form onFinish={submitSetup} size="large">
          <Form.Item
            name="username"
            rules={[
              { required: true, message: '请输入用户名' },
              { min: 2, max: 32, message: '长度 2-32 字符' },
            ]}
          >
            <Input prefix={<UserOutlined style={{ color: '#bbb' }} />} placeholder="用户名" style={{ borderRadius: 8 }} />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, min: 6, message: '密码至少 6 位' }]}>
            <Input.Password prefix={<LockOutlined style={{ color: '#bbb' }} />} placeholder="密码 (至少 6 位)" style={{ borderRadius: 8 }} />
          </Form.Item>
          <Form.Item name="confirm" rules={[{ required: true, message: '请再次输入密码' }]}>
            <Input.Password prefix={<LockOutlined style={{ color: '#bbb' }} />} placeholder="确认密码" style={{ borderRadius: 8 }} />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading} style={primaryBtnStyle}>
            创建并进入
          </Button>
        </Form>
      </>,
    )
  }

  // 令牌模式: 单口令
  if (status.token_mode) {
    return card(
      <Form onFinish={submitLogin} size="large">
        <Form.Item name="token" rules={[{ required: true, message: '请输入访问口令' }]}>
          <Input.Password prefix={<LockOutlined style={{ color: '#bbb' }} />} placeholder="访问口令" style={{ borderRadius: 8 }} />
        </Form.Item>
        <Button type="primary" htmlType="submit" block loading={loading} style={primaryBtnStyle}>
          登 录
        </Button>
      </Form>,
    )
  }

  // 管理员账号模式: 用户名 + 密码
  return card(
    <Form onFinish={submitLogin} size="large">
      <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
        <Input prefix={<UserOutlined style={{ color: '#bbb' }} />} placeholder="用户名" style={{ borderRadius: 8 }} />
      </Form.Item>
      <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
        <Input.Password prefix={<LockOutlined style={{ color: '#bbb' }} />} placeholder="密码" style={{ borderRadius: 8 }} />
      </Form.Item>
      <Button type="primary" htmlType="submit" block loading={loading} style={primaryBtnStyle}>
        登 录
      </Button>
    </Form>,
  )
}
