import { useEffect, useState } from 'react'
import { api } from '../api'
import type { DataPoint, Device } from '../types'
import { styles } from '../styles'
import { Modal } from './Modal'
import { DataPointForm } from './DataPointForm'

interface DataPointsPanelProps {
  device: Device
  onClose: () => void
}

export function DataPointsPanel({ device, onClose }: DataPointsPanelProps) {
  const [points, setPoints] = useState<DataPoint[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState<DataPoint | 'new' | null>(null)
  const [testResults, setTestResults] = useState<Record<number, string>>({})

  const load = () => {
    setLoading(true)
    api
      .listDataPoints(device.id)
      .then(setPoints)
      .catch((err) => setError(String(err.message ?? err)))
      .finally(() => setLoading(false))
  }

  useEffect(load, [device.id])

  const handleSave = async (dp: Partial<DataPoint>) => {
    if (editing === 'new') {
      await api.createDataPoint(device.id, dp)
    } else if (editing) {
      await api.updateDataPoint(editing.id, dp)
    }
    setEditing(null)
    load()
  }

  const handleDelete = async (dp: DataPoint) => {
    if (!confirm(`Delete data point "${dp.tag_name}"?`)) return
    await api.deleteDataPoint(dp.id)
    load()
  }

  const handleToggleEnabled = async (dp: DataPoint) => {
    await api.updateDataPoint(dp.id, { ...dp, enabled: !dp.enabled })
    load()
  }

  const handleTestRead = async (dp: DataPoint) => {
    setTestResults((r) => ({ ...r, [dp.id]: 'Reading…' }))
    try {
      const result = await api.testDataPoint(dp.id)
      const text =
        result.quality === 'GOOD'
          ? `${result.value} ${result.unit ?? ''}`
          : `${result.quality}${result.error ? `: ${result.error}` : ''}`
      setTestResults((r) => ({ ...r, [dp.id]: text }))
    } catch (err) {
      setTestResults((r) => ({ ...r, [dp.id]: String(err instanceof Error ? err.message : err) }))
    }
  }

  return (
    <div style={{ marginTop: '1rem', border: '1px solid #ddd', borderRadius: 8, padding: '1rem' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h3 style={{ margin: 0 }}>Data Points — {device.name}</h3>
        <div style={{ display: 'flex', gap: '0.5rem' }}>
          <button style={styles.primaryButton} onClick={() => setEditing('new')}>
            + Add Data Point
          </button>
          <button style={styles.smallButton} onClick={onClose}>
            Close
          </button>
        </div>
      </div>

      {error && <div style={styles.errorBox}>{error}</div>}
      {loading ? (
        <p style={styles.muted}>Loading…</p>
      ) : points.length === 0 ? (
        <p style={styles.muted}>No data points configured yet.</p>
      ) : (
        <table style={styles.table}>
          <thead>
            <tr>
              <th style={styles.th}>Tag</th>
              <th style={styles.th}>Function</th>
              <th style={styles.th}>Address</th>
              <th style={styles.th}>Data Type</th>
              <th style={styles.th}>Scale</th>
              <th style={styles.th}>Unit</th>
              <th style={styles.th}>Enabled</th>
              <th style={styles.th}>Test Read</th>
              <th style={styles.th}></th>
            </tr>
          </thead>
          <tbody>
            {points.map((dp) => (
              <tr key={dp.id}>
                <td style={styles.td}>{dp.tag_name}</td>
                <td style={styles.td}>{dp.function_code}</td>
                <td style={styles.td}>{dp.register_address}</td>
                <td style={styles.td}>{dp.data_type}</td>
                <td style={styles.td}>{dp.scale}</td>
                <td style={styles.td}>{dp.unit}</td>
                <td style={styles.td}>
                  <label>
                    <input type="checkbox" checked={dp.enabled} onChange={() => handleToggleEnabled(dp)} />
                  </label>
                </td>
                <td style={styles.td}>
                  <button style={styles.smallButton} onClick={() => handleTestRead(dp)}>
                    Test
                  </button>
                  {testResults[dp.id] && <span style={{ marginLeft: '0.5rem', fontSize: '0.8rem' }}>{testResults[dp.id]}</span>}
                </td>
                <td style={styles.td}>
                  <div style={{ display: 'flex', gap: '0.4rem' }}>
                    <button style={styles.smallButton} onClick={() => setEditing(dp)}>
                      Edit
                    </button>
                    <button style={{ ...styles.smallButton, color: '#c0392b' }} onClick={() => handleDelete(dp)}>
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {editing && (
        <Modal title={editing === 'new' ? 'Add Data Point' : `Edit ${editing.tag_name}`} onClose={() => setEditing(null)}>
          <DataPointForm
            initial={editing === 'new' ? undefined : editing}
            onSubmit={handleSave}
            onCancel={() => setEditing(null)}
          />
        </Modal>
      )}
    </div>
  )
}
