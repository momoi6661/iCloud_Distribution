import { useState } from 'react'
import { Alert, Button, Input, Modal, Select, Space, Steps, Typography, message } from 'antd'
import { api } from '../api/client'

interface Props {
  accountId: string
  accountName: string
  open: boolean
  onClose: (refreshed: boolean) => void
}

interface Phone {
  id: number
  numberWithDialCode: string
}

// AutoLoginModal 自动授权向导: 输入密码 → (如需 2FA) 设备推送或短信验证 → 完成。
// 后端通过 SRP 协议自动完成登录全流程并保存 Cookie。
export default function AutoLoginModal({ accountId, accountName, open, onClose }: Props) {
  const [step, setStep] = useState(0)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [sessionId, setSessionId] = useState('')
  const [method, setMethod] = useState<'device' | 'sms'>('device')
  const [phones, setPhones] = useState<Phone[]>([])
  const [phoneId, setPhoneId] = useState<number>(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const reset = () => {
    setStep(0)
    setPassword('')
    setCode('')
    setSessionId('')
    setMethod('device')
    setPhones([])
    setPhoneId(0)
    setError('')
    setLoading(false)
  }

  const close = (refreshed: boolean) => {
    reset()
    onClose(refreshed)
  }

  const submitPassword = async () => {
    if (!password) {
      setError('请输入 iCloud 密码')
      return
    }
    setLoading(true)
    setError('')
    try {
      const res = await api.loginStart(accountId, password)
      if (res.status === 'done') {
        message.success('授权成功,Cookie 已自动保存')
        close(true)
      } else {
        setSessionId(res.session_id || '')
        setStep(1)
        message.info('已请求推送验证码到受信任设备,也可用短信验证')
      }
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const switchToSMS = async () => {
    setLoading(true)
    setError('')
    try {
      const res = await api.loginPhones(accountId, sessionId)
      if (!res.phones?.length) {
        setError('账号没有受信任手机号')
        return
      }
      setPhones(res.phones)
      setPhoneId(res.phones[0].id)
      setMethod('sms')
      // 自动向第一个手机号发送短信
      await api.loginSMS(accountId, sessionId, res.phones[0].id)
      message.success('短信验证码已发送')
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const sendSMS = async (id: number) => {
    setPhoneId(id)
    setLoading(true)
    try {
      await api.loginSMS(accountId, sessionId, id)
      message.success('短信验证码已发送')
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const resendDevice = async () => {
    setLoading(true)
    try {
      await api.loginResend(accountId, sessionId)
      message.success('已重新请求推送')
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const submitOTP = async () => {
    if (!/^\d{6}$/.test(code)) {
      setError('请输入 6 位数字验证码')
      return
    }
    setLoading(true)
    setError('')
    try {
      await api.loginOTP(accountId, sessionId, code, method, phoneId)
      message.success('授权成功,Cookie 已自动保存')
      close(true)
    } catch (e) {
      setError((e as Error).message)
      // 验证码错误后会话已失效,回到第一步重新登录
      setStep(0)
      setPassword('')
      setCode('')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Modal
      title={`自动授权 — ${accountName}`}
      open={open}
      onCancel={() => close(false)}
      okText={step === 0 ? '开始授权' : '提交验证码'}
      onOk={step === 0 ? submitPassword : submitOTP}
      confirmLoading={loading}
      destroyOnClose
    >
      <Steps
        current={step}
        size="small"
        items={[{ title: '验证密码' }, { title: '双重认证' }]}
        style={{ marginBottom: 24 }}
      />
      {step === 0 ? (
        <>
          <Typography.Paragraph type="secondary">
            输入该账号的 iCloud 登录密码(不是 App 专用密码)。密码仅用于本次登录,不会被保存。
          </Typography.Paragraph>
          <Input.Password
            placeholder="iCloud 密码"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            onPressEnter={submitPassword}
            autoFocus
          />
        </>
      ) : (
        <>
          <Space style={{ marginBottom: 12 }}>
            <Button
              type={method === 'device' ? 'primary' : 'default'}
              size="small"
              onClick={() => setMethod('device')}
            >
              设备推送
            </Button>
            <Button type={method === 'sms' ? 'primary' : 'default'} size="small" onClick={switchToSMS}>
              手机短信
            </Button>
          </Space>

          {method === 'device' ? (
            <Typography.Paragraph type="secondary">
              验证码已推送到你的 iPhone / Mac(系统弹窗)。没收到可
              <Button type="link" size="small" onClick={resendDevice} style={{ padding: 0 }}>
                重新推送
              </Button>
              ,也可在设备上: 设置 → Apple 账户 → 登录与安全性 → 获取验证码。
            </Typography.Paragraph>
          ) : (
            <Space direction="vertical" style={{ width: '100%', marginBottom: 8 }}>
              <Select
                style={{ width: '100%' }}
                value={phoneId}
                onChange={sendSMS}
                options={phones.map((p) => ({ value: p.id, label: p.numberWithDialCode }))}
              />
              <Typography.Text type="secondary">切换手机号会自动重发短信</Typography.Text>
            </Space>
          )}

          <Input
            placeholder="6 位验证码"
            value={code}
            onChange={(e) => setCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            onPressEnter={submitOTP}
            maxLength={6}
            autoFocus
            style={{ letterSpacing: 8, fontSize: 20, textAlign: 'center' }}
          />
        </>
      )}
      {error && <Alert style={{ marginTop: 16 }} type="error" message={error} showIcon />}
    </Modal>
  )
}
