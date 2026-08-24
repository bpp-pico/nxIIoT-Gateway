import { useState } from 'react'
import { Dashboard } from './pages/Dashboard'
import { DevicesPage } from './pages/DevicesPage'
import { StoreForwardPage } from './pages/StoreForwardPage'
import { TimePage } from './pages/TimePage'
import { DiagnosticsPage } from './pages/DiagnosticsPage'
import { LogsPage } from './pages/LogsPage'
import { ConfigPage } from './pages/ConfigPage'
import { styles } from './styles'

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

  return (
    <div style={styles.page}>
      <h1>nxIIoT Gateway</h1>

      <nav style={styles.nav}>
        {tabs.map((t) => (
          <button key={t.id} style={styles.navButton(tab === t.id)} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </nav>

      {tab === 'dashboard' && <Dashboard />}
      {tab === 'devices' && <DevicesPage />}
      {tab === 'store-forward' && <StoreForwardPage />}
      {tab === 'time' && <TimePage />}
      {tab === 'diagnostics' && <DiagnosticsPage />}
      {tab === 'logs' && <LogsPage />}
      {tab === 'config' && <ConfigPage />}
    </div>
  )
}

export default App
