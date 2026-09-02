import { useEffect, useState } from 'react'
import { api } from '../api'
import type { Connection, Protocol } from '../types'
import { styles } from '../styles'

const MANUAL_ENTRY = '__manual__'

interface ConnectionFormProps {
  initial?: Connection
  onSubmit: (connection: Partial<Connection>) => Promise<void>
  onCancel: () => void
}

const emptyForm = {
  name: '',
  protocol: 'TCP' as Protocol,
  interface: '',
  baud_rate: 9600,
  data_bits: 8,
  parity: 'N',
  stop_bits: 1,
  ip_address: '',
  port: 502,
  timeout_ms: 1000,
  retry: 3,
  enabled: true,
  next_device_delay_ms: 250,
}

export function ConnectionForm({ initial, onSubmit, onCancel }: ConnectionFormProps) {
  const [form, setForm] = useState({ ...emptyForm, ...initial })
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [ports, setPorts] = useState<string[] | null>(null)
  const [manualEntry, setManualEntry] = useState(false)

  const set = <K extends keyof typeof form>(key: K, value: (typeof form)[K]) =>
    setForm((f) => ({ ...f, [key]: value }))

  useEffect(() => {
    if (form.protocol !== 'RTU' || ports !== null) return
    api
      .listSerialPorts()
      .then((res) => {
        setPorts(res.ports)
        // No ports discovered (e.g. dev container with no device passthrough),
        // or the existing connection's interface isn't one of them: fall back
        // to manual entry so the field is never stuck unusable.
        if (res.ports.length === 0 || (form.interface && !res.ports.includes(form.interface))) {
          setManualEntry(true)
        }
      })
      .catch(() => {
        setPorts([])
        setManualEntry(true)
      })
  }, [form.protocol]) // intentionally excludes form.interface/ports: only (re)fetch when protocol changes

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

      <div style={styles.formRow}>
        <label style={styles.label}>Name</label>
        <input style={styles.input} value={form.name} onChange={(e) => set('name', e.target.value)} required />
      </div>

      <div style={styles.formRow}>
        <label style={styles.label}>Protocol</label>
        <select
          style={styles.input}
          value={form.protocol}
          onChange={(e) => set('protocol', e.target.value as Protocol)}
        >
          <option value="TCP">Modbus TCP</option>
          <option value="RTU">Modbus RTU</option>
        </select>
      </div>

      {form.protocol === 'TCP' ? (
        <>
          <div style={styles.formRow}>
            <label style={styles.label}>IP Address</label>
            <input style={styles.input} value={form.ip_address} onChange={(e) => set('ip_address', e.target.value)} required />
          </div>
          <div style={styles.formRow}>
            <label style={styles.label}>TCP Port</label>
            <input
              type="number"
              style={styles.input}
              value={form.port}
              onChange={(e) => set('port', Number(e.target.value))}
              required
            />
          </div>
        </>
      ) : (
        <>
          <div style={styles.formRow}>
            <label style={styles.label}>Serial Interface (COM Port)</label>
            {ports === null ? (
              <input style={styles.input} disabled value="Loading available ports…" />
            ) : manualEntry ? (
              <>
                <input
                  style={styles.input}
                  placeholder="e.g. /dev/ttyUSB0 or COM3"
                  value={form.interface}
                  onChange={(e) => set('interface', e.target.value)}
                  required
                />
                {ports.length > 0 && (
                  <button type="button" style={{ ...styles.smallButton, alignSelf: 'flex-start' }} onClick={() => setManualEntry(false)}>
                    Choose from detected ports
                  </button>
                )}
                {ports.length === 0 && <span style={styles.muted}>No serial ports detected on the gateway host — enter the path manually.</span>}
              </>
            ) : (
              <select
                style={styles.input}
                value={form.interface}
                onChange={(e) => {
                  if (e.target.value === MANUAL_ENTRY) {
                    setManualEntry(true)
                  } else {
                    set('interface', e.target.value)
                  }
                }}
                required
              >
                <option value="" disabled>
                  Select a port…
                </option>
                {ports.map((p) => (
                  <option key={p} value={p}>
                    {p}
                  </option>
                ))}
                <option value={MANUAL_ENTRY}>Other (type manually)…</option>
              </select>
            )}
          </div>
          <div style={{ display: 'flex', gap: '0.75rem' }}>
            <div style={{ ...styles.formRow, flex: 1 }}>
              <label style={styles.label}>Baud Rate</label>
              <input
                type="number"
                style={styles.input}
                value={form.baud_rate}
                onChange={(e) => set('baud_rate', Number(e.target.value))}
              />
            </div>
            <div style={{ ...styles.formRow, flex: 1 }}>
              <label style={styles.label}>Data Bits</label>
              <input
                type="number"
                style={styles.input}
                value={form.data_bits}
                onChange={(e) => set('data_bits', Number(e.target.value))}
              />
            </div>
          </div>
          <div style={{ display: 'flex', gap: '0.75rem' }}>
            <div style={{ ...styles.formRow, flex: 1 }}>
              <label style={styles.label}>Parity</label>
              <select style={styles.input} value={form.parity} onChange={(e) => set('parity', e.target.value)}>
                <option value="N">None</option>
                <option value="E">Even</option>
                <option value="O">Odd</option>
              </select>
            </div>
            <div style={{ ...styles.formRow, flex: 1 }}>
              <label style={styles.label}>Stop Bits</label>
              <input
                type="number"
                style={styles.input}
                value={form.stop_bits}
                onChange={(e) => set('stop_bits', Number(e.target.value))}
              />
            </div>
          </div>
        </>
      )}

      <div style={{ display: 'flex', gap: '0.75rem' }}>
        <div style={{ ...styles.formRow, flex: 1 }}>
          <label style={styles.label}>Timeout (ms)</label>
          <input
            type="number"
            style={styles.input}
            value={form.timeout_ms}
            onChange={(e) => set('timeout_ms', Number(e.target.value))}
          />
        </div>
        <div style={{ ...styles.formRow, flex: 1 }}>
          <label style={styles.label}>Retry</label>
          <input
            type="number"
            style={styles.input}
            value={form.retry}
            onChange={(e) => set('retry', Number(e.target.value))}
          />
        </div>
      </div>

      <div style={styles.formRow}>
        <label style={styles.label}>Delay Before Next Device (ms)</label>
        <input
          type="number"
          style={styles.input}
          value={form.next_device_delay_ms}
          onChange={(e) => set('next_device_delay_ms', Number(e.target.value))}
        />
        <span style={styles.muted}>
          How long the gateway waits after finishing one device's reads before moving to the next device sharing this
          connection (bus settle/turnaround time). Devices on this connection are now polled continuously,
          round-robin — there's no longer a separate per-device or per-tag interval to set.
        </span>
      </div>

      <div style={{ ...styles.formRow, flexDirection: 'row', alignItems: 'center', gap: '0.5rem' }}>
        <input
          type="checkbox"
          id="connection-enabled"
          checked={form.enabled}
          onChange={(e) => set('enabled', e.target.checked)}
        />
        <label htmlFor="connection-enabled" style={styles.label}>
          Enabled
        </label>
      </div>

      <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '0.5rem', marginTop: '1rem' }}>
        <button type="button" style={styles.button} onClick={onCancel} disabled={saving}>
          Cancel
        </button>
        <button type="submit" style={styles.primaryButton} disabled={saving}>
          {saving ? 'Saving…' : 'Save'}
        </button>
      </div>
    </form>
  )
}
