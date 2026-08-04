import { useState } from 'react'
import { Form, Input, InputNumber, Modal, Table, Tag, message } from 'antd'
import { BatchCreateResult, api } from '../api/client'

interface Props {
  accountId: string
  open: boolean
  onClose: (created: boolean) => void
}

// BatchCreateModal 批量创建别名: 设置数量和标签前缀,展示逐个结果。
export default function BatchCreateModal({ accountId, open, onClose }: Props) {
  const [form] = Form.useForm()
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<BatchCreateResult | null>(null)

  const submit = async (values: { count: number; label_prefix?: string }) => {
    setLoading(true)
    try {
      const res = await api.batchCreate(accountId, values.count, values.label_prefix || '')
      setResult(res)
      message.success(`完成: 成功 ${res.succeeded} 个,失败 ${res.failed} 个`)
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  const close = () => {
    const created = (result?.succeeded || 0) > 0
    setResult(null)
    form.resetFields()
    onClose(created)
  }

  return (
    <Modal
      title="批量创建别名"
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText="开始创建"
      confirmLoading={loading}
      width={640}
      destroyOnClose
    >
      {!result ? (
        <Form form={form} layout="vertical" onFinish={submit} initialValues={{ count: 5 }}>
          <Form.Item
            name="count"
            label="创建数量"
            rules={[{ required: true, message: '请输入数量' }]}
            extra="单次最多 50 个,频繁创建可能触发 iCloud 风控"
          >
            <InputNumber min={1} max={50} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item name="label_prefix" label="标签前缀 (可选)" extra="自动追加序号,例如「注册 1」「注册 2」">
            <Input placeholder="例如: 注册" />
          </Form.Item>
        </Form>
      ) : (
        <Table
          rowKey="index"
          size="small"
          pagination={false}
          scroll={{ y: 360 }}
          dataSource={result.results}
          columns={[
            { title: '#', dataIndex: 'index', width: 50 },
            { title: '标签', dataIndex: 'label' },
            {
              title: '结果',
              key: 'result',
              render: (_, r) =>
                r.success ? <Tag color="success">{r.email}</Tag> : <Tag color="error">{r.error}</Tag>,
            },
          ]}
        />
      )}
    </Modal>
  )
}
