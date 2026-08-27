import { useEffect, useState } from 'react'
import { api } from '../api'
import type { Device } from '../types'
import { styles, qualityBadgeStyle } from '../styles'
import { Icon } from '../icons'
import { Modal } from '../components/Modal'
import { DeviceForm } from '../components/DeviceForm'
import { DataPointsPanel } from '../components/DataPointsPanel'

export function DevicesPage() {
  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState<Device | 'new' | null>(null)
  const [selectedDeviceId, setSelectedDeviceId] = useState<number | null>(null)
  const [testResults, setTestResults] = useState<Record<number, string>>({})

  const load = () => {
    setLoading(true)
    api
      .listDevices()
      .then(setDevices)
      .catch((err) => setError(String(err.message ?? err)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, 5000)
    return () => clearInterval(interval)
  }, [])

  const handleSave = async (device: Partial<Device>) => {
    if (editing === 'new') {
      await api.createDevice(device)
    } else if (editing) {
      await api.updateDevice(editing.id, device)
    }
    setEditing(null)
    load()
  }

  const handleDelete = async (device: Device) => {
    if (!confirm(`Delete device "${device.name}" and all its data points?`)) return
    await api.deleteDevice(device.id)
    if (selectedDeviceId === device.id) setSelectedDeviceId(null)
    load()
  }

  const handleToggleEnabled = async (device: Device) => {
    await api.updateDevice(device.id, { ...device, enabled: !device.enabled })
    load()
  }

  const handleTestConnection = async (device: Device) => {
    setTestResults((r) => ({ ...r, [device.id]: 'Testing…' }))
    try {
      const result = await api.testDevice(device.id)
      setTestResults((r) => ({
        ...r,
        [device.id]: result.quality + (result.error ? `: ${result.error}` : ''),
      }))
    } catch (err) {
      setTestResults((r) => ({ ...r, [device.id]: String(err instanceof Error ? err.message : err) }))
    }
  }

  const selectedDevice = devices.find((d) => d.id === selectedDeviceId) ?? null

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
        <h2>Devices</h2>
        <button style={{ ...styles.primaryButton, display: 'inline-flex', alignItems: 'center', gap: 8 }} onClick={() => setEditing('new')}>
          <Icon name="plus" size={16} color="#fff" />
          Add Device
        </button>
      </div>

      {error && <div style={styles.errorBox}>{error}</div>}
      {loading && devices.length === 0 ? (
        <p style={styles.muted}>Loading…</p>
      ) : devices.length === 0 ? (
        <p style={styles.muted}>No devices configured yet.</p>
      ) : (
        <div style={{ background: '#fff', border: '1px solid #ECE9F7', borderRadius: 20, padding: 8 }}>
          <table style={styles.table}>
            <thead>
              <tr>
                <th style={styles.th}>Name</th>
                <th style={styles.th}>Protocol</th>
                <th style={styles.th}>Address</th>
                <th style={styles.th}>Polling</th>
                <th style={styles.th}>Enabled</th>
                <th style={styles.th}>Status</th>
                <th style={styles.th}>Test</th>
                <th style={styles.th}></th>
              </tr>
            </thead>
            <tbody>
              {devices.map((d) => (
                <tr key={d.id}>
                  <td style={styles.td}>
                    <button
                      style={{ ...styles.smallButton, border: 'none', padding: 0, background: 'transparent', color: '#1E1B2E', fontWeight: selectedDeviceId === d.id ? 700 : 600 }}
                      onClick={() => setSelectedDeviceId(selectedDeviceId === d.id ? null : d.id)}
                    >
                      {d.name}
                    </button>
                  </td>
                  <td style={styles.td}>{d.protocol}</td>
                  <td style={styles.td}>{d.protocol === 'TCP' ? `${d.ip_address}:${d.port}` : d.interface}</td>
                  <td style={styles.td}>{d.polling_interval_ms} ms</td>
                  <td style={styles.td}>
                    <input type="checkbox" style={styles.checkbox} checked={d.enabled} onChange={() => handleToggleEnabled(d)} />
                  </td>
                  <td style={styles.td}>
                    {d.status ? (
                      <span style={qualityBadgeStyle(d.status)}>
                        <span style={styles.badgeDot} />
                        {d.status}
                      </span>
                    ) : (
                      <span style={styles.muted}>—</span>
                    )}
                  </td>
                  <td style={styles.td}>
                    <button style={styles.smallButton} onClick={() => handleTestConnection(d)}>
                      Test
                    </button>
                    {testResults[d.id] && <span style={{ marginLeft: '0.5rem', fontSize: '0.8rem', color: '#6B6580' }}>{testResults[d.id]}</span>}
                  </td>
                  <td style={styles.td}>
                    <div style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end' }}>
                      <button style={styles.smallButton} onClick={() => setEditing(d)}>
                        Edit
                      </button>
                      <button style={styles.dangerButton} onClick={() => handleDelete(d)}>
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {selectedDevice && <DataPointsPanel device={selectedDevice} onClose={() => setSelectedDeviceId(null)} />}

      {editing && (
        <Modal title={editing === 'new' ? 'Add Device' : `Edit ${editing.name}`} onClose={() => setEditing(null)}>
          <DeviceForm initial={editing === 'new' ? undefined : editing} onSubmit={handleSave} onCancel={() => setEditing(null)} />
        </Modal>
      )}
    </div>
  )
}
