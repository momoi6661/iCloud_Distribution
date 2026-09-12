import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type Account, type BatchAccountResult } from '../api/client'
import Icon from '../components/Icon'
import PageLayout from '../components/PageLayout'
import { Dialog } from '../components/Overlay'

const PAGE_SIZE = 20

function archiveNotice(result: BatchAccountResult, action: 'restore' | 'delete') { const notFound = result.not_found ?? 0; if (action === 'restore') return `已恢复 ${result.restored ?? 0} 个账号${notFound ? `，${notFound} 个未找到` : ''}。`; return `已删除 ${result.deleted ?? 0} 个账号${notFound ? `，${notFound} 个未找到` : ''}。` }

export default function DisabledAccountsPage({ onLogout }: { onLogout: () => void }) {
  const [accounts, setAccounts] = useState<Account[]>([]); const [loading, setLoading] = useState(true); const [selected, setSelected] = useState<string[]>([]); const [confirmDelete, setConfirmDelete] = useState(false); const [busy, setBusy] = useState(false); const [notice, setNotice] = useState(''); const [page, setPage] = useState(1); const navigate = useNavigate()
  const refresh = async () => { setLoading(true); try { setAccounts(await api.listDisabledAccounts()) } catch (e) { setNotice((e as Error).message) } finally { setLoading(false) } }
  useEffect(() => { refresh() }, [])
  const totalPages = Math.max(1, Math.ceil(accounts.length / PAGE_SIZE)); const pagedAccounts = useMemo(() => accounts.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE), [accounts, page]); useEffect(() => { setPage((current) => Math.min(current, totalPages)) }, [totalPages]); useEffect(() => { if (!notice) return undefined; const timer = window.setTimeout(() => setNotice(''), 4200); return () => window.clearTimeout(timer) }, [notice])
  const allSelected = pagedAccounts.length > 0 && pagedAccounts.every((account) => selected.includes(account.id)); const toggle = (id: string) => setSelected((current) => current.includes(id) ? current.filter((item) => item !== id) : [...current, id]); const toggleAll = () => setSelected(allSelected ? selected.filter((id) => !pagedAccounts.some((account) => account.id === id)) : [...new Set([...selected, ...pagedAccounts.map((account) => account.id)])])
  const restoreSelected = async () => {
    if (!selected.length) return
    setBusy(true)
    try {
      const results = await Promise.allSettled(selected.map((id) => api.restoreAccount(id)))
      const restored = results.filter((result) => result.status === 'fulfilled').length
      const failed = results.length - restored
      setSelected([])
      setNotice(`已恢复 ${restored} 个账号${failed ? `，${failed} 个恢复失败` : ''}。`)
      await refresh()
    } finally {
      setBusy(false)
    }
  }
  const deleteSelected = async () => { if (!selected.length) return; setBusy(true); try { const result = await api.batchDeleteAccounts(selected); setNotice(archiveNotice(result, 'delete')); setSelected([]); setConfirmDelete(false); await refresh() } catch (e) { setNotice((e as Error).message) } finally { setBusy(false) } }

  return <PageLayout title="禁用账号" eyebrow="归档" onLogout={onLogout}>
    {notice && <div className="notice toast" role="status" aria-live="polite"><span>{notice}</span><button className="text-button" onClick={() => setNotice('')}>关闭</button></div>}
    <section className="archive-intro"><div><span className="eyebrow">安全保留</span><h2>暂不参与转发的账号</h2><p>禁用账号会保留本地记录和别名。你可以批量恢复，也可以永久删除。</p></div><div className="archive-count"><strong>{accounts.length}</strong><span>个禁用账号</span></div></section>
    <section className="panel account-panel" aria-labelledby="disabled-list-title"><div className="panel-header"><div><span className="eyebrow">归档管理</span><h2 id="disabled-list-title">禁用矩阵 <span className="count-badge">{accounts.length}</span></h2></div></div>
      {selected.length > 0 && <div className="batch-toolbar"><span><strong>{selected.length}</strong> 个已选</span><span className="toolbar-separator" /><button className="button secondary" disabled={busy} onClick={restoreSelected}><Icon name="restore" size={16} />批量恢复</button><button className="danger-ghost" onClick={() => setConfirmDelete(true)}><Icon name="trash" size={16} />批量删除</button><button className="text-button" onClick={() => setSelected([])}>清除选择</button></div>}
      {loading ? <div className="skeleton-list">{[1, 2].map((item) => <div className="skeleton-row" key={item}><span /><span /><span /><span /></div>)}</div> : accounts.length === 0 ? <div className="empty-state"><div className="empty-glyph"><Icon name="archive" size={22} /></div><h3>归档中没有账号。</h3><p>停用账号后，它们会出现在这里，随时可以恢复。</p><button className="button secondary" onClick={() => navigate('/')}><Icon name="back" size={17} />查看活跃账号</button></div> : <div className="table-wrap"><table className="account-table"><thead><tr><th className="check-col"><label className="checkbox-hit"><input type="checkbox" aria-label="全选禁用账号" checked={allSelected} onChange={toggleAll} /></label></th><th>账号</th><th>状态</th><th>别名记录</th><th>最近验证</th><th className="action-col"><span className="sr-only">操作</span></th></tr></thead><tbody>{pagedAccounts.map((account) => <tr key={account.id} className={selected.includes(account.id) ? 'selected' : ''} onClick={() => navigate(`/accounts/${account.id}`)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') navigate(`/accounts/${account.id}`) }} tabIndex={0}><td className="check-col" onClick={(event) => event.stopPropagation()}><label className="checkbox-hit"><input type="checkbox" aria-label={`选择 ${account.name}`} checked={selected.includes(account.id)} onChange={() => toggle(account.id)} /></label></td><td><div className="account-identity"><span className="account-sigil sigil-disabled">{account.name.slice(0, 1).toUpperCase()}</span><span><strong>{account.name}</strong><small className="mono">{account.icloud_email || account.real_email || account.id}</small></span></div></td><td><span className="status status-disabled">已禁用</span></td><td><span className="mono">{account.alias_total || 0} 个别名已保留</span></td><td><span className="validation-time">{account.last_validated ? new Date(account.last_validated).toLocaleDateString() : '—'}</span></td><td className="action-col"><button className="button small secondary" onClick={(event) => { event.stopPropagation(); void (async () => { await api.restoreAccount(account.id); setNotice(`${account.name} 已恢复到活跃账号。`); await refresh() })() }}><Icon name="restore" size={15} />恢复</button></td></tr>)}</tbody></table></div>}{totalPages > 1 && <nav className="pagination" aria-label="禁用账号分页"><button className="button secondary" disabled={page === 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>上一页</button><span aria-live="polite">第 {page} / {totalPages} 页</span><button className="button secondary" disabled={page === totalPages} onClick={() => setPage((current) => Math.min(totalPages, current + 1))}>下一页</button></nav>}
    </section>
    <Dialog open={confirmDelete} title="删除所选账号？" onClose={() => setConfirmDelete(false)}><div className="dialog-body"><div className="dialog-warning destructive"><Icon name="trash" size={20} /><div><strong>此操作无法撤销。</strong><p>将永久删除所选 {selected.length} 个账号及其本地凭据和别名记录。</p></div></div><div className="dialog-actions"><button className="button secondary" onClick={() => setConfirmDelete(false)}>取消</button><button className="button danger" disabled={busy} onClick={deleteSelected}>{busy ? '删除中…' : '确认删除'}</button></div></div></Dialog>
  </PageLayout>
}
