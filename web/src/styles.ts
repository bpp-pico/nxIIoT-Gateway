import type { CSSProperties } from 'react'

export const styles = {
  page: { fontFamily: 'sans-serif', padding: '2rem', maxWidth: 900, margin: '0 auto' } satisfies CSSProperties,
  nav: { display: 'flex', gap: '1rem', marginBottom: '1.5rem', borderBottom: '1px solid #ccc', paddingBottom: '0.5rem' } satisfies CSSProperties,
  navButton: (active: boolean): CSSProperties => ({
    background: 'none',
    border: 'none',
    cursor: 'pointer',
    fontSize: '1rem',
    fontWeight: active ? 700 : 400,
    color: active ? '#111' : '#555',
    padding: '0.25rem 0',
    borderBottom: active ? '2px solid #111' : '2px solid transparent',
  }),
  table: { width: '100%', borderCollapse: 'collapse', marginTop: '0.5rem' } satisfies CSSProperties,
  th: { textAlign: 'left', borderBottom: '2px solid #ccc', padding: '0.4rem 0.6rem', fontSize: '0.85rem', color: '#555' } satisfies CSSProperties,
  td: { borderBottom: '1px solid #eee', padding: '0.4rem 0.6rem', fontSize: '0.9rem' } satisfies CSSProperties,
  button: { cursor: 'pointer', padding: '0.35rem 0.8rem', border: '1px solid #ccc', borderRadius: 4, background: '#fff', fontSize: '0.85rem' } satisfies CSSProperties,
  primaryButton: { cursor: 'pointer', padding: '0.35rem 0.8rem', border: '1px solid #111', borderRadius: 4, background: '#111', color: '#fff', fontSize: '0.85rem' } satisfies CSSProperties,
  dangerButton: { cursor: 'pointer', padding: '0.35rem 0.8rem', border: '1px solid #c0392b', borderRadius: 4, background: '#fff', color: '#c0392b', fontSize: '0.85rem' } satisfies CSSProperties,
  smallButton: { cursor: 'pointer', padding: '0.2rem 0.5rem', border: '1px solid #ccc', borderRadius: 4, background: '#fff', fontSize: '0.8rem' } satisfies CSSProperties,
  overlay: { position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.4)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 10 } satisfies CSSProperties,
  modal: { background: '#fff', borderRadius: 8, padding: '1.5rem', width: 480, maxHeight: '85vh', overflowY: 'auto', boxShadow: '0 8px 30px rgba(0,0,0,0.2)' } satisfies CSSProperties,
  formRow: { display: 'flex', flexDirection: 'column', gap: '0.2rem', marginBottom: '0.75rem' } satisfies CSSProperties,
  label: { fontSize: '0.8rem', color: '#555' } satisfies CSSProperties,
  input: { padding: '0.4rem 0.5rem', border: '1px solid #ccc', borderRadius: 4, fontSize: '0.9rem' } satisfies CSSProperties,
  errorBox: { background: '#fdecea', color: '#c0392b', padding: '0.5rem 0.75rem', borderRadius: 4, fontSize: '0.85rem', marginBottom: '0.75rem' } satisfies CSSProperties,
  badgeGood: { padding: '0.1rem 0.5rem', borderRadius: 12, background: '#e6f4ea', color: '#1e7e34', fontSize: '0.75rem' } satisfies CSSProperties,
  badgeBad: { padding: '0.1rem 0.5rem', borderRadius: 12, background: '#fdecea', color: '#c0392b', fontSize: '0.75rem' } satisfies CSSProperties,
  badgeNeutral: { padding: '0.1rem 0.5rem', borderRadius: 12, background: '#eee', color: '#555', fontSize: '0.75rem' } satisfies CSSProperties,
  muted: { color: '#888', fontSize: '0.85rem' } satisfies CSSProperties,
  cardGrid: { display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(220px, 1fr))', gap: '1rem', marginTop: '1rem' } satisfies CSSProperties,
  card: { border: '1px solid #eee', borderRadius: 8, padding: '1rem' } satisfies CSSProperties,
  cardTitle: { fontSize: '0.8rem', color: '#888', marginBottom: '0.4rem' } satisfies CSSProperties,
  cardValue: { fontSize: '1.4rem', fontWeight: 700 } satisfies CSSProperties,
  cardSub: { fontSize: '0.8rem', color: '#888', marginTop: '0.2rem' } satisfies CSSProperties,
  sectionTitle: { fontSize: '0.85rem', color: '#555', fontWeight: 700, marginTop: '1.5rem', marginBottom: '0.25rem' } satisfies CSSProperties,
  progressTrack: { background: '#eee', borderRadius: 4, height: 8, overflow: 'hidden' } satisfies CSSProperties,
  progressFill: (pct: number, danger: boolean): CSSProperties => ({
    width: `${Math.max(0, Math.min(100, pct))}%`,
    height: '100%',
    background: danger ? '#c0392b' : '#111',
  }),
  logLine: {
    display: 'grid',
    gridTemplateColumns: '5.5rem 3.5rem 1fr',
    gap: '0.5rem',
    alignItems: 'baseline',
    fontFamily: 'ui-monospace, monospace',
    fontSize: '0.8rem',
    whiteSpace: 'pre-wrap',
    padding: '0.15rem 0',
    borderBottom: '1px solid #f2f2f2',
    textAlign: 'left',
  } satisfies CSSProperties,
  logPanel: { maxHeight: '70vh', overflowY: 'auto', border: '1px solid #eee', borderRadius: 8, padding: '0.5rem 0.75rem', marginTop: '0.5rem', background: '#fafafa' } satisfies CSSProperties,
}

export function qualityBadgeStyle(quality?: string): CSSProperties {
  if (!quality) return styles.badgeNeutral
  return quality === 'GOOD' ? styles.badgeGood : styles.badgeBad
}
