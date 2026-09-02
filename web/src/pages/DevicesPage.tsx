import { useEffect, useState } from 'react'
import { api } from '../api'
import type { Connection, Device } from '../types'
import { styles, qualityBadgeStyle, color } from '../styles'
import { Icon } from '../icons'
import { Modal } from '../components/Modal'
import { ConnectionForm } from '../components/ConnectionForm'
import { DeviceForm } from '../components/DeviceForm'
import { DataPointsPanel } from '../components/DataPointsPanel'
import { fmtNum } from '../format'

export function DevicesPage() {
  const [connections, setConnections] = useState<Connection[]>([])
  const [devices, setDevices] = useState<Device[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editingConnection, setEditingConnection] = useState<Connection | 'new' | null>(null)
  const [editingDevice, setEditingDevice] = useState<Device | 'new' | null>(null)
  const [selectedDeviceId, setSelectedDeviceId] = useState<number | null>(null)
  const [testResults, setTestResults] = useState<Record<number, string>>({})

  const load = () => {
    setLoading(true)
    Promise.all([api.listConnections(), api.listDevices()])
      .then(([conns, devs]) => {
        setConnections(conns)
        setDevices(devs)
      })
      .catch((err) => setError(String(err.message ?? err)))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, 5000)
    return () => clearInterval(interval)
  }, [])

  const connectionName = (id: number) => connections.find((c) => c.id === id)?.name ?? `#${id}`
  const connectionDelay = (id: number) => connections.find((c) => c.id === id)?.next_device_delay_ms

  const handleSaveConnection = async (connection: Partial<Connection>) => {
    if (editingConnection === 'new') {
      await api.createConnection(connection)
    } else if (editingConnection) {
      await api.updateConnection(editingConnection.id, connection)
    }
    setEditingConnection(null)
    load()
  }

  const handleDeleteConnection = async (connection: Connection) => {
    if (!confirm(`Delete connection "${connection.name}"? This is blocked while any device still uses it.`)) return
    try {
      await api.deleteConnection(connection.id)
      load()
    } catch (err) {
      setError(String(err instanceof Error ? err.message : err))
    }
  }

  const handleToggleConnectionEnabled = async (connection: Connection) => {
    await api.updateConnection(connection.id, { ...connection, enabled: !connection.enabled })
    load()
  }

  const handleSaveDevice = async (device: Partial<Device>) => {
    if (editingDevice === 'new') {
      await api.createDevice(device)
    } else if (editingDevice) {
      await api.updateDevice(editingDevice.id, device)
    }
    setEditingDevice(null)
    load()
  }

  const handleDeleteDevice = async (device: Device) => {
    if (!confirm(`Delete device "${device.name}" and all its data points?`)) return
    await api.deleteDevice(device.id)
    if (selectedDeviceId === device.id) setSelectedDeviceId(null)
    load()
  }

  const handleToggleDeviceEnabled = async (device: Device) => {
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
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
        <h2>Connections</h2>
        <button style={{ ...styles.primaryButton, display: 'inline-flex', alignItems: 'center', gap: 8 }} onClick={() => setEditingConnection('new')}>
          <Icon name="plus" size={16} color="#fff" />
          Add Connection
        </button>
      </div>

      {error && <div style={styles.errorBox}>{error}</div>}

      {loading && connections.length === 0 ? (
        <p style={styles.muted}>Loading…</p>
      ) : connections.length === 0 ? (
        <p style={styles.muted}>No connections configured yet — add one before adding devices.</p>
      ) : (
        <div style={{ background: '#fff', border: '1px solid #ECE9F7', borderRadius: 20, padding: 8, marginBottom: '1.5rem' }}>
          <table style={styles.table}>
            <thead>
              <tr>
                <th style={styles.th}>Name</th>
                <th style={styles.th}>Protocol</th>
                <th style={styles.th}>Address</th>
                <th style={styles.th}>Enabled</th>
                <th style={styles.th}></th>
              </tr>
            </thead>
            <tbody>
              {connections.map((c) => (
                <tr key={c.id}>
                  <td style={styles.td}>{c.name}</td>
                  <td style={styles.td}>{c.protocol}</td>
                  <td style={styles.td}>{c.protocol === 'TCP' ? `${c.ip_address}:${c.port}` : c.interface}</td>
                  <td style={styles.td}>
                    <input type="checkbox" style={styles.checkbox} checked={c.enabled} onChange={() => handleToggleConnectionEnabled(c)} />
                  </td>
                  <td style={styles.td}>
                    <div style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end' }}>
                      <button style={styles.smallButton} onClick={() => setEditingConnection(c)}>
                        Edit
                      </button>
                      <button style={styles.dangerButton} onClick={() => handleDeleteConnection(c)}>
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

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
        <h2>Devices</h2>
        <button
          style={{ ...styles.primaryButton, display: 'inline-flex', alignItems: 'center', gap: 8 }}
          onClick={() => setEditingDevice('new')}
          disabled={connections.length === 0}
        >
          <Icon name="plus" size={16} color="#fff" />
          Add Device
        </button>
      </div>

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
                <th style={styles.th}>Connection</th>
                <th style={styles.th}>Slave ID</th>
                <th style={styles.th}>Next-Device Delay</th>
                <th style={styles.th}>Poll Time</th>
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
                      title="Click to view/manage this device's data points"
                      style={{
                        ...styles.smallButton,
                        border: `1px solid ${selectedDeviceId === d.id ? color.accent : 'transparent'}`,
                        borderRadius: 999,
                        padding: '0.35rem 0.85rem',
                        background: selectedDeviceId === d.id ? color.accent : color.accentWash,
                        color: selectedDeviceId === d.id ? '#fff' : color.accent,
                        fontWeight: 700,
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: 6,
                      }}
                      onClick={() => setSelectedDeviceId(selectedDeviceId === d.id ? null : d.id)}
                    >
                      <span style={{ fontSize: '0.7em', transform: selectedDeviceId === d.id ? 'rotate(90deg)' : 'none', transition: 'transform 0.15s' }}>▶</span>
                      {d.name}
                    </button>
                  </td>
                  <td style={styles.td}>{connectionName(d.connection_id)}</td>
                  <td style={styles.td}>{d.slave_id}</td>
                  <td style={styles.td}>{connectionDelay(d.connection_id) ?? '—'} ms</td>
                  <td style={styles.td}>
                    {d.last_poll_duration_ms != null ? (
                      <span
                        title={`${d.datapoints_polled ?? 0} data point(s) read using ${d.block_reads ?? '?'} physical Modbus request(s) in the last poll cycle`}
                        style={
                          connectionDelay(d.connection_id) != null && d.last_poll_duration_ms > connectionDelay(d.connection_id)!
                            ? { color: '#C0392B', fontWeight: 600 }
                            : undefined
                        }
                      >
                        {fmtNum(d.last_poll_duration_ms)} ms ({d.datapoints_polled ?? 0} pts, {d.block_reads ?? '?'} reads)
                      </span>
                    ) : (
                      <span style={styles.muted}>—</span>
                    )}
                  </td>
                  <td style={styles.td}>
                    <input type="checkbox" style={styles.checkbox} checked={d.enabled} onChange={() => handleToggleDeviceEnabled(d)} />
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
                      <button style={styles.smallButton} onClick={() => setEditingDevice(d)}>
                        Edit
                      </button>
                      <button style={styles.dangerButton} onClick={() => handleDeleteDevice(d)}>
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

      {editingConnection && (
        <Modal title={editingConnection === 'new' ? 'Add Connection' : `Edit ${editingConnection.name}`} onClose={() => setEditingConnection(null)}>
          <ConnectionForm
            initial={editingConnection === 'new' ? undefined : editingConnection}
            onSubmit={handleSaveConnection}
            onCancel={() => setEditingConnection(null)}
          />
        </Modal>
      )}

      {editingDevice && (
        <Modal title={editingDevice === 'new' ? 'Add Device' : `Edit ${editingDevice.name}`} onClose={() => setEditingDevice(null)}>
          <DeviceForm
            initial={editingDevice === 'new' ? undefined : editingDevice}
            connections={connections}
            onSubmit={handleSaveDevice}
            onCancel={() => setEditingDevice(null)}
          />
        </Modal>
      )}
    </div>
  )
}
