export type Protocol = 'RTU' | 'TCP'
export type Priority = 'CRITICAL' | 'HIGH' | 'NORMAL' | 'LOW'
export type DataType = 'INT16' | 'UINT16' | 'INT32' | 'UINT32' | 'FLOAT32' | 'FLOAT64'

export interface Device {
  id: number
  name: string
  protocol: Protocol
  interface?: string
  baud_rate?: number
  data_bits?: number
  parity?: string
  stop_bits?: number
  ip_address?: string
  port?: number
  slave_id: number
  polling_interval_ms: number
  timeout_ms: number
  retry: number
  enabled: boolean
  status?: string
  last_seen?: string
}

export interface DataPoint {
  id: number
  device_id: number
  tag_name: string
  function_code: number
  register_address: number
  data_type: DataType
  byte_order?: string
  word_order?: string
  scale: number
  offset: number
  unit?: string
  polling_interval_ms: number
  priority?: Priority
  enabled: boolean
}

export interface TestConnectionResult {
  quality: string
  error?: string
}

export interface TestReadResult {
  tag: string
  value: number | null
  unit?: string
  quality: string
  error?: string
}
