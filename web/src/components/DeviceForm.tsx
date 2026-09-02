import { useEffect, useState } from 'react'
import type { Connection, Device } from '../types'
import { styles } from '../styles'

interface DeviceFormProps {
  initial?: Device
  connections: Connection[]
  onSubmit: (device: Partial<Device>) => Promise<void>
  onCancel: () => void
}

const emptyForm = {
  name: '',
  connection_id: 0,
  slave_id: 1,
  enabled: true,
}

export function DeviceForm({ initial, connections, onSubmit, onCancel }: DeviceFormProps) {
  const [form, setForm] = useState({
    ...emptyForm,
    connection_id: connections[0]?.id ?? 0,
    ...initial,
  })
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) =>
    setForm((f) => ({ ...f, [key]: value }))

  // If the device's own connection was deleted out from under it, fall back
  // to the first still-available connection rather than submitting a
  // connection_id that no longer exists.
  useEffect(() => {
    if (connections.length > 0 && !connections.some((c) => c.id === form.connection_id)) {
      set('connection_id', connections[0].id)
    }
  }, [connections])

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setSaving(true)
    setError(null)
    try {
      await onSubmit(form)
    } catch (err) {
      setError(String(err instanceof Error ? err.message : err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={handleSubmit}>
      {error && <div style={styles.errorBox}>{error}</div>}

      {connections.length === 0 && (
        <div style={styles.errorBox}>Add a Connection first — a device must belong to one.</div>
      )}

      <div style={styles.formRow}>
        <label style={styles.label}>Name</label>
        <input style={styles.input} value={form.name} onChange={(e) => set('name', e.target.value)} required />
      </div>

      <div style={styles.formRow}>
        <label style={styles.label}>Connection</label>
        <select
          style={styles.input}
          value={form.connection_id}
          onChange={(e) => set('connection_id', Number(e.target.value))}
          required
        >
          {connections.length === 0 && <option value={0}>No connections available</option>}
          {connections.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name} ({c.protocol === 'TCP' ? `${c.ip_address}:${c.port}` : c.interface})
            </option>
          ))}
        </select>
      </div>

      <div style={styles.formRow}>
        <label style={styles.label}>Slave ID (1-247)</label>
        <input
          type="number"
          min={1}
          max={247}
          style={styles.input}
          value={form.slave_id}
          onChange={(e) => set('slave_id', Number(e.target.value))}
          required
        />
      </div>

      <div style={{ ...styles.formRow, flexDirection: 'row', alignItems: 'center', gap: '0.5rem' }}>
        <input
          type="checkbox"
          id="device-enabled"
          checked={form.enabled}
          onChange={(e) => set('enabled', e.target.checked)}
        />
        <label htmlFor="device-enabled" style={styles.label}>
          Enabled
        </label>
      </div>

      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', marginTop: '1rem' }}>
        <button type="button" style={styles.button} onClick={onCancel} disabled={saving}>
          Cancel
        </button>
        <button type="submit" style={styles.primaryButton} disabled={saving || connections.length === 0}>
          {saving ? 'Saving…' : 'Save'}
        </button>
      </div>
    </form>
  )
}
