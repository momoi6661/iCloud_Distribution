import { useEffect, useState, type FormEvent } from 'react'
import { api, type AppUser } from '../api/client'
import Icon from '../components/Icon'
import PasswordInput from '../components/PasswordInput'
import PageLayout from '../components/PageLayout'
import { Dialog } from '../components/Overlay'

export default function UsersPage({ onLogout }: { onLogout: () => void }) {
  const [users,setUsers]=useState<AppUser[]>([]); const [busy,setBusy]=useState(false); const [notice,setNotice]=useState('')
  const [resetUser,setResetUser]=useState<AppUser|null>(null)
  const load=()=>api.listUsers().then((value)=>setUsers(value.users||[])).catch((e)=>setNotice((e as Error).message))
  useEffect(()=>{void load()},[])
  const create=async(event:FormEvent<HTMLFormElement>)=>{event.preventDefault();const form=event.currentTarget;const values=Object.fromEntries(new FormData(form).entries()) as Record<string,string>;setBusy(true);try{await api.createUser(values.username,values.password,true);form.reset();setNotice('用户已创建，首次登录后需要修改密码。');await load()}catch(e){setNotice((e as Error).message)}finally{setBusy(false)}}
  const reset=async(event:FormEvent<HTMLFormElement>)=>{event.preventDefault();if(!resetUser)return;const values=Object.fromEntries(new FormData(event.currentTarget).entries()) as Record<string,string>;setBusy(true);try{await api.resetUserPassword(resetUser.id,values.password,true);setResetUser(null);setNotice('密码已重置，该用户现有会话已失效。');await load()}catch(e){setNotice((e as Error).message)}finally{setBusy(false)}}
  const toggle=async(user:AppUser)=>{const next=user.status==='active'?'disabled':'active';if(next==='disabled'&&!window.confirm(`确认停用 ${user.username}？该用户会立即退出登录。`))return;try{await api.setUserStatus(user.id,next);setNotice(next==='active'?'用户已恢复。':'用户已停用。');await load()}catch(e){setNotice((e as Error).message)}}
  const remove=async(user:AppUser)=>{if(!window.confirm(`确认删除用户 ${user.username}？此操作不会自动删除其 iCloud 账号。`))return;try{await api.deleteUser(user.id);setNotice('用户已删除。');await load()}catch(e){setNotice((e as Error).message)}}
  return <PageLayout title="用户管理" eyebrow="超级管理员" onLogout={onLogout}>
    {notice&&<div className="notice"><span>{notice}</span><button className="text-button" onClick={()=>setNotice('')}>关闭</button></div>}
    <section className="panel user-create-panel"><div className="panel-header"><div><span className="eyebrow">创建账号</span><h2>添加普通用户</h2></div></div><form className="user-create-form" onSubmit={create}><label className="field"><span>用户名</span><input name="username" autoComplete="off" required placeholder="字母、数字、点或下划线" /></label><label className="field"><span>初始密码</span><PasswordInput name="password" autoComplete="new-password" minLength={8} required placeholder="至少 8 位" /></label><button className="button primary" disabled={busy}>{busy?'创建中…':'创建用户'}</button></form></section>
    <section className="panel user-list-panel"><div className="panel-header"><div><span className="eyebrow">访问控制</span><h2>普通用户 <span className="count-badge">{users.length}</span></h2></div></div><div className="user-list">{users.length===0?<div className="empty-state small-empty"><h3>还没有普通用户。</h3><p>创建后，用户可以添加并管理自己的 iCloud 账号。</p></div>:users.map(user=><div className="user-row" key={user.id}><div className="account-sigil">{user.username.slice(0,1).toUpperCase()}</div><div className="user-main"><strong>{user.username}</strong><span>{user.status==='active'?'可登录':'已停用'} · {user.must_change_password?'等待首次修改密码':'密码已设置'}{user.last_login_at?` · 最近登录 ${new Date(user.last_login_at).toLocaleString('zh-CN')}`:''}</span></div><div className="user-actions"><button className="button secondary" onClick={()=>setResetUser(user)}>重置密码</button><button className="button secondary" onClick={()=>void toggle(user)}>{user.status==='active'?'停用':'恢复'}</button><button className="icon-button danger-text" aria-label={`删除 ${user.username}`} onClick={()=>void remove(user)}><Icon name="trash" /></button></div></div>)}</div></section>
    <Dialog open={resetUser!==null} title={`重置 ${resetUser?.username||''} 的密码`} onClose={()=>setResetUser(null)}><form className="dialog-body" onSubmit={reset}><label className="field"><span>新密码</span><PasswordInput name="password" minLength={8} autoComplete="new-password" required autoFocus placeholder="至少 8 位" /></label><p className="form-intro">保存后，该用户当前登录会话会立即失效。</p><div className="dialog-actions"><button type="button" className="button secondary" onClick={()=>setResetUser(null)}>取消</button><button className="button primary" disabled={busy}>{busy?'保存中…':'确认重置'}</button></div></form></Dialog>
  </PageLayout>
}
