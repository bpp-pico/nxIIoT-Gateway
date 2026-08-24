import { useRef, useState } from 'react'
import { api } from '../api'
import { styles } from '../styles'
import type { ConfigImportResult } from '../types'

export function ConfigPage() {
  const [error, setError] = useState<string | null>(null)
  const [importing, setImporting] = useState(false)
  const [importResult, setImportResult] = useState<ConfigImportResult | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleExport = async () => {
    setError(null)
    try {
      const data = await api.exportConfig()
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = `gateway-config-${data.gateway.id || 'export'}.json`
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    } catch (err) {
      setError(String(err instanceof Error ? err.message : err))
    }
  }

  const handleImportFile = async (file: File) => {
    setImporting(true)
    setImportResult(null)
    setError(null)
    try {
      const text = await file.text()
      const payload = JSON.parse(text)
      const result = await api.importConfig(payload)
      setImportResult(result)
    } catch (err) {
      setError(String(err instanceof Error ? err.message : err))
    } finally {
      setImporting(false)
    }
  }

  return (
    <div>
      <h2 style={{ marginTop: 0 }}>Configuration Backup &amp; Restore</h2>
      {error && <div style={styles.errorBox}>{error}</div>}

      <div style={styles.sectionTitle}>Backup</div>
      <p style={styles.muted}>
        Downloads devices, data points, and non-secret gateway settings as JSON. Passwords are never included.
      </p>
      <button style={styles.primaryButton} onClick={handleExport}>
        Export Configuration
      </button>

      <div style={styles.sectionTitle}>Restore</div>
      <p style={styles.muted}>
        Restores devices and data points from a previously exported file (matched by id — an id that doesn't exist on
        this gateway is created fresh). Gateway/Forwarder/MQTT/Time settings are not restored automatically: edit
        configs/config.yaml and restart the gateway for those.
      </p>
      <input
        ref={fileInputRef}
        type="file"
        accept="application/json"
        style={{ display: 'none' }}
        onChange={(e) => {
          const file = e.target.files?.[0]
          if (file) handleImportFile(file)
          e.target.value = ''
        }}
      />
      <button style={styles.button} disabled={importing} onClick={() => fileInputRef.current?.click()}>
        {importing ? 'Importing…' : 'Import Configuration'}
      </button>

      {importResult && (
        <table style={{ ...styles.table, marginTop: '1rem' }}>
          <tbody>
            <tr>
              <td style={styles.td}>Devices created</td>
              <td style={styles.td}>{importResult.devices_created}</td>
            </tr>
            <tr>
              <td style={styles.td}>Devices updated</td>
              <td style={styles.td}>{importResult.devices_updated}</td>
            </tr>
            <tr>
              <td style={styles.td}>Data points created</td>
              <td style={styles.td}>{importResult.data_points_created}</td>
            </tr>
            <tr>
              <td style={styles.td}>Data points updated</td>
              <td style={styles.td}>{importResult.data_points_updated}</td>
            </tr>
            {importResult.errors && importResult.errors.length > 0 && (
              <tr>
                <td style={styles.td}>Errors</td>
                <td style={styles.td}>
                  <ul style={{ margin: 0, paddingLeft: '1.2rem' }}>
                    {importResult.errors.map((e, i) => (
                      <li key={i}>{e}</li>
                    ))}
                  </ul>
                </td>
              </tr>
            )}
          </tbody>
        </table>
      )}
    </div>
  )
}
