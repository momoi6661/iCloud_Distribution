import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { Button, Card, Form, Input, Modal, Popconfirm, Select, Space, Table, Tabs, Tag, Tooltip, message } from 'antd'
import { CopyOutlined, LinkOutlined, MailOutlined, PlusOutlined, SearchOutlined, ShareAltOutlined, ThunderboltOutlined } from '@ant-design/icons'
import { Account, Alias, api } from '../api/client'
import PageLayout from '../components/PageLayout'
import AutoLoginModal from '../components/AutoLoginModal'
import BatchCreateModal from '../components/BatchCreateModal'
import MailReader from '../components/MailReader'
import ShareModal from '../components/ShareModal'

// AccountDetailPage 账号详情: 别名管理 + 收件箱 + 账号设置。
export default function AccountDetailPage({ onLogout }: { onLogout: () => void }) {
  const { id = '' } = useParams()
  const [account, setAccount] = useState<Account | null>(null)
  const [aliases, setAliases] = useState<Alias[]>([])
  const [loading, setLoading] = useState(false)
  const [tab, setTab] = useState('aliases')
  const [mailAlias, setMailAlias] = useState('')
  const [loginOpen, setLoginOpen] = useState(false)
  const [batchOpen, setBatchOpen] = useState(false)
  const [pwdOpen, setPwdOpen] = useState(false)
  const [shareOpen, setShareOpen] = useState(false)
  const [newLabel, setNewLabel] = useState('')
  const [creating, setCreating] = useState(false)
  const [aliasQuery, setAliasQuery] = useState('')
  const [pwdForm] = Form.useForm()

  const loadAccount = async () => {
    try {
      const list = await api.listAccounts()
      setAccount(list.find((a) => a.id === id) || null)
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const loadAliases = async () => {
    setLoading(true)
    try {
      const res = await api.listAliases(id)
      setAliases(res.aliases || [])
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadAccount()
    loadAliases()
  }, [id])

  const createOne = async () => {
    if (creating) return // 防重复点击 (创建耗时数秒,重复点击会产生多个别名)
    setCreating(true)
    try {
      const res = await api.createAlias(id, newLabel.trim())
      message.success(`已创建: ${res.email}`)
      setNewLabel('')
      loadAliases()
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setCreating(false)
    }
  }

  const shareAlias = async (a: Alias) => {
    try {
      const res = await api.createShare(id, a.email, a.label)
      const url = `${window.location.origin}${res.url}`
      await navigator.clipboard.writeText(url)
      message.success(`分享链接已复制: ${url}`)
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const setAppPassword = async (values: { icloud_email: string; app_password: string }) => {
    try {
      await api.setAppPassword(id, values.icloud_email, values.app_password)
      message.success('App 专用密码已设置并验证通过')
      setPwdOpen(false)
      pwdForm.resetFields()
    } catch (e) {
      message.error((e as Error).message)
    }
  }

  const viewMail = (alias: string) => {
    setMailAlias(alias)
    setTab('inbox')
  }

  // 按邮箱地址/标签过滤别名 (本地即时过滤)
  const filteredAliases = aliasQuery.trim()
    ? aliases.filter((a) => {
        const q = aliasQuery.trim().toLowerCase()
        return a.email.toLowerCase().includes(q) || (a.label || '').toLowerCase().includes(q)
      })
    : aliases

  return (
    <PageLayout title={account ? `账号: ${account.name}` : '账号详情'} onLogout={onLogout}>
      <Card
        size="small"
        className="hme-card" style={{ marginBottom: 16 }}
        title={
          <Space>
            <span>{account?.real_email || account?.icloud_email || id}</span>
            {account?.status === 'active' ? <Tag color="success">正常</Tag> : <Tag color="warning">{account?.status}</Tag>}
            {aliases[0]?.forwardTo && (
              <Tooltip title="HME 别名邮件的转发目标 (账号级设置)">
                <Tag color="blue">转发至 {aliases[0].forwardTo}</Tag>
              </Tooltip>
            )}
          </Space>
        }
        extra={
          <Space>
            <Button icon={<ThunderboltOutlined />} type="primary" ghost onClick={() => setLoginOpen(true)}>
              自动授权
            </Button>
            <Button onClick={() => setPwdOpen(true)}>设置 App 密码</Button>
            <Popconfirm
              title="把所有别名的转发地址改为 iCloud 邮箱?"
              description="改后别名邮件进入 iCloud 收件箱,面板可直接读取"
              onConfirm={() =>
                api
                  .setForwardTo(id, account?.icloud_email || '')
                  .then(() => {
                    message.success('转发地址已修改')
                    loadAliases()
                  })
                  .catch((e) => message.error((e as Error).message))
              }
            >
              <Button disabled={!account?.icloud_email}>转发至 iCloud 邮箱</Button>
            </Popconfirm>
          </Space>
        }
      />

      <Tabs
        activeKey={tab}
        onChange={setTab}
        items={[
          {
            key: 'aliases',
            label: `别名管理 (${aliases.length})`,
            children: (
              <Card
                size="small"
                className="hme-card"
                extra={
                  <Space wrap>
                    <Input
                      placeholder="搜索邮箱 / 标签"
                      prefix={<SearchOutlined style={{ color: '#bbb' }} />}
                      allowClear
                      style={{ width: 200 }}
                      value={aliasQuery}
                      onChange={(e) => setAliasQuery(e.target.value)}
                    />
                    <Input
                      placeholder="标签 (可选)"
                      style={{ width: 140 }}
                      value={newLabel}
                      onChange={(e) => setNewLabel(e.target.value)}
                      onPressEnter={createOne}
                    />
                    <Button type="primary" icon={<PlusOutlined />} loading={creating} onClick={createOne}>
                      {creating ? '创建中...' : '新建别名'}
                    </Button>
                    <Button onClick={() => setBatchOpen(true)}>批量创建</Button>
                    <Button icon={<LinkOutlined />} onClick={() => setShareOpen(true)}>
                      分享链接
                    </Button>
                  </Space>
                }
              >
                <Table
                  rowKey="anonymousId"
                  size="small"
                  loading={loading}
                  dataSource={filteredAliases}
                  pagination={{ pageSize: 20 }}
                  columns={[
                    {
                      title: '邮箱地址',
                      dataIndex: 'email',
                      render: (v: string) => (
                        <Space>
                          {v}
                          <Tooltip title="复制">
                            <Button
                              size="small"
                              type="text"
                              icon={<CopyOutlined />}
                              onClick={() => {
                                navigator.clipboard.writeText(v)
                                message.success('已复制')
                              }}
                            />
                          </Tooltip>
                        </Space>
                      ),
                    },
                    { title: '标签', dataIndex: 'label' },
                    {
                      title: '状态',
                      dataIndex: 'active',
                      render: (v: boolean) => (v ? <Tag color="success">启用</Tag> : <Tag>停用</Tag>),
                    },
                    {
                      title: '操作',
                      key: 'actions',
                      render: (_, a) => (
                        <Space>
                          <Button size="small" icon={<MailOutlined />} onClick={() => viewMail(a.email)}>
                            邮件
                          </Button>
                          <Button size="small" icon={<ShareAltOutlined />} onClick={() => shareAlias(a)}>
                            分享
                          </Button>
                          {a.active ? (
                            <Button size="small" onClick={() => api.deactivateAlias(id, a.anonymousId).then(loadAliases)}>
                              停用
                            </Button>
                          ) : (
                            <Button size="small" onClick={() => api.reactivateAlias(id, a.anonymousId).then(loadAliases)}>
                              激活
                            </Button>
                          )}
                          <Popconfirm
                            title="确认删除该别名? 不可恢复"
                            onConfirm={() => api.deleteAlias(id, a.anonymousId).then(loadAliases)}
                          >
                            <Button size="small" danger>
                              删除
                            </Button>
                          </Popconfirm>
                        </Space>
                      ),
                    },
                  ]}
                />
              </Card>
            ),
          },
          {
            key: 'inbox',
            label: '收件箱',
            children: (
              <Card size="small" className="hme-card">
                <Space style={{ marginBottom: 16 }}>
                  <span>邮件范围:</span>
                  <Select
                    style={{ width: 320 }}
                    value={mailAlias}
                    onChange={setMailAlias}
                    options={[
                      { value: '', label: '整个收件箱' },
                      ...aliases.map((a) => ({ value: a.email, label: `${a.email}${a.label ? ` (${a.label})` : ''}` })),
                    ]}
                  />
                  {mailAlias && (
                    <Button
                      icon={<ShareAltOutlined />}
                      onClick={() => shareAlias(aliases.find((a) => a.email === mailAlias) || { email: mailAlias } as Alias)}
                    >
                      分享此邮箱
                    </Button>
                  )}
                </Space>
                <MailReader key={mailAlias} accountId={id} alias={mailAlias} />
              </Card>
            ),
          },
        ]}
      />

      {loginOpen && (
        <AutoLoginModal
          accountId={id}
          accountName={account?.name || id}
          open
          onClose={(refreshed) => {
            setLoginOpen(false)
            if (refreshed) loadAccount()
          }}
        />
      )}
      <BatchCreateModal
        accountId={id}
        open={batchOpen}
        onClose={(created) => {
          setBatchOpen(false)
          if (created) loadAliases()
        }}
      />
      <ShareModal accountId={id} open={shareOpen} onClose={() => setShareOpen(false)} />

      <Modal
        title="设置 App 专用密码 (IMAP 读邮件)"
        open={pwdOpen}
        onCancel={() => setPwdOpen(false)}
        onOk={() => pwdForm.submit()}
        destroyOnClose
      >
        <Form form={pwdForm} layout="vertical" onFinish={setAppPassword}>
          <Form.Item name="icloud_email" label="iCloud 邮箱" rules={[{ required: true, message: '必填' }]}>
            <Input placeholder="you@icloud.com" />
          </Form.Item>
          <Form.Item
            name="app_password"
            label="App 专用密码"
            rules={[{ required: true, message: '必填' }]}
            extra="在 appleid.apple.com → 登录和安全 → App 专用密码 生成"
          >
            <Input.Password placeholder="xxxx-xxxx-xxxx-xxxx" />
          </Form.Item>
        </Form>
      </Modal>
    </PageLayout>
  )
}
