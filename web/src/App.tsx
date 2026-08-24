import { useState } from 'react'
import { Dashboard } from './pages/Dashboard'
import { DevicesPage } from './pages/DevicesPage'
import { styles } from './styles'

type Tab = 'dashboard' | 'devices'

function App() {
  const [tab, setTab] = useState<Tab>('dashboard')

  return (
    <div style={styles.page}>
      <h1>nxIIoT Gateway</h1>

      <nav style={styles.nav}>
        <button style={styles.navButton(tab === 'dashboard')} onClick={() => setTab('dashboard')}>
          Dashboard
        </button>
        <button style={styles.navButton(tab === 'devices')} onClick={() => setTab('devices')}>
          Devices
        </button>
      </nav>

      {tab === 'dashboard' ? <Dashboard /> : <DevicesPage />}
    </div>
  )
}

export default App
