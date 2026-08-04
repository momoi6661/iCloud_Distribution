import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, Col, Form, Input, Modal, Popconfirm, Row, Select, Space, Statistic, Table, Tag, Tooltip, message } from 'antd'
import { CheckCircleOutlined, DeleteOutlined, KeyOutlined, MailOutlined, PlusOutlined, ThunderboltOutlined, UserOutlined } from '@ant-design/icons'
import { Account, api } from '../api/client'
import PageLayout from '../components/PageLayout'
import AutoLoginModal from '../components/AutoLoginModal'

// AccountsPage 账号列表页: 状态总览、添加账号、自动授权入口。
export default function AccountsPage({ onLogout }: { onLogout: () => void }) {
  const [accounts, setAccounts] = useState<Account[]>([])
  const [loading, setLoading] = useState(true)
  const [addOpen, setAddOpen] = useState(false)
  const [loginTarget, setLoginTarget] = useState<Account | null>(null)
  const navigate = useNavigate()
  const [form] = Form.useForm()

  const refresh = async () => {
    setLoading(true)
    try {
      setAccounts(await api.listAccounts())
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    refresh()
  }, [])

  const addAccount = async (values: { name: string; email?: string; cookies?: string; proxy?: string; host?: string }) => {
    try {
      await api.addAccount(values)
      message.success('账号已添加')
      setAddOpen(false)
      form.resetFields()
      refresh()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const statusTag = (acc: Account) => {
    switch (acc.status) {
      case 'active':
        return <Tag color="success">正常</Tag>
      case 'pending':
        return <Tag color="warning">待授权</Tag>
      default:
        return (
          <Tooltip title={acc.last_error}>
            <Tag color="error">异常</Tag>
          </Tooltip>
        )
    }
  }

  return (
    <PageLayout title="iCloud Distribution" onLogout={onLogout}>
      <Row gutter={16} style={{ marginBottom: 20 }}>
        {[
          { title: '账号总数', value: accounts.length, color: '#667eea', icon: <UserOutlined /> },
          {
            title: '正常账号',
            value: accounts.filter((a) => a.status === 'active').length,
            color: '#52c41a',
            icon: <CheckCircleOutlined />,
          },
          {
            title: '别名总数',
            value: accounts.reduce((sum, a) => sum + (a.alias_total || 0), 0),
            color: '#764ba2',
            icon: <MailOutlined />,
          },
        ].map((s) => (
          <Col span={8} key={s.title}>
            <Card className="stat-card hme-card" styles={{ body: { padding: '18px 24px' } }}>
              <Space size={16}>
                <div
                  style={{
                    width: 44,
                    height: 44,
                    borderRadius: 12,
                    background: `${s.color}14`,
                    color: s.color,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 22,
                  }}
                >
                  {s.icon}
                </div>
                <Statistic title={s.title} value={s.value} valueStyle={{ color: s.color, fontWeight: 700 }} />
              </Space>
            </Card>
          </Col>
        ))}
      </Row>
      <Card
        className="hme-card"
        title="账号管理"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setAddOpen(true)}>
            添加账号
          </Button>
        }
      >
        <Table
          rowKey="id"
          loading={loading}
          dataSource={accounts}
          pagination={false}
          onRow={(acc) => ({ onClick: () => navigate(`/accounts/${acc.id}`), style: { cursor: 'pointer' } })}
          columns={[
            { title: '名称', dataIndex: 'name' },
            { title: '邮箱', dataIndex: 'real_email', render: (v: string) => v || '-' },
            { title: '状态', key: 'status', render: (_, acc) => statusTag(acc) },
            {
              title: '别名',
              key: 'aliases',
              render: (_, acc) => `${acc.alias_active} / ${acc.alias_total}`,
            },
            {
              title: '操作',
              key: 'actions',
              render: (_, acc) => (
                <Space onClick={(e) => e.stopPropagation()}>
                  <Button
                    size="small"
                    type="primary"
                    ghost
                    icon={<ThunderboltOutlined />}
                    onClick={() => setLoginTarget(acc)}
                  >
                    自动授权
                  </Button>
                  <Popconfirm title="确认删除该账号?" onConfirm={() => api.removeAccount(acc.id).then(refresh)}>
                    <Button size="small" danger icon={<DeleteOutlined />} />
                  </Popconfirm>
                </Space>
              ),
            },
          ]}
        />
      </Card>

      <Modal
        title="添加 iCloud 账号"
        open={addOpen}
        onCancel={() => setAddOpen(false)}
        onOk={() => form.submit()}
        destroyOnClose
      >
        <Form form={form} layout="vertical" onFinish={addAccount}>
          <Form.Item name="name" label="账号名称" rules={[{ required: true, message: '请输入名称' }]}>
            <Input placeholder="例如: 主号" />
          </Form.Item>
          <Form.Item name="email" label="Apple ID (用于自动授权)" tooltip="不填则只能粘贴 Cookie 使用">
            <Input placeholder="you@example.com" />
          </Form.Item>
          <Form.Item
            name="host"
            label="账号区域"
            initialValue="icloud.com.cn"
            tooltip="国区 Apple ID 必须选择国区,否则授权会失败"
          >
            <Select
              options={[
                { value: 'icloud.com.cn', label: '国区 (icloud.com.cn)' },
                { value: 'icloud.com', label: '国际区 (icloud.com)' },
              ]}
            />
          </Form.Item>
          <Form.Item name="cookies" label="Cookie (可选)" tooltip="JSON 或 Header 格式,粘贴后跳过自动授权">
            <Input.TextArea rows={3} placeholder='{"X-APPLE-WEBAUTH-TOKEN": "..."}' />
          </Form.Item>
          <Form.Item name="proxy" label="代理 (可选)">
            <Input placeholder="http://user:pass@host:port 或 socks5://..." />
          </Form.Item>
        </Form>
        <Space direction="vertical" style={{ color: '#888' }}>
          <span>
            <KeyOutlined /> 添加后点击「自动授权」,输入密码即可自动获取 Cookie。
          </span>
        </Space>
      </Modal>

      {loginTarget && (
        <AutoLoginModal
          accountId={loginTarget.id}
          accountName={loginTarget.name}
          open
          onClose={(refreshed) => {
            setLoginTarget(null)
            if (refreshed) refresh()
          }}
        />
      )}
    </PageLayout>
  )
}
