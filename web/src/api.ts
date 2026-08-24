import type { DataPoint, Device, TestConnectionResult, TestReadResult } from './types'

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
}
