import { useEffect, useState } from 'react'
import { api } from '../api'
import { styles } from '../styles'
import type { TimeStatus } from '../types'

function fmtTime(v?: string) {
  return v ? new Date(v).toLocaleString() : '—'
}

function qualityStyle(quality: TimeStatus['time_quality']) {
  if (quality === 'SYNCED') return styles.badgeGood
  if (quality === 'RTC' || quality === 'UNSYNCED') return styles.badgeNeutral
  return styles.badgeBad
}

export function TimePage() {
  const [status, setStatus] = useState<TimeStatus | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const load = () => {
      api
        .getTime()
        .then((t) => {
          setStatus(t)
          setError(null)
        })
        .catch((err) => setError(String(err instanceof Error ? err.message : err)))
    }
    load()
    const interval = setInterval(load, 5000)
    return () => clearInterval(interval)
  }, [])

  return (
    <div>
      <h2 style={{ marginTop: 0 }}>Time</h2>
      {error && <div style={styles.errorBox}>{error}</div>}

      {!status ? (
        !error && <p style={styles.muted}>Loading…</p>
      ) : (
        <>
          <div style={styles.cardGrid}>
            <div style={styles.card}>
              <div style={styles.cardTitle}>Time Quality</div>
              <div style={styles.cardValue}>
                <span style={qualityStyle(status.time_quality)}>{status.time_quality}</span>
              </div>
            </div>
            <div style={styles.card}>
              <div style={styles.cardTitle}>NTP Status</div>
              <div style={styles.cardValue}>
                <span style={status.ntp_status ? styles.badgeGood : styles.badgeBad}>
                  {status.ntp_status ? 'Synced' : 'Not Synced'}
                </span>
              </div>
            </div>
            <div style={styles.card}>
              <div style={styles.cardTitle}>RTC Status</div>
              <div style={styles.cardValue}>
                <span style={status.rtc_status ? styles.badgeGood : styles.badgeNeutral}>
                  {status.rtc_status ? 'Available' : 'Not Available'}
                </span>
              </div>
            </div>
            <div style={styles.card}>
              <div style={styles.cardTitle}>Clock Offset</div>
              <div style={styles.cardValue}>
                {status.clock_offset_ms !== undefined ? `${status.clock_offset_ms.toFixed(1)} ms` : '—'}
              </div>
            </div>
          </div>

          <table style={{ ...styles.table, marginTop: '1.5rem' }}>
            <tbody>
              <tr>
                <td style={styles.td}>System Time</td>
                <td style={styles.td}>{fmtTime(status.system_time)}</td>
              </tr>
              <tr>
                <td style={styles.td}>Timezone</td>
                <td style={styles.td}>{status.timezone}</td>
              </tr>
              <tr>
                <td style={styles.td}>NTP Server</td>
                <td style={styles.td}>{status.ntp_server || <span style={styles.muted}>not configured</span>}</td>
              </tr>
              <tr>
                <td style={styles.td}>Last Sync</td>
                <td style={styles.td}>{fmtTime(status.last_sync)}</td>
              </tr>
              <tr>
                <td style={styles.td}>RTC Time</td>
                <td style={styles.td}>{fmtTime(status.rtc_time)}</td>
              </tr>
            </tbody>
          </table>
        </>
      )}
    </div>
  )
}
