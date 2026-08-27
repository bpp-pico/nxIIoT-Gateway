// Minimal stroke-based icon set for the Web UI's Modern Minimal redesign.
// One consistent style (24px grid, round joins/caps) so every icon reads
// as part of the same family regardless of which card/page uses it.
import type { ReactElement } from 'react'

export type IconName =
  | 'gateway'
  | 'cpu'
  | 'ram'
  | 'storage'
  | 'network'
  | 'devices'
  | 'data-points'
  | 'cloud'
  | 'queue'
  | 'clock'
  | 'ntp'
  | 'rtc'
  | 'activity'
  | 'timeout'
  | 'crc'
  | 'retry'
  | 'sending'
  | 'plus'
  | 'download'
  | 'upload'

const paths: Record<IconName, ReactElement> = {
  gateway: (
    <>
      <rect x="4" y="14" width="3" height="6" rx="1" />
      <rect x="10.5" y="10" width="3" height="10" rx="1" />
      <rect x="17" y="5" width="3" height="15" rx="1" />
    </>
  ),
  cpu: (
    <>
      <rect x="6" y="6" width="12" height="12" rx="2" />
      <path d="M9 3v3M15 3v3M9 18v3M15 18v3M3 9h3M3 15h3M18 9h3M18 15h3" />
    </>
  ),
  ram: (
    <>
      <rect x="4" y="5" width="16" height="4" rx="1.5" />
      <rect x="4" y="10" width="16" height="4" rx="1.5" />
      <rect x="4" y="15" width="16" height="4" rx="1.5" />
    </>
  ),
  storage: (
    <>
      <ellipse cx="12" cy="6" rx="8" ry="3" />
      <path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6" />
      <path d="M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3" />
    </>
  ),
  network: (
    <>
      <path d="M12 3v12" />
      <path d="M7 10l5 5 5-5" />
      <path d="M5 21h14" />
    </>
  ),
  devices: (
    <>
      <path d="M9 2v4M15 2v4" />
      <path d="M7 6h10v5a5 5 0 0 1-10 0V6z" />
      <path d="M12 16v6" />
    </>
  ),
  'data-points': <path d="M4 9h16M4 15h16M9 4v16M15 4v16" />,
  cloud: (
    <>
      <path d="M7 18a4 4 0 0 1-1-7.9A5 5 0 0 1 16 8a4.5 4.5 0 0 1 1 8.9" />
      <path d="M9 15l2 2 4-4" />
    </>
  ),
  queue: <path d="M4 6h16M4 12h16M4 18h10" />,
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3.5 2" />
    </>
  ),
  ntp: (
    <>
      <path d="M12 2v4" />
      <path d="M8 6a6 6 0 0 1 8 0" />
      <path d="M5 9a10 10 0 0 1 14 0" />
      <circle cx="12" cy="18" r="2" />
    </>
  ),
  rtc: (
    <>
      <rect x="5" y="4" width="14" height="17" rx="2" />
      <path d="M9 2v4M15 2v4" />
      <circle cx="12" cy="14" r="3" />
    </>
  ),
  activity: <path d="M3 12h4l2 7 4-14 2 7h6" />,
  timeout: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3.5 2" />
    </>
  ),
  crc: (
    <>
      <path d="M10.3 3.9L2.5 17a2 2 0 0 0 1.7 3h15.6a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z" />
      <path d="M12 9v4M12 17h.01" />
    </>
  ),
  retry: (
    <>
      <path d="M3 12a9 9 0 1 0 3-6.7" />
      <path d="M3 3v5h5" />
    </>
  ),
  sending: (
    <>
      <path d="M4 4v6h6" />
      <path d="M20 20v-6h-6" />
      <path d="M5 15a8 8 0 0 0 14.3 3M19 9A8 8 0 0 0 4.7 6" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  download: (
    <>
      <path d="M7 16a4 4 0 0 1-1-7.9A5 5 0 0 1 16 8a4.5 4.5 0 0 1 1 8.9H8" />
      <path d="M12 11v6M9 15l3 3 3-3" />
    </>
  ),
  upload: (
    <>
      <path d="M7 16a4 4 0 0 1-1-7.9A5 5 0 0 1 16 8a4.5 4.5 0 0 1 1 8.9H8" />
      <path d="M12 17v-6M9 13l3-3 3 3" />
    </>
  ),
}

export function Icon({ name, size = 18, color = 'currentColor' }: { name: IconName; size?: number; color?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke={color} strokeWidth={1.75} strokeLinecap="round" strokeLinejoin="round">
      {paths[name]}
    </svg>
  )
}
