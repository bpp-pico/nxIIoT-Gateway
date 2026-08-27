import { useEffect, useState } from 'react'
import { Dashboard } from './pages/Dashboard'
import { DevicesPage } from './pages/DevicesPage'
import { StoreForwardPage } from './pages/StoreForwardPage'
import { TimePage } from './pages/TimePage'
import { DiagnosticsPage } from './pages/DiagnosticsPage'
import { LogsPage } from './pages/LogsPage'
import { ConfigPage } from './pages/ConfigPage'
import { styles } from './styles'
import { Icon } from './icons'
import { api } from './api'

type Tab = 'dashboard' | 'devices' | 'store-forward' | 'time' | 'diagnostics' | 'logs' | 'config'

const tabs: { id: Tab; label: string }[] = [
  { id: 'dashboard', label: 'Dashboard' },
  { id: 'devices', label: 'Devices' },
  { id: 'store-forward', label: 'Store & Forward' },
  { id: 'time', label: 'Time' },
  { id: 'diagnostics', label: 'Diagnostics' },
  { id: 'logs', label: 'Logs' },
  { id: 'config', label: 'Config' },
]

function App() {
  const [tab, setTab] = useState<Tab>('dashboard')
  const [online, setOnline] = useState<boolean | null>(null)

  useEffect(() => {
    let cancelled = false
    const check = () => {
      api
        .getSystem()
        .then(() => !cancelled && setOnline(true))
        .catch(() => !cancelled && setOnline(false))
    }
    check()
    const interval = setInterval(check, 5000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [])

  return (
    <div style={styles.page}>
      <header style={styles.header}>
        <div style={styles.brand}>
          <div style={styles.logoMark}>
            <Icon name="gateway" size={20} color="#fff" />
          </div>
          <div>
            <div style={styles.brandTitle}>nxIIoT Gateway</div>
            <div style={styles.brandSub}>GW001</div>
          </div>
        </div>

        <nav style={styles.nav}>
          {tabs.map((t) => (
            <button key={t.id} style={styles.navButton(tab === t.id)} onClick={() => setTab(t.id)}>
              {t.label}
            </button>
          ))}
        </nav>

        <div style={styles.onlineChip(online ?? false)}>
          <span style={styles.onlineDot(online ?? false)} />
          <span style={styles.onlineLabel(online ?? false)}>{online === null ? 'Checking…' : online ? 'Online' : 'Offline'}</span>
        </div>
      </header>

      <div style={styles.content}>
        {tab === 'dashboard' && <Dashboard />}
        {tab === 'devices' && <DevicesPage />}
        {tab === 'store-forward' && <StoreForwardPage />}
        {tab === 'time' && <TimePage />}
        {tab === 'diagnostics' && <DiagnosticsPage />}
        {tab === 'logs' && <LogsPage />}
        {tab === 'config' && <ConfigPage />}
      </div>
    </div>
  )
}

export default App
