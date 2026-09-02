import type { CSSProperties } from 'react'

// Modern Minimal design system — pastel purple + clean white.
// Palette (light-only; this is an internal ops tool, no dark mode requested).
export const color = {
  bg: '#FAFAFF',
  surface: '#FFFFFF',
  border: '#ECE9F7',
  borderStrong: '#DDD3FA',
  text: '#1E1B2E',
  textSecondary: '#6B6580',
  textMuted: '#9C96AF',
  accent: '#8B5CF6',
  accentHover: '#7C3AED',
  accentWash: '#F3EFFE',
  good: '#15803D',
  goodBg: '#EAFBF3',
  bad: '#DC2626',
  badBg: '#FDECEC',
  warn: '#B45309',
  warnBg: '#FFF7E6',
  neutral: '#6B6580',
  neutralBg: '#F1EEF9',
  track: '#F0EDFA',
}

const fontFamily = "'Plus Jakarta Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif"
const monoFamily = "'JetBrains Mono', ui-monospace, monospace"

export const styles = {
  page: { fontFamily, background: color.bg, minHeight: '100vh', color: color.text } satisfies CSSProperties,
  content: { maxWidth: 1360, margin: '0 auto', padding: '32px 40px 60px' } satisfies CSSProperties,

  // --- Top navbar ---
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: '1.5rem',
    padding: '1rem 2.5rem',
    background: color.surface,
    borderBottom: `1px solid ${color.border}`,
  } satisfies CSSProperties,
  brand: { display: 'flex', alignItems: 'center', gap: '0.75rem', flex: 'none' } satisfies CSSProperties,
  logoMark: {
    width: 36,
    height: 36,
    borderRadius: 10,
    background: `linear-gradient(135deg, #A78BFA, ${color.accent})`,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    flex: 'none',
  } satisfies CSSProperties,
  brandTitle: { fontWeight: 800, fontSize: '0.95rem', color: color.text, lineHeight: 1.15 } satisfies CSSProperties,
  brandSub: { fontSize: '0.7rem', color: color.textMuted } satisfies CSSProperties,
  nav: {
    display: 'flex',
    alignItems: 'center',
    gap: 2,
    background: color.bg,
    padding: 4,
    borderRadius: 14,
    border: `1px solid ${color.border}`,
    overflowX: 'auto',
  } satisfies CSSProperties,
  navButton: (active: boolean): CSSProperties => ({
    background: active ? color.accentWash : 'transparent',
    border: 'none',
    cursor: 'pointer',
    fontSize: '0.8rem',
    fontFamily,
    fontWeight: active ? 700 : 500,
    color: active ? color.accent : color.textSecondary,
    padding: '0.5rem 1rem',
    borderRadius: 10,
    whiteSpace: 'nowrap',
  }),
  onlineChip: (online: boolean): CSSProperties => ({
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '0.4rem 0.85rem',
    borderRadius: 999,
    background: online ? color.goodBg : color.badBg,
    flex: 'none',
  }),
  onlineDot: (online: boolean): CSSProperties => ({
    width: 8,
    height: 8,
    borderRadius: '50%',
    background: online ? color.good : color.bad,
  }),
  onlineLabel: (online: boolean): CSSProperties => ({
    fontSize: '0.8rem',
    fontWeight: 700,
    color: online ? color.good : color.bad,
  }),

  // --- Tables ---
  table: { width: '100%', borderCollapse: 'collapse', marginTop: '0.5rem' } satisfies CSSProperties,
  th: {
    textAlign: 'left',
    padding: '0 0.9rem 0.7rem',
    fontSize: '0.72rem',
    fontWeight: 700,
    color: color.textMuted,
    textTransform: 'uppercase',
    letterSpacing: '0.04em',
  } satisfies CSSProperties,
  td: { borderTop: `1px solid ${color.border}`, padding: '0.9rem', fontSize: '0.875rem' } satisfies CSSProperties,

  // --- Buttons ---
  button: {
    cursor: 'pointer',
    padding: '0.55rem 1.1rem',
    border: `1px solid ${color.borderStrong}`,
    borderRadius: 12,
    background: color.surface,
    color: color.accent,
    fontFamily,
    fontSize: '0.85rem',
    fontWeight: 700,
  } satisfies CSSProperties,
  primaryButton: {
    cursor: 'pointer',
    padding: '0.6rem 1.15rem',
    border: '1px solid transparent',
    borderRadius: 12,
    background: color.accent,
    color: '#fff',
    fontFamily,
    fontSize: '0.85rem',
    fontWeight: 700,
  } satisfies CSSProperties,
  dangerButton: {
    cursor: 'pointer',
    padding: '0.4rem 0.8rem',
    border: '1px solid #F6D2D2',
    borderRadius: 9,
    background: color.surface,
    color: color.bad,
    fontFamily,
    fontSize: '0.8rem',
    fontWeight: 600,
  } satisfies CSSProperties,
  smallButton: {
    cursor: 'pointer',
    padding: '0.4rem 0.8rem',
    border: `1px solid ${color.borderStrong}`,
    borderRadius: 9,
    background: color.surface,
    color: color.textSecondary,
    fontFamily,
    fontSize: '0.8rem',
    fontWeight: 600,
  } satisfies CSSProperties,

  // --- Modal / forms ---
  overlay: {
    position: 'fixed',
    inset: 0,
    background: 'rgba(30,27,46,0.35)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 10,
  } satisfies CSSProperties,
  modal: {
    background: color.surface,
    borderRadius: 20,
    padding: '1.75rem',
    width: 480,
    maxHeight: '85vh',
    overflowY: 'auto',
    boxShadow: '0 12px 40px rgba(30,27,46,0.16)',
    fontFamily,
  } satisfies CSSProperties,
  formRow: { display: 'flex', flexDirection: 'column', gap: '0.35rem', marginBottom: '0.9rem' } satisfies CSSProperties,
  label: { fontSize: '0.78rem', color: color.textSecondary, fontWeight: 500 } satisfies CSSProperties,
  input: {
    padding: '0.55rem 0.7rem',
    border: `1px solid ${color.borderStrong}`,
    borderRadius: 10,
    fontSize: '0.875rem',
    fontFamily,
    color: color.text,
    background: color.surface,
  } satisfies CSSProperties,
  select: {
    padding: '0.55rem 0.7rem',
    border: `1px solid ${color.borderStrong}`,
    borderRadius: 10,
    fontSize: '0.875rem',
    fontFamily,
    color: color.text,
    background: color.surface,
    cursor: 'pointer',
  } satisfies CSSProperties,
  checkbox: { accentColor: color.accent, width: 17, height: 17, cursor: 'pointer' } satisfies CSSProperties,
  errorBox: {
    background: color.badBg,
    color: color.bad,
    padding: '0.6rem 0.9rem',
    borderRadius: 10,
    fontSize: '0.85rem',
    marginBottom: '0.9rem',
  } satisfies CSSProperties,

  // --- Badges ---
  badgeGood: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    padding: '0.2rem 0.7rem',
    borderRadius: 999,
    background: color.goodBg,
    color: color.good,
    fontSize: '0.75rem',
    fontWeight: 700,
  } satisfies CSSProperties,
  badgeBad: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    padding: '0.2rem 0.7rem',
    borderRadius: 999,
    background: color.badBg,
    color: color.bad,
    fontSize: '0.75rem',
    fontWeight: 700,
  } satisfies CSSProperties,
  badgeNeutral: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    padding: '0.2rem 0.7rem',
    borderRadius: 999,
    background: color.neutralBg,
    color: color.neutral,
    fontSize: '0.75rem',
    fontWeight: 700,
  } satisfies CSSProperties,
  badgeDot: { width: 6, height: 6, borderRadius: '50%', background: 'currentColor', flex: 'none' } satisfies CSSProperties,

  muted: { color: color.textMuted, fontSize: '0.85rem' } satisfies CSSProperties,

  // --- Cards / stat tiles ---
  cardGrid: { display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: '1rem', marginTop: '1rem' } satisfies CSSProperties,
  card: { background: color.surface, border: `1px solid ${color.border}`, borderRadius: 20, padding: '1.25rem 1.35rem' } satisfies CSSProperties,
  cardIcon: {
    width: 36,
    height: 36,
    borderRadius: 10,
    background: color.accentWash,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: '0.85rem',
    color: color.accent,
  } satisfies CSSProperties,
  cardTitle: { fontSize: '0.8rem', color: color.textSecondary, marginBottom: '0.4rem' } satisfies CSSProperties,
  cardValue: { fontSize: '1.6rem', fontWeight: 800, letterSpacing: '-0.01em' } satisfies CSSProperties,
  cardSub: { fontSize: '0.75rem', color: color.textMuted, marginTop: '0.4rem' } satisfies CSSProperties,

  sectionTitle: { fontSize: '1rem', color: color.text, fontWeight: 800, marginTop: '2.25rem', marginBottom: '0.35rem' } satisfies CSSProperties,

  // --- Progress ---
  progressTrack: { background: color.track, borderRadius: 999, height: 7, overflow: 'hidden', marginTop: '0.75rem' } satisfies CSSProperties,
  progressFill: (pct: number, danger: boolean): CSSProperties => ({
    width: `${Math.max(0, Math.min(100, pct))}%`,
    height: '100%',
    borderRadius: 999,
    background: danger ? color.bad : color.accent,
  }),

  // --- Logs ---
  logLine: {
    display: 'grid',
    gridTemplateColumns: '5.5rem 3.5rem 1fr',
    gap: '0.9rem',
    alignItems: 'baseline',
    fontFamily: monoFamily,
    fontSize: '0.8rem',
    whiteSpace: 'pre-wrap',
    padding: '0.5rem 0.9rem',
    borderRadius: 10,
    textAlign: 'left',
  } satisfies CSSProperties,
  logPanel: { maxHeight: '70vh', overflowY: 'auto', border: `1px solid ${color.border}`, borderRadius: 20, padding: '0.5rem', marginTop: '0.5rem', background: color.surface } satisfies CSSProperties,
}

export function qualityBadgeStyle(quality?: string): CSSProperties {
  if (!quality) return styles.badgeNeutral
  return quality === 'GOOD' ? styles.badgeGood : styles.badgeBad
}

export const logLevelColor: Record<string, string> = {
  ERROR: '#DC2626',
  WARN: '#B45309',
  INFO: '#8B5CF6',
  DEBUG: '#9C96AF',
}
