import { useEffect, useState } from 'react'
import { Alert, Button, Drawer, Input, InputNumber, Space, Spin, Tag, Typography, message } from 'antd'
import { ReloadOutlined, SearchOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { FullMailMessage, InboxData, MailMessage, api } from '../api/client'
import MailList from './MailList'

interface Props {
  accountId: string
  alias: string // 空字符串 = 整个收件箱
}

// MailReader 邮件阅读器: 邮件列表 + 详情抽屉。
// web_api 路径只提供摘要,点击邮件时提示配置 App Password 以阅读正文。
export default function MailReader({ accountId, alias }: Props) {
  const [data, setData] = useState<InboxData | null>(null)
  const [loading, setLoading] = useState(false)
  const [days, setDays] = useState(7)
  const [detail, setDetail] = useState<FullMailMessage | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [query, setQuery] = useState('')

  const load = async (d = days) => {
    setLoading(true)
    try {
      setData(await api.inbox(accountId, alias, 30, d))
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accountId, alias])

  const openMessage = async (msg: MailMessage) => {
    if (data?.method !== 'imap') {
      message.info('当前为 Web API 读取,仅支持摘要。配置 App 专用密码后可阅读正文。')
      return
    }
    setDrawerOpen(true)
    setDetailLoading(true)
    setDetail(null)
    try {
      setDetail(await api.getMessage(accountId, msg.id, msg.folder))
    } catch (e) {
      message.error((e as Error).message)
      setDrawerOpen(false)
    } finally {
      setDetailLoading(false)
    }
  }

  // 按主题/发件人/收件人/预览本地即时过滤
  const filteredMessages = (() => {
    const msgs = data?.messages ?? null
    if (!msgs || !query.trim()) return msgs
    const q = query.trim().toLowerCase()
    return msgs.filter(
      (m) =>
        m.subject.toLowerCase().includes(q) ||
        m.from.toLowerCase().includes(q) ||
        m.to.toLowerCase().includes(q) ||
        m.preview.toLowerCase().includes(q),
    )
  })()

  return (
    <Spin spinning={loading && !!data}>
      <Space style={{ marginBottom: 12 }} wrap>
        <span>最近</span>
        <InputNumber
          min={1}
          max={30}
          value={days}
          onChange={(v) => {
            const d = v || 7
            setDays(d)
            load(d)
          }}
        />
        <span>天</span>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={() => load()}>
          刷新
        </Button>
        <Input
          placeholder="搜索主题 / 发件人 / 内容"
          prefix={<SearchOutlined style={{ color: '#bbb' }} />}
          allowClear
          style={{ width: 220 }}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        {data && <Tag color={data.method === 'imap' ? 'blue' : 'orange'}>{data.method === 'imap' ? 'IMAP' : 'Web API'}</Tag>}
        {data && <span style={{ color: '#9ca3af' }}>共 {data.count} 封</span>}
      </Space>

      {data?.method === 'web_api' && (
        <Alert
          style={{ marginBottom: 12, borderRadius: 10 }}
          type="info"
          showIcon
          message="当前通过 Web API 读取,仅显示邮件摘要。设置 App 专用密码后可使用 IMAP 阅读完整正文。"
        />
      )}

      <MailList messages={filteredMessages} loading={loading} onOpen={openMessage} />

      <Drawer
        title={detail?.subject || '邮件详情'}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        width={680}
      >
        <Spin spinning={detailLoading}>
          {detail && (
            <>
              <Typography.Paragraph type="secondary" style={{ fontSize: 13 }}>
                发件人: {detail.from}
                <br />
                收件人: {detail.to}
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
    </Spin>
  )
}
