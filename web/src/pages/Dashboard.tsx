import { useEffect, useState } from 'react'
import { api } from '../api'
import { styles } from '../styles'
import type { DashboardSummary, StoreForwardStatus, SystemInfo, TimeStatus } from '../types'

function fmtPercent(v?: number) {
  return v === undefined ? '—' : `${v.toFixed(1)}%`
}

function fmtBytes(v?: number) {
  if (v === undefined) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let n = v
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(1)} ${units[i]}`
}

function timeQualityBadgeStyle(quality?: string) {
  if (quality === 'SYNCED') return styles.badgeGood
  if (quality === 'RTC' || quality === 'UNSYNCED') return styles.badgeNeutral
  return styles.badgeBad
}

export function Dashboard() {
  const [system, setSystem] = useState<SystemInfo | null>(null)
  const [summary, setSummary] = useState<DashboardSummary | null>(null)
  const [storeForward, setStoreForward] = useState<StoreForwardStatus | null>(null)
  const [time, setTime] = useState<TimeStatus | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const load = () => {
      Promise.all([api.getSystem(), api.getDashboardSummary(), api.getStoreForwardStatus(), api.getTime()])
        .then(([sys, sum, sf, t]) => {
          setSystem(sys)
          setSummary(sum)
          setStoreForward(sf)
          setTime(t)
          setError(null)
        })
        .catch((err) => setError(String(err instanceof Error ? err.message : err)))
    }

    load()
    const interval = setInterval(load, 5000)
    return () => clearInterval(interval)
  }, [])

  if (error && !system) {
    return (
      <div>
        <h2 style={{ marginTop: 0 }}>Dashboard</h2>
        <div style={styles.errorBox}>Cannot reach gateway API: {error}</div>
      </div>
    )
  }

  if (!system || !summary || !storeForward || !time) {
    return (
      <div>
        <h2 style={{ marginTop: 0 }}>Dashboard</h2>
        <p style={styles.muted}>Loading…</p>
      </div>
    )
  }

  return (
    <div>
      <h2 style={{ marginTop: 0 }}>Dashboard</h2>
      {error && <div style={styles.errorBox}>Last refresh failed: {error}</div>}

      <div style={styles.cardGrid}>
        <div style={styles.card}>
          <div style={styles.cardTitle}>Gateway Status</div>
          <div style={styles.cardValue}>{system.status === 'ok' ? 'Running' : system.status}</div>
          <div style={styles.cardSub}>uptime {(system.uptime_seconds / 60).toFixed(1)} min</div>
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>CPU</div>
          <div style={styles.cardValue}>{fmtPercent(system.cpu_percent)}</div>
          <div style={styles.progressTrack}>
            <div style={styles.progressFill(system.cpu_percent ?? 0, (system.cpu_percent ?? 0) >= 90)} />
          </div>
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>RAM</div>
          <div style={styles.cardValue}>{fmtPercent(system.mem_used_percent)}</div>
          <div style={styles.progressTrack}>
            <div style={styles.progressFill(system.mem_used_percent ?? 0, (system.mem_used_percent ?? 0) >= 90)} />
          </div>
          <div style={styles.cardSub}>
            {system.mem_used_mb?.toFixed(0)} / {system.mem_total_mb?.toFixed(0)} MB
          </div>
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>Storage</div>
          <div style={styles.cardValue}>{fmtPercent(system.disk_used_percent)}</div>
          <div style={styles.progressTrack}>
            <div style={styles.progressFill(system.disk_used_percent ?? 0, (system.disk_used_percent ?? 0) >= 90)} />
          </div>
          <div style={styles.cardSub}>
            {system.disk_used_gb?.toFixed(1)} / {system.disk_total_gb?.toFixed(1)} GB
          </div>
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>Network (cumulative)</div>
          <div style={styles.cardValue}>{fmtBytes(system.net_bytes_sent)}</div>
          <div style={styles.cardSub}>sent · {fmtBytes(system.net_bytes_recv)} received</div>
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>Devices</div>
          <div style={styles.cardValue}>
            {summary.enabled_device_count} / {summary.device_count}
          </div>
          <div style={styles.cardSub}>enabled / total</div>
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>Data Points</div>
          <div style={styles.cardValue}>{summary.data_point_count}</div>
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>Server Connection</div>
          <div style={styles.cardValue}>
            <span style={storeForward.server_connected ? styles.badgeGood : styles.badgeBad}>
              {storeForward.server_connected ? 'Connected' : 'Disconnected'}
            </span>
          </div>
          {storeForward.server_last_error && <div style={styles.cardSub}>{storeForward.server_last_error}</div>}
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>Pending Queue</div>
          <div style={styles.cardValue}>{storeForward.pending_records}</div>
          <div style={styles.cardSub}>{storeForward.retry_count} retries so far</div>
        </div>

        <div style={styles.card}>
          <div style={styles.cardTitle}>Time Synchronization</div>
          <div style={styles.cardValue}>
            <span style={timeQualityBadgeStyle(time.time_quality)}>{time.time_quality}</span>
          </div>
          <div style={styles.cardSub}>{time.ntp_server || 'no NTP server configured'}</div>
        </div>
      </div>
    </div>
  )
}
