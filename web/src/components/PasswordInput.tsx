import { useState, type InputHTMLAttributes } from 'react'
import Icon from './Icon'

export default function PasswordInput(props: InputHTMLAttributes<HTMLInputElement>) {
  const [visible, setVisible] = useState(false)
  return <div className="password-field">
    <input {...props} type={visible ? 'text' : 'password'} />
    <button type="button" className="icon-button" aria-label={visible ? '隐藏密码' : '显示密码'} title={visible ? '隐藏密码' : '显示密码'} onClick={() => setVisible(value => !value)}>
      <Icon name={visible ? 'eye-off' : 'eye'} size={17} />
    </button>
  </div>
}
