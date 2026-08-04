import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Alert, Button, Card, Drawer, Space, Spin, Tag, Typography, message } from 'antd'
import { MailOutlined, ReloadOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { FullMailMessage, MailMessage, api } from '../api/client'
import MailList from '../components/MailList'

// SharePage 公开分享页 (免登录): 通过 token 链接只读查看别名收到的邮件。
// 每 30 秒自动刷新,也可手动刷新。
export default function SharePage() {
  const { token = '' } = useParams()
  const [alias, setAlias] = useState('')
  const [messages, setMessages] = useState<MailMessage[] | null>(null)
  const [method, setMethod] = useState<'imap' | 'web_api'>('web_api')
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [notFound, setNotFound] = useState(false)
  const [detail, setDetail] = useState<FullMailMessage | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [lastRefresh, setLastRefresh] = useState('')

  const load = async (silent = false) => {
    if (!silent) setRefreshing(true)
    try {
      const res = await api.publicShareInbox(token)
      setAlias(res.alias)
      setMessages(res.messages || [])
      setMethod(res.method)
      setLastRefresh(dayjs().format('HH:mm:ss'))
      setNotFound(false)
    } catch (e) {
      const err = e as { status?: number; message?: string }
      if (err.status === 404) setNotFound(true)
      else if (!silent) message.error(err.message || '加载失败')
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }

  useEffect(() => {
    load()
    // 30 秒静默轮询新邮件
    const timer = setInterval(() => load(true), 30000)
    return () => clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token])

  const openMessage = async (msg: MailMessage) => {
    if (method !== 'imap') {
      message.info('当前仅支持查看摘要')
      return
    }
    setDrawerOpen(true)
    setDetailLoading(true)
    setDetail(null)
    try {
      setDetail(await api.publicShareMessage(token, msg.id, msg.folder))
    } catch (e) {
      message.error((e as Error).message)
      setDrawerOpen(false)
    } finally {
      setDetailLoading(false)
    }
  }

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', marginTop: 200 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (notFound) {
    return (
      <div style={{ maxWidth: 480, margin: '120px auto' }}>
        <Alert type="error" message="链接无效" description="该分享链接不存在或已被吊销。" showIcon style={{ borderRadius: 12 }} />
      </div>
    )
  }

  return (
    <div style={{ minHeight: '100vh', background: '#f0f2f7', padding: '32px 16px' }}>
      <Card
        className="hme-card"
        style={{ maxWidth: 860, margin: '0 auto' }}
        title={
          <Space>
            <div className="mail-avatar" style={{ width: 32, height: 32, fontSize: 14 }}>
              <MailOutlined />
            </div>
            <span>{alias}</span>
            <Tag color={method === 'imap' ? 'blue' : 'orange'}>{method === 'imap' ? 'IMAP' : 'Web API'}</Tag>
          </Space>
        }
        extra={
          <Space>
            {lastRefresh && <Typography.Text type="secondary">更新于 {lastRefresh}</Typography.Text>}
            <Button icon={<ReloadOutlined />} loading={refreshing} onClick={() => load()}>
              刷新
            </Button>
          </Space>
        }
      >
        <MailList messages={messages} loading={refreshing} onOpen={openMessage} />
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          每 30 秒自动刷新 · 由 iCloud Distribution 分享
        </Typography.Text>
      </Card>

      <Drawer title={detail?.subject || '邮件详情'} open={drawerOpen} onClose={() => setDrawerOpen(false)} width={680}>
        <Spin spinning={detailLoading}>
          {detail && (
            <>
              <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
                发件人: {detail.from}
                <br />
                时间: {dayjs(detail.date).format('YYYY-MM-DD HH:mm:ss')}
              </Typography.Paragraph>
              {detail.content_type?.includes('html') ? (
                <iframe
                  title="mail-body"
                  sandbox=""
                  srcDoc={detail.body}
                  style={{ width: '100%', height: '60vh', border: '1px solid #eee', borderRadius: 10 }}
                />
              ) : (
                <Typography.Paragraph style={{ whiteSpace: 'pre-wrap' }}>{detail.body}</Typography.Paragraph>
              )}
            </>
          )}
        </Spin>
      </Drawer>
    </div>
  )
}

