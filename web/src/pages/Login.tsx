import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Form, Input, Typography, message } from 'antd'
import { LockOutlined, MailOutlined } from '@ant-design/icons'
import { api } from '../api/client'

// LoginPage UI 访问口令登录页。
export default function LoginPage({ onSuccess }: { onSuccess: () => void }) {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const submit = async ({ token }: { token: string }) => {
    setLoading(true)
    try {
      await api.uiLogin(token)
      onSuccess()
      navigate('/')
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
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
        style={{
          width: 400,
          borderRadius: 18,
          boxShadow: '0 24px 64px rgba(0,0,0,0.35)',
          border: 'none',
        }}
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
        <Form onFinish={submit} size="large">
          <Form.Item name="token" rules={[{ required: true, message: '请输入访问口令' }]}>
            <Input.Password
              prefix={<LockOutlined style={{ color: '#bbb' }} />}
              placeholder="访问口令"
              style={{ borderRadius: 8 }}
            />
          </Form.Item>
          <Button
            type="primary"
            htmlType="submit"
            block
            loading={loading}
            style={{
              height: 44,
              borderRadius: 8,
              background: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
              border: 'none',
            }}
          >
            登 录
          </Button>
        </Form>
      </Card>
    </div>
  )
}
