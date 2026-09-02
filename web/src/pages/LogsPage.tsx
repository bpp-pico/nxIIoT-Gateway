import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import { styles, logLevelColor, color } from '../styles'
import { fmtNum } from '../format'
import type { LogEntry } from '../types'

function fmtAttrValue(v: unknown): string {
  return typeof v === 'number' ? fmtNum(v) : JSON.stringify(v)
}

const LOG_LEVELS = ['ERROR', 'WARN', 'INFO', 'DEBUG']

export function LogsPage() {
  const [logs, setLogs] = useState<LogEntry[]>([])
  const [error, setError] = useState<string | null>(null)
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [enabledLevels, setEnabledLevels] = useState<Set<string>>(new Set(LOG_LEVELS))

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

  function toggleLevel(level: string) {
    setEnabledLevels((prev) => {
      const next = new Set(prev)
      if (next.has(level)) next.delete(level)
      else next.add(level)
      return next
    })
  }

  const filteredLogs = useMemo(() => logs.filter((l) => enabledLevels.has(l.level)), [logs, enabledLevels])

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h2>Logs</h2>
        <label style={{ ...styles.muted, display: 'flex', alignItems: 'center', gap: '0.5rem', fontWeight: 600 }}>
          <input type="checkbox" style={styles.checkbox} checked={autoRefresh} onChange={(e) => setAutoRefresh(e.target.checked)} />
          Auto-refresh
        </label>
      </div>
      <p style={styles.muted}>
        Showing {fmtNum(filteredLogs.length)} of {fmtNum(logs.length)} in-memory log entries (not persisted across restarts).
      </p>

      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '1rem' }}>
        {LOG_LEVELS.map((level) => {
          const active = enabledLevels.has(level)
          const levelColor = logLevelColor[level] ?? color.textSecondary
          return (
            <button
              key={level}
              onClick={() => toggleLevel(level)}
              title={active ? `Hide ${level} entries` : `Show ${level} entries`}
              style={{
                ...styles.smallButton,
                borderRadius: 999,
                padding: '0.3rem 0.8rem',
                border: `1px solid ${active ? levelColor : color.border}`,
                background: active ? `${levelColor}1A` : color.surface,
                color: active ? levelColor : color.textMuted,
                fontWeight: 700,
                fontSize: '0.75rem',
              }}
            >
              {level}
            </button>
          )
        })}
      </div>

      {error && <div style={styles.errorBox}>{error}</div>}

      <div style={styles.logPanel}>
        {filteredLogs.length === 0 ? (
          <p style={styles.muted}>{logs.length === 0 ? 'No log entries yet.' : 'No log entries match the selected levels.'}</p>
        ) : (
          filteredLogs.map((entry, i) => (
            <div key={i} style={styles.logLine}>
              <span style={styles.muted}>{new Date(entry.time).toLocaleTimeString()}</span>
              <span style={{ color: logLevelColor[entry.level] ?? '#1E1B2E', fontWeight: 700 }}>{entry.level}</span>
              <span>
                {entry.message}
                {entry.attrs && Object.keys(entry.attrs).length > 0 && (
                  <span style={styles.muted}>
                    {' '}
                    {Object.entries(entry.attrs)
                      .map(([k, v]) => `${k}=${fmtAttrValue(v)}`)
                      .join(' ')}
                  </span>
                )}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
