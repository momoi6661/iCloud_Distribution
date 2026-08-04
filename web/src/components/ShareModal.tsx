import { useEffect, useState } from 'react'
import { Button, Input, List, Modal, Popconfirm, Space, Typography, message } from 'antd'
import { CopyOutlined, DeleteOutlined, LinkOutlined } from '@ant-design/icons'
import dayjs from 'dayjs'
import { api } from '../api/client'

interface ShareItem {
  token: string
  account_id: string
  alias: string
  label?: string
  created_at: string
}

interface Props {
  accountId: string
  open: boolean
  onClose: () => void
}

// ShareModal 分享链接管理: 查看/复制/吊销该账号的所有分享链接。
// 创建分享在别名列表的「分享」按钮触发,此处展示已有链接。
export default function ShareModal({ accountId, open, onClose }: Props) {
  const [shares, setShares] = useState<ShareItem[]>([])
  const [loading, setLoading] = useState(false)

  const load = async () => {
    setLoading(true)
    try {
      setShares((await api.listShares(accountId)) || [])
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (open) load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const copy = (token: string) => {
    const url = `${window.location.origin}/share/${token}`
    navigator.clipboard.writeText(url)
    message.success('链接已复制')
  }

  return (
    <Modal title="分享链接管理" open={open} onCancel={onClose} footer={null} width={640}>
      <Typography.Paragraph type="secondary">
        持有链接的人无需登录即可查看对应邮箱的邮件 (只读)。请只把链接发给信任的人。
      </Typography.Paragraph>
      <List
        loading={loading}
        dataSource={shares}
        locale={{ emptyText: '暂无分享链接,在别名列表点击「分享」创建' }}
        renderItem={(sh) => (
          <List.Item
            actions={[
              <Button key="copy" size="small" icon={<CopyOutlined />} onClick={() => copy(sh.token)}>
                复制链接
              </Button>,
              <Popconfirm
                key="del"
                title="吊销后链接立即失效"
                onConfirm={() => api.deleteShare(sh.token).then(load)}
              >
                <Button size="small" danger icon={<DeleteOutlined />}>
                  吊销
                </Button>
              </Popconfirm>,
            ]}
          >
            <List.Item.Meta
              title={
                <Space>
                  <LinkOutlined />
                  {sh.alias}
                  {sh.label && <Typography.Text type="secondary">({sh.label})</Typography.Text>}
                </Space>
              }
              description={`创建于 ${dayjs(sh.created_at).format('YYYY-MM-DD HH:mm')}`}
            />
            <Input
              size="small"
              readOnly
              value={`${window.location.origin}/share/${sh.token}`}
              style={{ marginTop: 4, color: '#888' }}
              onFocus={(e) => e.target.select()}
            />
          </List.Item>
        )}
      />
    </Modal>
  )
}
