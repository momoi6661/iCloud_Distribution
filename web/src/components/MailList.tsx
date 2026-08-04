import { Empty, Skeleton, Tag } from 'antd'
import dayjs from 'dayjs'
import { MailMessage } from '../api/client'

interface Props {
  messages: MailMessage[] | null // null = 未加载
  loading: boolean
  onOpen: (msg: MailMessage) => void
}

// MailList 统一的邮件列表 (收件箱页与分享页共用)。
// 发件人首字母头像 + 主题/预览/日期,垃圾邮件带 Junk 标记。
export default function MailList({ messages, loading, onOpen }: Props) {
  if (loading && !messages) {
    return (
      <div style={{ padding: 8 }}>
        {[1, 2, 3].map((i) => (
          <div key={i} style={{ display: 'flex', gap: 14, padding: '14px 16px' }}>
            <Skeleton.Avatar active size={40} shape="square" />
            <Skeleton active title={{ width: '40%' }} paragraph={{ rows: 1, width: '80%' }} />
          </div>
        ))}
      </div>
    )
  }

  if (!messages || messages.length === 0) {
    return <Empty description="暂无邮件" style={{ padding: '48px 0' }} />
  }

  return (
    <div>
      {messages.map((msg) => {
        const isJunk = msg.folder === 'Junk'
        const initial = (msg.from || '?').replace(/["<>]/g, '').trim().charAt(0).toUpperCase() || '?'
        return (
          <div key={msg.id} className="mail-item" onClick={() => onOpen(msg)}>
            <div className={`mail-avatar${isJunk ? ' junk' : ''}`}>{initial}</div>
            <div className="mail-main">
              <div className="mail-title-row">
                <span className="mail-subject">
                  {msg.subject || '(无主题)'}
                  {isJunk && (
                    <Tag color="warning" style={{ marginLeft: 8, fontSize: 11 }}>
                      垃圾邮件
                    </Tag>
                  )}
                </span>
                <span className="mail-date">{dayjs(msg.date).format('MM-DD HH:mm')}</span>
              </div>
              <div className="mail-from">{msg.from}</div>
              {msg.preview && <div className="mail-preview">{msg.preview}</div>}
            </div>
          </div>
        )
      })}
    </div>
  )
}
