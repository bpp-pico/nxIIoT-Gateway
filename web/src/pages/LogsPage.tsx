import { useEffect, useState } from 'react'
import { api } from '../api'
import { styles } from '../styles'
import type { LogEntry } from '../types'

const levelColor: Record<string, string> = {
  ERROR: '#c0392b',
  WARN: '#b7791f',
  INFO: '#111',
  DEBUG: '#888',
}

export function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [error, setError] = useState<string | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(true)

  useEffect(() => {
    if (!autoRefresh) return
    const load = () => {
      api
        .getLogs(300)
        .then((l) => {
          setLogs(l)
          setError(null)
        })
        .catch((err) => setError(String(err instanceof Error ? err.message : err)))
    }
    load()
    const interval = setInterval(load, 3000)
    return () => clearInterval(interval)
  }, [autoRefresh])

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2 style={{ margin: 0 }}>Logs</h2>
        <label style={{ ...styles.muted, display: 'flex', alignItems: 'center', gap: '0.4rem' }}>
          <input type="checkbox" checked={autoRefresh} onChange={(e) => setAutoRefresh(e.target.checked)} />
          Auto-refresh
        </label>
      </div>
      <p style={styles.muted}>Most recent {logs.length} in-memory log entries (not persisted across restarts).</p>

      {error && <div style={styles.errorBox}>{error}</div>}

      <div style={styles.logPanel}>
        {logs.length === 0 ? (
          <p style={styles.muted}>No log entries yet.</p>
        ) : (
          logs.map((entry, i) => (
            <div key={i} style={styles.logLine}>
              <span style={styles.muted}>{new Date(entry.time).toLocaleTimeString()}</span>{' '}
              <span style={{ color: levelColor[entry.level] ?? '#111', fontWeight: 700 }}>{entry.level}</span>{' '}
              {entry.message}
              {entry.attrs && Object.keys(entry.attrs).length > 0 && (
                <span style={styles.muted}>
                  {' '}
                  {Object.entries(entry.attrs)
                    .map(([k, v]) => `${k}=${JSON.stringify(v)}`)
                    .join(' ')}
                </span>
              )}
            </div>
          ))
        )}
      </div>
    </div>
  )
}
