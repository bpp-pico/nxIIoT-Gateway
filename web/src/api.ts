import type {
  ApplyNetworkDHCPRequest,
  ApplyNetworkRequest,
  ApplyNetworkResult,
  ConfigExport,
  ConfigImportResult,
  DashboardSummary,
  DataPoint,
  Device,
  Diagnostics,
  LogEntry,
  NetworkStatus,
  SaveSettingsResult,
  Settings,
  StoreForwardStatus,
  SystemInfo,
  TestConnectionResult,
  TestReadResult,
  TimeStatus,
} from './types'

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!res.ok) {
    let message = `HTTP ${res.status}`
    try {
      const body = await res.json()
      if (body?.error) message = body.error
    } catch {
      // ignore non-JSON error bodies
    }
    throw new Error(message)
  }
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

export const api = {
  listSerialPorts: () => request<{ ports: string[] }>('/api/system/serial-ports'),

  listDevices: () => request<Device[]>('/api/devices'),
  createDevice: (d: Partial<Device>) =>
    request<Device>('/api/devices', { method: 'POST', body: JSON.stringify(d) }),
  updateDevice: (id: number, d: Partial<Device>) =>
    request<Device>(`/api/devices/${id}`, { method: 'PUT', body: JSON.stringify(d) }),
  deleteDevice: (id: number) => request<void>(`/api/devices/${id}`, { method: 'DELETE' }),
  testDevice: (id: number) => request<TestConnectionResult>(`/api/devices/${id}/test`, { method: 'POST' }),

  listDataPoints: (deviceId: number) => request<DataPoint[]>(`/api/devices/${deviceId}/datapoints`),
  createDataPoint: (deviceId: number, dp: Partial<DataPoint>) =>
    request<DataPoint>(`/api/devices/${deviceId}/datapoints`, { method: 'POST', body: JSON.stringify(dp) }),
  updateDataPoint: (id: number, dp: Partial<DataPoint>) =>
    request<DataPoint>(`/api/datapoints/${id}`, { method: 'PUT', body: JSON.stringify(dp) }),
  deleteDataPoint: (id: number) => request<void>(`/api/datapoints/${id}`, { method: 'DELETE' }),
  testDataPoint: (id: number) => request<TestReadResult>(`/api/datapoints/${id}/test`, { method: 'POST' }),

  getSystem: () => request<SystemInfo>('/api/system'),
  getDashboardSummary: () => request<DashboardSummary>('/api/dashboard/summary'),
  getStoreForwardStatus: () => request<StoreForwardStatus>('/api/store-forward/status'),
  getTime: () => request<TimeStatus>('/api/time'),
  getDiagnostics: () => request<Diagnostics>('/api/diagnostics'),
  getLogs: (limit = 300) => request<LogEntry[]>(`/api/logs?limit=${limit}`),

  exportConfig: () => request<ConfigExport>('/api/config/export'),
  importConfig: (payload: unknown) =>
    request<ConfigImportResult>('/api/config/import', { method: 'POST', body: JSON.stringify(payload) }),

  getSettings: () => request<Settings>('/api/config/settings'),
  saveSettings: (s: Settings) =>
    request<SaveSettingsResult>('/api/config/settings', { method: 'PUT', body: JSON.stringify(s) }),

  getNetworkStatus: () => request<NetworkStatus>('/api/system/network'),
  applyNetwork: (req: ApplyNetworkRequest) =>
    request<ApplyNetworkResult>('/api/system/network', { method: 'POST', body: JSON.stringify(req) }),
  applyNetworkDHCP: (req: ApplyNetworkDHCPRequest) =>
    request<ApplyNetworkResult>('/api/system/network/dhcp', { method: 'POST', body: JSON.stringify(req) }),
  confirmNetwork: () => request<{ confirmed: boolean }>('/api/system/network/confirm', { method: 'POST' }),
}
