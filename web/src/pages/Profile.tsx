import { useState, type FormEvent } from 'react'
import { api } from '../api/client'
import PageLayout from '../components/PageLayout'
import PasswordInput from '../components/PasswordInput'

export default function ProfilePage({ onLogout }: { onLogout: () => void }) {
  const [notice,setNotice]=useState('');const [busy,setBusy]=useState(false)
  const submit=async(event:FormEvent<HTMLFormElement>)=>{event.preventDefault();const form=event.currentTarget;const values=Object.fromEntries(new FormData(form).entries()) as Record<string,string>;if(values.next!==values.confirm){setNotice('两次输入的新密码不一致。');return}setBusy(true);try{await api.changeOwnPassword(values.current,values.next);onLogout()}catch(e){setNotice((e as Error).message)}finally{setBusy(false)}}
  return <PageLayout title="账号安全" eyebrow="个人设置" onLogout={onLogout}><section className="panel profile-panel"><div className="panel-header"><div><span className="eyebrow">登录密码</span><h2>修改密码</h2></div></div>{notice&&<div className="inline-banner">{notice}</div>}<form className="profile-form" onSubmit={submit}><label className="field"><span>当前密码</span><PasswordInput name="current" autoComplete="current-password" required /></label><label className="field"><span>新密码</span><PasswordInput name="next" minLength={8} autoComplete="new-password" required /></label><label className="field"><span>确认新密码</span><PasswordInput name="confirm" minLength={8} autoComplete="new-password" required /></label><button className="button primary" disabled={busy}>{busy?'保存中…':'保存新密码'}</button></form></section></PageLayout>
}
