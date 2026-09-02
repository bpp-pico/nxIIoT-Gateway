export type Protocol = 'RTU' | 'TCP'
export type Priority = 'CRITICAL' | 'HIGH' | 'NORMAL' | 'LOW'
export type DataType = 'INT16' | 'UINT16' | 'INT32' | 'UINT32' | 'FLOAT32' | 'FLOAT64'

export interface Connection {
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
  timeout_ms: number
  retry: number
  enabled: boolean
  next_device_delay_ms: number
}

export interface Device {
  id: number
  name: string
  connection_id: number
  slave_id: number
  enabled: boolean
  status?: string
  last_seen?: string
  last_poll_duration_ms?: number
  datapoints_polled?: number
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

export interface SystemInfo {
  status: string
  uptime_seconds: number
  go_version: string
  goroutines: number
  num_cpu: number
  cpu_percent?: number
  mem_used_percent?: number
  mem_total_mb?: number
  mem_used_mb?: number
  disk_used_percent?: number
  disk_total_gb?: number
  disk_used_gb?: number
  net_bytes_sent?: number
  net_bytes_recv?: number
  database_size_bytes?: number
}

export interface DashboardSummary {
  device_count: number
  enabled_device_count: number
  data_point_count: number
}

export interface StoreForwardStatus {
  pending_records: number
  sending_records: number
  oldest_pending?: string
  newest_pending?: string
  retry_count: number
  storage_used_percent?: number
  storage_level?: 'NORMAL' | 'WARNING' | 'CRITICAL' | 'FULL'
  server_connected: boolean
  server_last_error?: string
  server_last_sent_at?: string
}

export interface TimeStatus {
  system_time: string
  timezone: string
  ntp_server?: string
  ntp_status: boolean
  last_sync?: string
  clock_offset_ms?: number
  rtc_status: boolean
  rtc_time?: string
  time_quality: 'SYNCED' | 'RTC' | 'UNSYNCED' | 'INVALID'
}

export interface Diagnostics {
  modbus_tx: number
  modbus_rx: number
  avg_response_time_ms: number
  timeout_count: number
  crc_error_count: number
  retry_count: number
}

export interface LogEntry {
  time: string
  level: string
  message: string
  attrs?: Record<string, unknown>
}

export interface ConfigExport {
  exported_at: string
  gateway: { id: string; name: string }
  forwarder: Record<string, unknown>
  mqtt: Record<string, unknown>
  time: Record<string, unknown>
  connections: Connection[]
  devices: Array<Device & { data_points: DataPoint[] }>
}

export interface ConfigImportResult {
  connections_created: number
  connections_updated: number
  devices_created: number
  devices_updated: number
  data_points_created: number
  data_points_updated: number
  errors?: string[]
}

export interface Settings {
  gateway: { id: string; name: string }
  mqtt: {
    broker_url: string
    client_id: string
    username?: string
    password?: string
    qos: number
    data_topic?: string
    ack_topic?: string
    keepalive_seconds: number
  }
  time: {
    ntp_server: string
    timezone: string
    sync_interval_seconds: number
  }
  // Optional: absent when talking to a gateway build from before this field
  // existed (a real possibility here — the Web UI hot-reloads on `git pull`
  // independently of the Go binary, which needs a manual rebuild+swap).
  queue?: {
    retention_days: number
  }
}

export interface SaveSettingsResult {
  saved: boolean
  restarting: boolean
}

export interface NetworkStatus {
  supported: boolean
  pending_confirmation?: boolean
  interface?: string
  method?: string
  address?: string
  prefix?: number
  gateway?: string
  dns?: string[]
}

export interface ApplyNetworkRequest {
  interface: string
  address: string
  prefix: number
  gateway: string
  dns?: string[]
}

export interface ApplyNetworkDHCPRequest {
  interface: string
}

export interface ApplyNetworkResult {
  applied: boolean
  confirm_within_seconds: number
}
