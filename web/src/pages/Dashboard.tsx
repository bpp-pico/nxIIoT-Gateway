import { useEffect, useState } from 'react'
import { styles } from '../styles'

interface SystemInfo {
  status: string
  uptime_seconds: number
  go_version: string
  goroutines: number
  num_cpu: number
}

export function Dashboard() {
  const [system, setSystem] = useState<SystemInfo | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const fetchSystem = () => {
      fetch('/api/system')
        .then((res) => {
          if (!res.ok) throw new Error(`HTTP ${res.status}`)
          return res.json()
        })
        .then((data: SystemInfo) => {
          setSystem(data)
          setError(null)
        })
        .catch((err) => setError(String(err)))
    }

    fetchSystem()
    const interval = setInterval(fetchSystem, 5000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div>
      <h2 style={{ marginTop: 0 }}>Dashboard</h2>

      {error && <div style={styles.errorBox}>Cannot reach gateway API: {error}</div>}

      {system ? (
        <table style={styles.table}>
          <tbody>
            <tr>
              <td style={styles.td}>Status</td>
              <td style={styles.td}>{system.status}</td>
            </tr>
            <tr>
              <td style={styles.td}>Uptime</td>
              <td style={styles.td}>{system.uptime_seconds.toFixed(1)}s</td>
            </tr>
            <tr>
              <td style={styles.td}>Go Version</td>
              <td style={styles.td}>{system.go_version}</td>
            </tr>
            <tr>
              <td style={styles.td}>Goroutines</td>
              <td style={styles.td}>{system.goroutines}</td>
            </tr>
            <tr>
              <td style={styles.td}>CPUs</td>
              <td style={styles.td}>{system.num_cpu}</td>
            </tr>
          </tbody>
        </table>
      ) : (
        !error && <p style={styles.muted}>Loading...</p>
      )}
    </div>
  )
}
