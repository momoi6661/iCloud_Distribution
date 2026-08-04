import React from 'react'
import ReactDOM from 'react-dom/client'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import App from './App'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={{
        token: {
          colorPrimary: '#667eea',
          colorInfo: '#667eea',
          borderRadius: 10,
          fontSize: 14,
        },
        components: {
          Card: { borderRadiusLG: 14 },
          Table: { headerBg: '#fafbfd' },
          Modal: { borderRadiusLG: 14 },
          Drawer: { borderRadiusLG: 14 },
        },
      }}
    >
      <App />
    </ConfigProvider>
  </React.StrictMode>,
)
